package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aerosight/server/internal/agent"
	"aerosight/server/internal/algorithm"
	"aerosight/server/internal/credentials"
	"aerosight/server/internal/inspection"
	issueworker "aerosight/server/internal/issue"
	"aerosight/server/internal/mission"
	"aerosight/server/internal/outbox"
	"aerosight/server/internal/report"
)

type inspectionProtocolStore struct{}

func (inspectionProtocolStore) ReadAlgorithmAsset(_ context.Context, key string) (algorithm.AlgorithmAsset, error) {
	return algorithm.AlgorithmAsset{Body: []byte("protocol image:" + key), ContentType: "image/jpeg"}, nil
}
func (inspectionProtocolStore) PutRawResult(_ context.Context, key string, reader io.Reader, _ string) (algorithm.RawResultObject, error) {
	body, err := io.ReadAll(reader)
	sum := sha256.Sum256(body)
	return algorithm.RawResultObject{Key: key, ChecksumSHA256: hex.EncodeToString(sum[:])}, err
}

func TestInspectionExternalFormalAPIAndHTTPConsumer(t *testing.T) {
	f := newAPIFixture(t)
	// This expanded matrix sends hundreds of writes from one test account.
	// Rate-limit behavior is exercised independently by rate_limit_integration_test.
	f.server.writeRate = userRateLimiter(1000, 1000)
	team, pid := f.project(t)
	id := func(q string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := f.db.QueryRow(q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	store := inspectionProtocolStore{}
	signer := algorithm.NewAssetURLSigner(strings.Repeat("s", 32), "https://placeholder.example")
	gatewayHandler := algorithm.NewAssetAccessHandler(f.db, store, signer)
	gateway := httptest.NewTLSServer(gatewayHandler)
	defer gateway.Close()
	// Replace signer before requests start; the handler keeps the same object.
	*signer = *algorithm.NewAssetURLSigner(strings.Repeat("s", 32), gateway.URL)
	calls := 0
	scenario := ""
	var detectionEntered, detectionRelease chan struct{}
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var input algorithm.Input
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			http.Error(w, "bad input", 400)
			return
		}
		res, err := gateway.Client().Get(input.InputAsset.AccessURL)
		if err != nil {
			t.Error(err)
			http.Error(w, "media unavailable", 400)
			return
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		if err != nil || res.StatusCode != 200 || !strings.HasPrefix(string(body), "protocol image:") {
			t.Errorf("media: %d %v", res.StatusCode, err)
			http.Error(w, "media unavailable", 400)
			return
		}
		if scenario == "detect cancel inflight" {
			close(detectionEntered)
			select {
			case <-detectionRelease:
			case <-r.Context().Done():
				return
			}
		}
		if scenario == "failure" && calls%2 == 0 {
			http.Error(w, "fixture rejection", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if scenario == "detections" || (strings.HasPrefix(scenario, "assess create") || scenario == "assess rollback" || scenario == "assess cancel completed" || (scenario == "assess cancel inflight" || scenario == "assess pause inflight")) {
			io.WriteString(w, `{"modelRevision":"protocol-fixture-v1","results":[{"id":"d1","class":"candidate","score":0.9,"bbox":{"type":"bbox","x":1,"y":2,"width":3,"height":4}}]}`)
		} else {
			io.WriteString(w, `{"modelRevision":"protocol-fixture-v1","results":[]}`)
		}
	}))
	defer provider.Close()
	providerID := id("insert into algorithm_providers(project_id,team_id,name,provider_type,base_url,status) values($1,$2,'protocol fixture','http-json',$3,'active') returning id", pid, team, provider.URL)
	definition := id("insert into algorithm_definitions(project_id,team_id,provider_id,name,capability_code) values($1,$2,$3,'fixture','detection') returning id", pid, team, providerID)
	algorithmVersion := id(`insert into algorithm_definition_versions(project_id,team_id,algorithm_definition_id,version,status,execution_mode,model_or_process,output_mapping_json) values($1,$2,$3,1,'published','synchronous','fixture','{"detectionsPath":"results","keyPath":"id","labelPath":"class","confidencePath":"score","geometryPath":"bbox"}') returning id`, pid, team, definition)
	sourceTask := id("insert into tasks(project_id,team_id,name,trigger_type,script) values($1,$2,'source','manual','fixture') returning id", pid, team)
	sourceRun := id("insert into task_runs(project_id,team_id,task_id,trigger_source,status) values($1,$2,$3,'manual','succeeded') returning id", pid, team, sourceTask)
	assets := []int64{}
	for n := 0; n < 2; n++ {
		assets = append(assets, id("insert into assets(project_id,team_id,task_run_id,kind,mime_type,storage_key,logical_key) values($1,$2,$3,'image','image/jpeg',$4,$4) returning id", pid, team, sourceRun, fmt.Sprintf("projects/fixture/%d.jpg", n)))
	}
	algorithmWorker := algorithm.NewProcessor(provider.Client(), nil, store, "", signer, nil, "").WithInspectionWorker(f.db)
	newConsumer := func(workerID string) *outbox.Consumer {
		consumer := outbox.NewConsumer(outbox.NewStore(f.db), workerID, "external-fixture-consumer", slog.New(slog.NewTextHandler(io.Discard, nil)))
		dispatcher := mission.NewProcessor(nil)
		consumer.Register("task_run.triggered", dispatcher.Handler)
		consumer.Register("task_run.transitioned", dispatcher.Handler)
		consumer.Register("mission.control", dispatcher.Handler)
		observe := inspection.NewObserveProcessor(func(ctx context.Context, key string) ([]byte, error) {
			asset, err := store.ReadAlgorithmAsset(ctx, key)
			return asset.Body, err
		})
		consumer.Register("task.step.inspection.observe.requested", mission.WithTaskStepFailurePolicy(observe.Handler))
		detect := inspection.NewDetectProcessor(algorithm.NewTrigger(signer))
		consumer.Register("task.step.inspection.detect.requested", mission.WithTaskStepFailurePolicy(detect.Handler))
		consumer.Register("inspection.algorithm.completed", mission.WithTaskStepFailurePolicy(detect.Handler))
		consumer.Register("algorithm.run.requested", algorithmWorker.Handler)
		consumer.Register("task.step.report.requested", mission.WithTaskStepFailurePolicy(report.NewProcessor(nil).Handler))
		consumer.Register("task.step.copilot.requested", mission.WithTaskStepFailurePolicy(agent.TaskStepHandler))
		consumer.Register("task.step.issue.requested", mission.WithTaskStepFailurePolicy(issueworker.NewTaskStepProcessor(nil).Handler))
		return consumer
	}
	consumer := newConsumer("external-fixture-worker")
	call := func(method, path string, body any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		res := f.request(t, method, path, string(raw))
		out := decodedResponse(t, res)
		if res.StatusCode != want {
			t.Fatalf("%s: %d want %d: %+v", path, res.StatusCode, want, out)
		}
		return out
	}
	otherTeam, otherPID := f.project(t)
	otherProvider := id("insert into algorithm_providers(project_id,team_id,name,provider_type,base_url,status) values($1,$2,'foreign fixture','http-json',$3,'active') returning id", otherPID, otherTeam, provider.URL)
	otherDefinition := id("insert into algorithm_definitions(project_id,team_id,provider_id,name,capability_code) values($1,$2,$3,'foreign','detection') returning id", otherPID, otherTeam, otherProvider)
	otherVersion := id("insert into algorithm_definition_versions(project_id,team_id,algorithm_definition_id,version,status,execution_mode,model_or_process) values($1,$2,$3,1,'published','synchronous','fixture') returning id", otherPID, otherTeam, otherDefinition)

	modelCalls := 0
	var modelEntered, modelRelease chan struct{}
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelCalls++
		if strings.HasPrefix(scenario, "assess timeout") {
			_, _ = io.Copy(io.Discard, r.Body)
			if scenario == "assess timeout body" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				w.(http.Flusher).Flush()
			}
			select {
			case <-r.Context().Done():
			case <-time.After(time.Second):
			}
			return
		}
		var payload struct {
			Messages    []map[string]string `json:"messages"`
			Temperature float64             `json:"temperature"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || len(payload.Messages) != 2 {
			t.Error("bad model request")
			http.Error(w, "bad", 400)
			return
		}
		wantTemperature := 0.2
		if scenario == "assess zero" {
			wantTemperature = 0.7
		}
		if payload.Temperature != wantTemperature {
			t.Errorf("temperature=%v want=%v", payload.Temperature, wantTemperature)
		}
		text := payload.Messages[1]["content"]
		var data struct {
			Evidence inspection.EvidenceSet `json:"evidenceSet"`
		}
		start := strings.Index(text, "{")
		if start < 0 || json.Unmarshal([]byte(text[start:]), &data) != nil {
			t.Error("missing evidence")
			http.Error(w, "bad", 400)
			return
		}
		action := "no_issue"
		if scenario == "assess review" {
			action = "needs_review"
		}
		decision, _ := json.Marshal(map[string]any{"decisions": []any{map[string]any{"action": action, "reason": "protocol fixture limited scope", "evidenceRefs": data.Evidence.EvidenceRefs, "missingInformation": []string{}}}})
		if strings.HasPrefix(scenario, "assess create") || scenario == "assess rollback" || scenario == "assess cancel completed" || (scenario == "assess cancel inflight" || scenario == "assess pause inflight") {
			decisions := []any{}
			for _, candidate := range data.Evidence.Candidates {
				decisions = append(decisions, map[string]any{"candidateId": candidate.ID, "action": "create", "reason": "protocol fixture candidate", "evidenceRefs": candidate.EvidenceRefs, "missingInformation": []string{}})
			}
			decision, _ = json.Marshal(map[string]any{"decisions": decisions})
		}
		if scenario == "assess invalid" {
			decision = []byte("invalid JSON fixture")
		}
		if (scenario == "assess cancel inflight" || scenario == "assess pause inflight") || (scenario == "assess create stale lease" && modelCalls == 1) {
			close(modelEntered)
			select {
			case <-modelRelease:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(decision)}}}})
	}))
	defer modelServer.Close()
	var author int32
	if err := f.db.QueryRow("select user_id from team_members where team_id=$1", team).Scan(&author); err != nil {
		t.Fatal(err)
	}
	modelSecret := strings.Repeat("m", 64)
	modelProvider := id("insert into ai_providers(name,provider_type,base_url,model_id,credential_envelope_json,enabled,is_default,created_by_user_id,updated_by_user_id) values('protocol fixture','openai',$1,'fixture-model','{}',true,true,$2,$2) returning id", modelServer.URL, author)
	envelope, err := credentials.EncryptJSON(map[string]string{"apiKey": "fixture-key"}, modelSecret, credentials.AAD("ai-provider", fmt.Sprint(modelProvider), nil))
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(envelope)
	if _, err = f.db.Exec("update ai_providers set credential_envelope_json=$2 where id=$1", modelProvider, encoded); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec("update agents set status='active' where project_id=$1 and config_json->>'kind'='copilot'", pid); err != nil {
		t.Fatal(err)
	}
	modelWorker := agent.JobProcessor{Database: f.db, AuthSecret: modelSecret, HTTPClient: modelServer.Client()}

	for _, check := range []struct {
		name    string
		values  map[string]any
		publish bool
	}{
		{"valid", map[string]any{"algorithmDefinitionVersionId": algorithmVersion}, true},
		{"foreign definition", map[string]any{"algorithmDefinitionVersionId": otherVersion}, false},
		{"disabled provider", map[string]any{"algorithmDefinitionVersionId": algorithmVersion}, false},
		{"missing definition", map[string]any{}, false},
		{"unknown definition", map[string]any{"algorithmDefinitionVersionId": int64(2147483647)}, false},
		{"bad limit", map[string]any{"algorithmDefinitionVersionId": algorithmVersion, "maxImages": 0}, false},
		{"invalid parameter object", map[string]any{"algorithmDefinitionVersionId": algorithmVersion, "parameters": "bad"}, false},
	} {
		if check.name == "disabled provider" {
			if _, err := f.db.Exec("update algorithm_providers set status='disabled' where id=$1", providerID); err != nil {
				t.Fatal(err)
			}
		}
		values := map[string]any{"source": "external", "observationId": "steps.observe.outputs.observationId"}
		for key, value := range check.values {
			values[key] = value
		}
		source, _ := json.Marshal(map[string]any{"apiVersion": "aerosight/v2", "name": check.name, "trigger": map[string]any{"type": "manual"}, "steps": []any{
			map[string]any{"key": "observe", "uses": "inspection.observe", "with": map[string]any{"mode": "assets", "assetIds": assets}},
			map[string]any{"key": "detect", "uses": "inspection.detect", "with": values},
		}})
		result := call("POST", fmt.Sprintf("/api/projects/%d/tasks/validate", pid), map[string]any{"sourceFormat": "json", "source": string(source)}, 200)
		if result["canPublish"] != check.publish {
			t.Fatalf("%s: %+v", check.name, result)
		}
		if check.name == "disabled provider" {
			if _, err := f.db.Exec("update algorithm_providers set status='active' where id=$1", providerID); err != nil {
				t.Fatal(err)
			}
		}
		if calls != 0 {
			t.Fatal("publication validation contacted provider")
		}
	}

	for _, name := range []string{"detections", "zero", "failure", "limit", "detect cancel inflight", "assess zero", "assess product template", "assess review", "assess invalid", "assess timeout", "assess timeout body", "assess cancel", "assess cancel completed", "assess cancel inflight", "assess pause inflight", "assess rollback", "assess create", "assess create restart", "assess create lease", "assess create detection lease", "assess create stale lease", "assess create replay"} {
		t.Run(name, func(t *testing.T) {
			scenario = name
			if name == "detect cancel inflight" {
				detectionEntered, detectionRelease = make(chan struct{}), make(chan struct{})
			}
			calls = 0
			modelCalls = 0
			modelWorker.HTTPClient = modelServer.Client()
			if strings.HasPrefix(name, "assess timeout") {
				modelWorker.HTTPClient = &http.Client{Timeout: 100 * time.Millisecond}
			}
			if name == "assess cancel inflight" || name == "assess pause inflight" || name == "assess create stale lease" {
				modelEntered, modelRelease = make(chan struct{}), make(chan struct{})
			}
			if name == "assess rollback" {
				if _, err := f.db.Exec(fmt.Sprintf(`create function fail_second_inspection_issue() returns trigger language plpgsql as $$ begin if new.project_id=%d and exists(select 1 from issues where project_id=new.project_id) then raise exception 'fixture second issue failure'; end if; return new; end $$`, pid)); err != nil {
					t.Fatal(err)
				}
				if _, err := f.db.Exec(`create trigger fail_second_inspection_issue before insert on issues for each row execute function fail_second_inspection_issue()`); err != nil {
					t.Fatal(err)
				}
			}

			if name == "assess create replay" {
				if _, err := f.db.Exec("update issues set status='closed' where project_id=$1", pid); err != nil {
					t.Fatal(err)
				}
			}
			maxImages := 2
			if name == "limit" {
				maxImages = 1
			}
			source, _ := json.Marshal(map[string]any{"apiVersion": "aerosight/v2", "name": name, "trigger": map[string]any{"type": "manual"}, "steps": []any{
				map[string]any{"key": "observe", "uses": "inspection.observe", "with": map[string]any{"mode": "assets", "assetIds": assets}},
				map[string]any{"key": "detect", "uses": "inspection.detect", "with": map[string]any{"source": "external", "observationId": "steps.observe.outputs.observationId", "algorithmDefinitionVersionId": algorithmVersion, "maxImages": maxImages}},
				map[string]any{"key": "report", "uses": "report.generate"},
			}})
			if strings.HasPrefix(name, "assess ") {
				var definition map[string]any
				_ = json.Unmarshal(source, &definition)
				steps := definition["steps"].([]any)
				definition["steps"] = []any{steps[0], steps[1], map[string]any{"key": "assess", "uses": "copilot.run", "with": map[string]any{"mode": "assessment", "evidenceSetId": "steps.detect.outputs.evidenceSetId"}}, map[string]any{"key": "issues", "uses": "issue.create-or-update", "with": map[string]any{"assessmentId": "steps.assess.outputs.assessmentId"}}, steps[2]}
				if name == "assess zero" {
					definition["steps"].([]any)[2].(map[string]any)["with"].(map[string]any)["temperature"] = 0.7
				}
				source, _ = json.Marshal(definition)
			}
			if name == "assess product template" {
				definition := productInspectionTemplate(t, "assets")
				for _, raw := range definition["steps"].([]any) {
					step := raw.(map[string]any)
					parameters := step["with"].(map[string]any)
					if step["uses"] == "inspection.observe" {
						parameters["assetIds"] = assets
					}
					if step["uses"] == "inspection.detect" {
						parameters["algorithmDefinitionVersionId"] = algorithmVersion
					}
				}
				source, _ = json.Marshal(definition)
			}
			base := fmt.Sprintf("/api/projects/%d/tasks", pid)
			created := call("POST", base, map[string]any{"sourceFormat": "json", "source": string(source), "idempotencyKey": "external-" + name}, 201)
			path := fmt.Sprintf("%s/%.0f", base, created["taskId"])
			call("POST", path+"/versions", map[string]any{"action": "publish", "versionId": created["versionId"], "expectedRevision": 1}, 200)
			call("PATCH", path, map[string]any{"status": "active"}, 200)
			invocation := map[string]any{"type": "manual", "occurredAt": time.Now().UTC().Format(time.RFC3339Nano), "inputs": map[string]any{}, "idempotencyKey": "run-" + name}
			accepted := call("POST", path+"/runs", invocation, 201)
			run := int64(accepted["taskRunId"].(float64))
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			var state string
			restarts := 0
			detectionLeaseRecovered := false
			var claimedChildren string
			cancelAndAssert := func(expectedModelCalls int) {
				stateVersion := id("select state_version from task_runs where id=$1", run)
				call("POST", fmt.Sprintf("/api/projects/%d/task-runs/%d/control", pid, run), map[string]any{"action": "cancel", "expectedVersion": stateVersion, "reason": "cancel protocol inspection at " + name}, 200)
				// Drain real persisted work after cancellation, including the
				// queued model job and later outbox deliveries.
				for drain := 0; drain < 8; drain++ {
					if _, err := consumer.ConsumeOnce(ctx); err != nil {
						t.Fatal(err)
					}
					if _, err := algorithmWorker.ProcessInspectionNext(ctx); err != nil {
						t.Fatal(err)
					}
					if _, err := modelWorker.ProcessNext(ctx); err != nil {
						t.Fatal(err)
					}
				}

				if err := f.db.QueryRow("select status from task_runs where id=$1", run).Scan(&state); err != nil || state != "canceled" {
					t.Fatal("cancel failed", state, err)
				}
				if modelCalls != expectedModelCalls || calls != 2 {
					t.Fatal("canceled run invoked model or repeated detection", calls, modelCalls)
				}
				if count := id("select count(*) from generated_reports where project_id=$1 and source_id=$2", pid, fmt.Sprint(run)); count != 0 {
					t.Fatal("report after cancel", count)
				}
				if count := id("select count(*) from issues where project_id=$1", pid); count != 0 {
					t.Fatal("case after cancel", count)
				}
				if count := id("select count(*) from device_commands where project_id=$1", pid); count != 0 {
					t.Fatal("business cancel issued physical command", count)
				}
				replay := call("POST", path+"/runs", invocation, 200)
				if replay["taskRunId"] != accepted["taskRunId"] {
					t.Fatal("canceled replay created new run")
				}
			}
			for n := 0; n < 50; n++ {
				if _, err := consumer.ConsumeOnce(ctx); err != nil {
					t.Fatal(err)
				}
				if name == "assess create detection lease" && !detectionLeaseRecovered && id("select count(*) from algorithm_runs where task_run_id=$1 and status='queued'", run) == 2 {
					if err := f.db.QueryRow("select string_agg(id::text,',' order by id) from algorithm_runs where task_run_id=$1", run).Scan(&claimedChildren); err != nil {
						t.Fatal(err)
					}
					if _, err := f.db.Exec("update algorithm_runs set status='running',started_at=now() where task_run_id=$1", run); err != nil {
						t.Fatal(err)
					}
					if worked, err := algorithmWorker.ProcessInspectionNext(ctx); err != nil || worked {
						t.Fatal("valid detection lease reclaimed", worked, err)
					}
					if calls != 0 {
						t.Fatal("valid detection lease sent duplicate request")
					}
					if _, err := f.db.Exec("update algorithm_runs set started_at=now()-interval '3 minutes' where task_run_id=$1", run); err != nil {
						t.Fatal(err)
					}
					detectionLeaseRecovered = true
				}
				if name == "detect cancel inflight" && id("select count(*) from algorithm_runs where task_run_id=$1 and status='queued'", run) > 0 {
					done := make(chan error, 1)
					go func() { _, err := algorithmWorker.ProcessInspectionNext(ctx); done <- err }()
					released, joined := false, false
					defer func() {
						if !released {
							close(detectionRelease)
						}
						if !joined {
							<-done
						}
					}()
					select {
					case <-detectionEntered:
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
					stateVersion := id("select state_version from task_runs where id=$1", run)
					call("POST", fmt.Sprintf("/api/projects/%d/task-runs/%d/control", pid, run), map[string]any{"action": "cancel", "expectedVersion": stateVersion, "reason": "cancel while detection response is withheld"}, 200)
					close(detectionRelease)
					released = true
					select {
					case err := <-done:
						joined = true
						if err != nil {
							t.Fatal(err)
						}
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
					for drain := 0; drain < 8; drain++ {
						if _, err := consumer.ConsumeOnce(ctx); err != nil {
							t.Fatal(err)
						}
						if _, err := algorithmWorker.ProcessInspectionNext(ctx); err != nil {
							t.Fatal(err)
						}
					}
					if calls != 1 || modelCalls != 0 {
						t.Fatal("canceled detection started more remote work", calls, modelCalls)
					}
					if id("select count(*) from task_runs where id=$1 and status='canceled'", run) != 1 || id("select count(*) from algorithm_runs where task_run_id=$1 and status='canceled'", run) != 2 {
						t.Fatal("canceled children not terminal")
					}
					if id("select count(*) from generated_reports where project_id=$1 and source_id=$2", pid, fmt.Sprint(run)) != 0 || id("select count(*) from inspection_assessments where task_run_id=$1", run) != 0 {
						t.Fatal("late detection advanced workflow")
					}
					if id("select count(*) from algorithm_runs where task_run_id=$1 and raw_result_object_key is not null", run) != 1 {
						t.Fatal("late detection raw result not retained")
					}
					return
				}
				if _, err := algorithmWorker.ProcessInspectionNext(ctx); err != nil {
					t.Fatal(err)
				}

				if name == "assess cancel" && id("select count(*) from inspection_assessments where task_run_id=$1", run) > 0 {
					cancelAndAssert(0)
					return
				}
				if name == "assess create restart" && restarts == 0 && id("select count(*) from inspection_assessments where task_run_id=$1", run) == 1 {
					consumer = newConsumer("external-fixture-restarted-before-model")
					modelWorker = agent.JobProcessor{Database: f.db, AuthSecret: modelSecret, HTTPClient: modelServer.Client()}
					restarts++
				}
				if name == "assess create lease" && restarts == 0 && id("select count(*) from inspection_assessments where task_run_id=$1", run) == 1 {
					if _, err := f.db.Exec("update agent_tool_jobs set status='running',started_at=now() where project_id=$1 and tool_name='inspection_assessment' and (args_json->>'taskRunId')::bigint=$2", pid, run); err != nil {
						t.Fatal(err)
					}
					if processed, err := modelWorker.ProcessAssessmentNext(ctx); err != nil || processed {
						t.Fatal("active lease was reclaimed", processed, err)
					}
					if modelCalls != 0 {
						t.Fatal("active lease called model")
					}
					if _, err := f.db.Exec("update agent_tool_jobs set started_at=now()-interval '3 minutes' where project_id=$1 and tool_name='inspection_assessment' and (args_json->>'taskRunId')::bigint=$2", pid, run); err != nil {
						t.Fatal(err)
					}
					restarts++
				}
				if name == "assess create stale lease" && restarts == 0 && id("select count(*) from inspection_assessments where task_run_id=$1", run) == 1 {
					done := make(chan error, 1)
					go func() { _, err := modelWorker.ProcessAssessmentNext(ctx); done <- err }()
					released, joined := false, false
					defer func() {
						if !released {
							close(modelRelease)
						}
						if !joined {
							<-done
						}
					}()
					select {
					case <-modelEntered:
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
					// Explicitly inject a replacement lease while the original
					// HTTP response is outstanding. No new result exists yet.
					if _, err := f.db.Exec("update agent_tool_jobs set started_at=started_at+interval '1 second' where project_id=$1 and (args_json->>'taskRunId')::bigint=$2", pid, run); err != nil {
						t.Fatal(err)
					}
					close(modelRelease)
					released = true
					select {
					case err := <-done:
						joined = true
						if err != nil {
							t.Fatal(err)
						}
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
					if id("select count(*) from inspection_assessments where task_run_id=$1 and status='pending' and original_output is null", run) != 1 {
						t.Fatal("stale lease committed result")
					}
					if id("select count(*) from inspection_assessment_revisions where assessment_id in(select id from inspection_assessments where task_run_id=$1)", run) != 0 {
						t.Fatal("stale lease created revision")
					}
					if _, err := f.db.Exec("update agent_tool_jobs set started_at=now()-interval '3 minutes' where project_id=$1 and (args_json->>'taskRunId')::bigint=$2", pid, run); err != nil {
						t.Fatal(err)
					}
					restarts++
				}
				if (name == "assess cancel inflight" || name == "assess pause inflight") && id("select count(*) from inspection_assessments where task_run_id=$1", run) == 1 {
					done := make(chan error, 1)
					go func() { _, err := modelWorker.ProcessNext(ctx); done <- err }()
					released := false
					defer func() {
						if !released {
							close(modelRelease)
						}
						<-done
					}()
					select {
					case <-modelEntered:
					case <-ctx.Done():
						t.Fatal("model request not reached", ctx.Err())
					}
					// This must finish while the HTTP response is still withheld.
					if name == "assess pause inflight" {
						stateVersion := id("select state_version from task_runs where id=$1", run)
						call("POST", fmt.Sprintf("/api/projects/%d/task-runs/%d/control", pid, run), map[string]any{"action": "pause", "expectedVersion": stateVersion, "reason": "pause model in flight"}, 200)
					} else {
						cancelAndAssert(1)
					}
					close(modelRelease)
					released = true
					select {
					case err := <-done:
						done <- err
						if err != nil {
							t.Fatal(err)
						}
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
					for drain := 0; drain < 8; drain++ {
						if _, err := consumer.ConsumeOnce(ctx); err != nil {
							t.Fatal(err)
						}
						if _, err := algorithmWorker.ProcessInspectionNext(ctx); err != nil {
							t.Fatal(err)
						}
					}
					if id("select count(*) from issues where project_id=$1", pid) != 0 || id("select count(*) from generated_reports where project_id=$1 and source_id=$2", pid, fmt.Sprint(run)) != 0 {
						t.Fatal("late model response advanced canceled run")
					}
					var raw, status string
					if err := f.db.QueryRow("select original_output,status from inspection_assessments where task_run_id=$1", run).Scan(&raw, &status); err != nil || status != "canceled" || !strings.Contains(raw, `"create"`) {
						t.Fatal("late result not retained as canceled", status, err)
					}
					if name == "assess pause inflight" {
						if id("select count(*) from task_runs where id=$1 and status='paused'", run) != 1 {
							t.Fatal("late response changed paused run")
						}
						stateVersion := id("select state_version from task_runs where id=$1", run)
						call("POST", fmt.Sprintf("/api/projects/%d/task-runs/%d/control", pid, run), map[string]any{"action": "resume", "expectedVersion": stateVersion, "reason": "inspect failed assessment after pause"}, 200)
						for drain := 0; drain < 8; drain++ {
							if _, err := consumer.ConsumeOnce(ctx); err != nil {
								t.Fatal(err)
							}
							if _, err := algorithmWorker.ProcessInspectionNext(ctx); err != nil {
								t.Fatal(err)
							}
						}
						if id("select count(*) from task_runs where id=$1 and status='failed'", run) != 1 {
							t.Fatal("resume left failed assessment hanging")
						}
						if modelCalls != 1 || id("select count(*) from issues where project_id=$1", pid) != 0 {
							t.Fatal("resume repeated model or created case")
						}
					}
					return
				}
				if _, err := modelWorker.ProcessNext(ctx); err != nil {
					t.Fatal(err)
				}
				if name == "assess cancel completed" && id("select count(*) from inspection_assessments where task_run_id=$1 and status='succeeded'", run) == 1 {
					// The model result and continuation event are committed, but
					// the issue consumer has not processed that event yet.
					var original string
					if err := f.db.QueryRow("select original_output from inspection_assessments where task_run_id=$1", run).Scan(&original); err != nil || !strings.Contains(original, `"create"`) {
						t.Fatal("missing committed create decision", original, err)
					}
					cancelAndAssert(1)
					var preserved, assessmentState string
					if err := f.db.QueryRow("select original_output,status from inspection_assessments where task_run_id=$1", run).Scan(&preserved, &assessmentState); err != nil || preserved != original || assessmentState != "succeeded" {
						t.Fatal("cancel altered completed assessment", assessmentState, err)
					}
					return
				}
				if name == "assess create restart" && restarts == 1 && id("select count(*) from inspection_assessments where task_run_id=$1 and status='succeeded'", run) == 1 {
					consumer = newConsumer("external-fixture-restarted-after-model")
					modelWorker = agent.JobProcessor{Database: f.db, AuthSecret: modelSecret, HTTPClient: modelServer.Client()}
					restarts++
				}
				if err := f.db.QueryRow("select status from task_runs where id=$1", run).Scan(&state); err != nil {
					t.Fatal(err)
				}
				if state == "paused" && name == "assess review" {
					var assessmentID, evidenceID string
					if err := f.db.QueryRow("select id::text,evidence_set_id::text from inspection_assessments where task_run_id=$1", run).Scan(&assessmentID, &evidenceID); err != nil {
						t.Fatal(err)
					}
					var evidenceRaw []byte
					if err := f.db.QueryRow("select evidence_json from inspection_evidence_sets where id=$1", evidenceID).Scan(&evidenceRaw); err != nil {
						t.Fatal(err)
					}
					var evidence inspection.EvidenceSet
					_ = json.Unmarshal(evidenceRaw, &evidence)
					reviewPath := fmt.Sprintf("/api/projects/%d/inspection/assessments/%s/review", pid, assessmentID)
					input := map[string]any{"expectedRevision": 1, "idempotencyKey": "formal-review", "decisions": []any{map[string]any{"action": "reject", "reason": "human fixture dismissal", "evidenceRefs": evidence.EvidenceRefs, "missingInformation": []string{}}}}
					call("POST", reviewPath, input, 200)
					call("POST", reviewPath, input, 200)
				}
				if state == "succeeded" || state == "failed" {
					break
				}
			}
			if name == "detect cancel inflight" || name == "assess cancel" || name == "assess cancel completed" || name == "assess cancel inflight" || name == "assess pause inflight" {
				t.Fatal("queued assessment cancellation boundary was not reached")
			}
			want := "succeeded"
			if name == "failure" || name == "limit" || (name == "assess invalid" || name == "assess rollback" || strings.HasPrefix(name, "assess timeout")) {
				want = "failed"
			}
			if state != want {
				var detail []byte
				f.db.QueryRow("select jsonb_agg(jsonb_build_object('status',status,'output',output_snapshot_json)) from task_run_steps where task_run_id=$1", run).Scan(&detail)
				t.Fatalf("got %s want %s: %s", state, want, detail)
			}
			if name == "assess create detection lease" {
				if !detectionLeaseRecovered {
					t.Fatal("detection lease boundary not reached")
				}
				var restoredChildren string
				if err := f.db.QueryRow("select string_agg(id::text,',' order by id) from algorithm_runs where task_run_id=$1", run).Scan(&restoredChildren); err != nil || restoredChildren != claimedChildren {
					t.Fatal("recovery replaced child identities", err)
				}
				if id("select count(*) from algorithm_runs where task_run_id=$1 and status='succeeded'", run) != 2 {
					t.Fatal("recovered children did not complete")
				}
			}
			if strings.HasPrefix(name, "assess timeout") {
				var code, raw string
				if err := f.db.QueryRow("select failure_code,coalesce(original_output,'') from inspection_assessments where task_run_id=$1", run).Scan(&code, &raw); err != nil || code != "MODEL_REQUEST_TIMEOUT" || raw != "" {
					t.Fatal("timeout did not preserve explicit failure", code, raw, err)
				}
				if id("select count(*) from inspection_assessment_revisions where assessment_id in(select id from inspection_assessments where task_run_id=$1)", run) != 0 || id("select count(*) from generated_reports where project_id=$1 and source_id=$2", pid, fmt.Sprint(run)) != 0 || id("select count(*) from issues where project_id=$1", pid) != 0 {
					t.Fatal("timeout fabricated decisions or downstream results")
				}
			}
			expectedCalls := 2
			if name == "limit" {
				expectedCalls = 0
			}
			if calls != expectedCalls {
				t.Fatalf("provider calls=%d want=%d", calls, expectedCalls)
			}
			var evidenceCount int
			if err := f.db.QueryRow("select count(*) from inspection_evidence_sets where task_run_id=$1", run).Scan(&evidenceCount); err != nil {
				t.Fatal(err)
			}
			if want == "succeeded" {
				var raw []byte
				if err := f.db.QueryRow("select evidence_json from inspection_evidence_sets where task_run_id=$1", run).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				var evidence inspection.EvidenceSet
				if err := json.Unmarshal(raw, &evidence); err != nil {
					t.Fatal(err)
				}
				detections := 0
				if name == "detections" || strings.HasPrefix(name, "assess create") {
					detections = 2
				}
				if evidenceCount != 1 || len(evidence.ExternalResults) != 2 || len(evidence.Candidates) != detections || !evidence.TargetAlgorithmConfirmed {
					t.Fatalf("incorrect batch: %s", raw)
				}
			} else if name == "assess invalid" || name == "assess rollback" || strings.HasPrefix(name, "assess timeout") {
				if evidenceCount != 1 {
					t.Fatal("failed model lost detection evidence")
				}
			} else if evidenceCount != 0 {
				t.Fatal("failed batch sealed evidence")
			}
			if want == "succeeded" {
				var reportRaw []byte
				if err := f.db.QueryRow(`select version.content_json from generated_report_versions version join generated_reports report on report.id=version.generated_report_id and report.project_id=version.project_id where report.project_id=$1 and report.source_id=$2 order by version.version desc limit 1`, pid, fmt.Sprint(run)).Scan(&reportRaw); err != nil {
					t.Fatal(err)
				}
				var report struct {
					Assets     []map[string]any `json:"assets"`
					Issues     []map[string]any `json:"issues"`
					Inspection struct {
						ScopeNotice  string `json:"scopeNotice"`
						Observations []any  `json:"observations"`
						EvidenceSets []any  `json:"evidenceSets"`
						Assessments  []any  `json:"assessments"`
					} `json:"inspection"`
				}
				if err := json.Unmarshal(reportRaw, &report); err != nil {
					t.Fatal(err)
				}
				if len(report.Assets) != 2 || len(report.Inspection.Observations) != 1 || len(report.Inspection.EvidenceSets) != 1 || report.Inspection.ScopeNotice == "" {
					t.Fatalf("report lost analysis scope: %s", reportRaw)
				}
				if strings.HasPrefix(name, "assess ") && len(report.Inspection.Assessments) != 1 {
					t.Fatal("report lost assessment")
				}
				if name == "assess zero" {
					if len(report.Issues) != 0 {
						t.Fatal("no_issue report has cases")
					}
					a := report.Inspection.Assessments[0].(map[string]any)
					decisions := a["decisions"].([]any)
					if a["status"] != "succeeded" || len(decisions) != 1 || decisions[0].(map[string]any)["action"] != "no_issue" {
						t.Fatal("no_issue report lost formal decision", a)
					}
				}
				if strings.HasPrefix(name, "assess create") && len(report.Issues) != 2 {
					t.Fatal("report lost linked original issues")
				}
			}
			if (name == "assess create lease" || name == "assess create stale lease") && restarts != 1 {
				t.Fatal("lease recovery boundary not reached")
			}
			if name == "assess create restart" && restarts != 2 {
				t.Fatal("recovery boundaries not exercised", restarts)
			}
			expectedModelCalls := 1
			if name == "assess create stale lease" {
				expectedModelCalls = 2
			}
			if strings.HasPrefix(name, "assess ") && modelCalls != expectedModelCalls {
				t.Fatal("model repeat or missing call", modelCalls)
			}
			replay := call("POST", path+"/runs", invocation, 200)
			if replay["taskRunId"] != accepted["taskRunId"] {
				t.Fatal("replay changed run")
			}
			for n := 0; n < 3; n++ {
				if _, err := consumer.ConsumeOnce(ctx); err != nil {
					t.Fatal(err)
				}
				if _, err := algorithmWorker.ProcessInspectionNext(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if calls != expectedCalls {
				t.Fatal("replay dispatched provider")
			}
			if name == "assess rollback" {
				for _, table := range []string{"issues", "issue_links", "inspection_issue_sources"} {
					var count int
					if err := f.db.QueryRow("select count(*) from "+table+" where project_id=$1", pid).Scan(&count); err != nil || count != 0 {
						t.Fatal("half batch persisted", table, count, err)
					}
				}
				if _, err := f.db.Exec("drop trigger fail_second_inspection_issue on issues"); err != nil {
					t.Fatal(err)
				}
				if _, err := f.db.Exec("drop function fail_second_inspection_issue()"); err != nil {
					t.Fatal(err)
				}
			}
			if strings.HasPrefix(name, "assess create") {
				var count, total int
				if err := f.db.QueryRow("select count(*),sum(occurrence_count) from issues where project_id=$1", pid).Scan(&count, &total); err != nil || count != 2 || total != 2 {
					t.Fatal("source replay duplicated issues", count, total, err)
				}
				if name == "assess create replay" {
					if err := f.db.QueryRow("select count(*) from issues where project_id=$1 and status='closed'", pid).Scan(&count); err != nil || count != 2 {
						t.Fatal("closed issue reopened", count, err)
					}
				}
			}
			var count int
			if err := f.db.QueryRow("select count(*) from assets where task_run_id=$1", sourceRun).Scan(&count); err != nil || count != 2 {
				t.Fatal("asset provenance changed", count, err)
			}
		})
	}
}
