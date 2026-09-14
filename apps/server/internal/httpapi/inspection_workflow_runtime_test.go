package httpapi

import (
	"aerosight/server/internal/agent"
	"aerosight/server/internal/algorithm"
	issueworker "aerosight/server/internal/issue"
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

	"aerosight/server/internal/connector"
	"aerosight/server/internal/credentials"
	"aerosight/server/internal/flighthub"
	"aerosight/server/internal/inspection"
	"aerosight/server/internal/mission"
	"aerosight/server/internal/outbox"
	"aerosight/server/internal/report"
)

type inspectionRuntimeTransport func(*http.Request) (*http.Response, error)

func (f inspectionRuntimeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type inspectionRuntimeToken struct{}

func (inspectionRuntimeToken) ResolveToken(context.Context, connector.Instance) (string, error) {
	return "fixture-token", nil
}

func TestInspectionExistingFlightFormalAPIAndConsumer(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	id := func(q string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := f.db.QueryRow(q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := f.db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	adapter := id(`insert into device_adapters(project_id,team_id,name,adapter_type,protocol_version,status,discovery_scope_json) values($1,$2,'protocol fixture','dji-flighthub2','2','connected','{"projectUuid":"11111111-1111-4111-8111-111111111111","projectName":"fixture"}') returning id`, pid, team)
	sourceTask := id("insert into tasks(project_id,team_id,name,trigger_type,script) values($1,$2,'projected flight','manual','fixture') returning id", pid, team)
	sourceRun := id("insert into task_runs(project_id,team_id,task_id,trigger_source,status) values($1,$2,$3,'manual','succeeded') returning id", pid, team, sourceTask)
	exec("insert into connector_remote_resources(project_id,team_id,connector_instance_id,resource_kind,remote_id,canonical_target_type,canonical_target_id) values($1,$2,$3,'flight-task','fixture-flight','task_run',$4)", pid, team, adapter, fmt.Sprint(sourceRun))
	mediaResource := id("insert into connector_remote_resources(project_id,team_id,connector_instance_id,resource_kind,remote_id) values($1,$2,$3,'flight-media','fixture-media') returning id", pid, team, adapter)
	timestamp := "2026-09-01T10:00:00Z"
	imageBytes := "real bytes of protocol image fixture"
	versionSum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:image:jpg", timestamp, len(imageBytes))))
	version := hex.EncodeToString(versionSum[:16])
	asset := id("insert into assets(project_id,team_id,task_run_id,kind,mime_type,storage_key,logical_key,object_version) values($1,$2,$3,'image','image/jpeg','projects/fixture/remote.jpg','fixture-remote',$4) returning id", pid, team, sourceRun, version)
	secret := strings.Repeat("f", 64)
	locator := map[string]string{"taskUUID": "fixture-flight", "mediaUUID": "fixture-media"}
	envelope, err := credentials.EncryptJSON(locator, secret, credentials.AAD("flighthub-asset-reference", asset, pid))
	if err != nil {
		t.Fatal(err)
	}
	envelopeJSON, _ := json.Marshal(envelope)
	exec("insert into connector_asset_access_refs(id,project_id,team_id,connector_instance_id,remote_resource_id,access_kind,reference_digest,credential_envelope_json) values($1,$2,$3,$4,$5,'flight-media',$6,$7)", asset, pid, team, adapter, mediaResource, strings.Repeat("a", 64), envelopeJSON)
	countsKnown := true
	requests := 0
	mediaQueries := 0
	changeVersion := false
	assetMode := false
	duplicateMedia := false
	provider, err := flighthub.NewChinaClient(flighthub.Config{RequestID: func() string { return "fixture" }, AllowedLinkHosts: []string{"media.fixture.example"}, RequestsPerSecond: 100, RequestBurst: 100, HTTPClient: &http.Client{Transport: inspectionRuntimeTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		response := func(body string) *http.Response {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}
		}
		if r.URL.Hostname() == "media.fixture.example" {
			if r.Header.Get("Authorization") != "" || r.Header.Get("X-User-Token") != "" {
				t.Error("API token sent to image host")
			}
			return response(imageBytes), nil
		}
		switch r.URL.Path {
		case "/openapi/v2.0/flight-task/fixture-flight":
			folder := "{}"
			if countsKnown {
				folder = `{"expected_file_count":1,"uploaded_file_count":1}`
			}
			return response(fmt.Sprintf(`{"code":0,"message":"","data":{"uuid":"fixture-flight","name":"fixture","task_type":"immediate","status":"success","sn":"fixture-device","wayline_uuid":"fixture-wayline","begin_at":"%s","end_at":"%s","folder_info":%s}}`, timestamp, timestamp, folder)), nil
		case "/openapi/v2.0/flight-task/fixture-flight/media":
			mediaQueries++
			updated := timestamp
			if changeVersion && (assetMode || mediaQueries > 1) {
				updated = "2026-09-01T10:01:00Z"
			}
			body := fmt.Sprintf(`{"code":0,"message":"","data":{"list":[{"uuid":"fixture-media","name":"fixture","file_type":"image","suffix":"jpg","size":%d,"preview_url":"","original_url":"https://media.fixture.example/image?auth_key=%d-0-0-test","create_at":"%s","update_at":"%s"}]}}`, len(imageBytes), time.Now().Add(time.Minute).Unix(), timestamp, updated)
			if duplicateMedia {
				start := strings.Index(body, "[")
				end := strings.LastIndex(body, "]")
				item := body[start+1 : end]
				body = body[:start+1] + item + "," + item + body[end:]
			}
			return response(body), nil
		default:
			t.Errorf("unexpected provider request %s", r.URL.Path)
			return nil, fmt.Errorf("unexpected fixture request")
		}
	})}})
	if err != nil {
		t.Fatal(err)
	}
	access, err := flighthub.NewFlightAssetAccessService(f.db, provider, inspectionRuntimeToken{}, secret, nil)
	if err != nil {
		t.Fatal(err)
	}
	signer := algorithm.NewAssetURLSigner(strings.Repeat("s", 32), "https://placeholder.example")
	gateway := httptest.NewTLSServer(algorithm.NewAssetAccessHandler(f.db, nil, signer).WithRemoteReader(access.ReadAlgorithmAsset))
	defer gateway.Close()
	*signer = *algorithm.NewAssetURLSigner(strings.Repeat("s", 32), gateway.URL)
	algorithmCalls := 0
	algorithmServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		algorithmCalls++
		var input algorithm.Input
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			http.Error(w, "bad input", 400)
			return
		}
		res, err := gateway.Client().Get(input.InputAsset.AccessURL)
		if err != nil {
			t.Error(err)
			http.Error(w, "unavailable", 400)
			return
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		if err != nil || res.StatusCode != 200 || string(body) != imageBytes {
			t.Errorf("remote bytes status=%d err=%v", res.StatusCode, err)
			http.Error(w, "bad image", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"modelRevision":"remote-protocol-fixture-v1","results":[]}`)
	}))
	defer algorithmServer.Close()
	algorithmProvider := id("insert into algorithm_providers(project_id,team_id,name,provider_type,base_url,status) values($1,$2,'remote fixture','http-json',$3,'active') returning id", pid, team, algorithmServer.URL)
	algorithmDefinition := id("insert into algorithm_definitions(project_id,team_id,provider_id,name,capability_code) values($1,$2,$3,'fixture','detection') returning id", pid, team, algorithmProvider)
	algorithmVersion := id(`insert into algorithm_definition_versions(project_id,team_id,algorithm_definition_id,version,status,execution_mode,model_or_process,output_mapping_json) values($1,$2,$3,1,'published','synchronous','fixture','{"detectionsPath":"results"}') returning id`, pid, team, algorithmDefinition)
	observer := flighthub.NewInspectionFlightObserver(provider, access, inspectionRuntimeToken{})
	consumer := outbox.NewConsumer(outbox.NewStore(f.db), "inspection-fixture-worker", "inspection-fixture-consumer", slog.New(slog.NewTextHandler(io.Discard, nil)))
	dispatcher := mission.NewProcessor(nil)
	consumer.Register("task_run.triggered", dispatcher.Handler)
	consumer.Register("task_run.transitioned", dispatcher.Handler)
	consumer.Register("task.step.inspection.observe.requested", mission.WithTaskStepFailurePolicy(inspection.NewObserveProcessor(nil, observer.Observe).WithRemoteAssetReader(observer.ReadAsset).Handler))
	detect := inspection.NewDetectProcessor(algorithm.NewTrigger(signer))
	consumer.Register("task.step.inspection.detect.requested", mission.WithTaskStepFailurePolicy(detect.Handler))
	consumer.Register("inspection.algorithm.completed", mission.WithTaskStepFailurePolicy(detect.Handler))
	consumer.Register("algorithm.run.requested", algorithm.NewProcessor(algorithmServer.Client(), nil, inspectionProtocolStore{}, "", signer, nil, "").Handler)
	consumer.Register("task.step.report.requested", mission.WithTaskStepFailurePolicy(report.NewProcessor(nil).Handler))
	call := func(method, path string, body any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		res := f.request(t, method, path, string(raw))
		out := decodedResponse(t, res)
		if res.StatusCode != want {
			t.Fatalf("%s status %d want %d: %+v", path, res.StatusCode, want, out)
		}
		return out
	}

	modelCalls := 0
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelCalls++
		var request struct {
			Messages []map[string]string `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil || len(request.Messages) != 2 {
			http.Error(w, "bad fixture request", 400)
			return
		}
		content := request.Messages[1]["content"]
		start := strings.Index(content, "{")
		var input struct {
			Evidence inspection.EvidenceSet `json:"evidenceSet"`
		}
		if start < 0 || json.Unmarshal([]byte(content[start:]), &input) != nil {
			http.Error(w, "bad fixture evidence", 400)
			return
		}
		decision, _ := json.Marshal(map[string]any{"decisions": []any{map[string]any{"action": "needs_review", "reason": "protocol fixture: native coverage unknown", "evidenceRefs": input.Evidence.EvidenceRefs, "missingInformation": []string{"coverage confirmation"}}}})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(decision)}}}})
	}))
	defer modelServer.Close()
	author := id("select user_id from team_members where team_id=$1", team)
	modelSecret := strings.Repeat("m", 64)
	modelProvider := id("insert into ai_providers(name,provider_type,base_url,model_id,credential_envelope_json,enabled,is_default,created_by_user_id,updated_by_user_id) values('review protocol fixture','openai',$1,'fixture-model','{}',true,true,$2,$2) returning id", modelServer.URL, author)
	modelEnvelope, modelErr := credentials.EncryptJSON(map[string]string{"apiKey": "fixture-key"}, modelSecret, credentials.AAD("ai-provider", fmt.Sprint(modelProvider), nil))
	if modelErr != nil {
		t.Fatal(modelErr)
	}
	encodedModel, _ := json.Marshal(modelEnvelope)
	exec("update ai_providers set credential_envelope_json=$2 where id=$1", modelProvider, encodedModel)
	exec("update agents set status='active' where project_id=$1 and config_json->>'kind'='copilot'", pid)
	modelWorker := agent.JobProcessor{Database: f.db, AuthSecret: modelSecret, HTTPClient: modelServer.Client()}
	consumer.Register("task.step.copilot.requested", mission.WithTaskStepFailurePolicy(agent.TaskStepHandler))
	consumer.Register("task.step.issue.requested", mission.WithTaskStepFailurePolicy(issueworker.NewTaskStepProcessor(nil).Handler))
	for _, tc := range []struct {
		name           string
		known, confirm bool
		want           string
	}{
		{"product template", true, false, "succeeded"}, {"external existing flight", true, false, "succeeded"}, {"wrong flight", true, false, "rejected"}, {"complete", true, false, "succeeded"}, {"duplicate media", true, false, "succeeded"}, {"unknown counts", false, false, "failed"}, {"finite scope", false, true, "succeeded"}, {"version drift", true, false, "failed"}, {"remote assets", true, false, "succeeded"}, {"version drift assets", true, false, "failed"}, {"revoked", true, false, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			countsKnown = tc.known
			mediaQueries = 0
			changeVersion = strings.HasPrefix(tc.name, "version drift")
			assetMode = strings.HasSuffix(tc.name, "assets")
			duplicateMedia = tc.name == "duplicate media"
			parameters := map[string]any{"mode": "existing-flight", "connectorId": adapter, "flightUuid": "fixture-flight"}
			if tc.name == "wrong flight" {
				parameters["flightUuid"] = "another-flight"
			}
			if tc.confirm {
				parameters["assetIds"] = []int64{asset}
				parameters["confirmLimitedScope"] = true
				parameters["scopeDescription"] = "用户指定的一张夹具图片"
			}
			definition := map[string]any{"apiVersion": "aerosight/v2", "name": tc.name, "trigger": map[string]any{"type": "manual"}, "steps": []any{
				map[string]any{"key": "observe", "uses": "inspection.observe", "with": parameters},
				map[string]any{"key": "detect", "uses": "inspection.detect", "with": map[string]any{"source": "flighthub-ai", "observationId": "steps.observe.outputs.observationId"}},
				map[string]any{"key": "report", "uses": "report.generate"},
			}}
			if tc.name == "external existing flight" {
				step := definition["steps"].([]any)[1].(map[string]any)
				step["with"] = map[string]any{"source": "external", "observationId": "steps.observe.outputs.observationId", "algorithmDefinitionVersionId": algorithmVersion}
			}
			if assetMode {
				definition["steps"] = []any{map[string]any{"key": "observe", "uses": "inspection.observe", "with": map[string]any{"mode": "assets", "assetIds": []int64{asset}}}, map[string]any{"key": "report", "uses": "report.generate"}}
			}
			if tc.name == "product template" {
				definition = productInspectionTemplate(t, "existing-flight")
				for _, raw := range definition["steps"].([]any) {
					step := raw.(map[string]any)
					if step["uses"] == "inspection.observe" {
						step["with"] = parameters
					}
				}
			}
			source, _ := json.Marshal(definition)
			base := fmt.Sprintf("/api/projects/%d/tasks", pid)
			created := call("POST", base, map[string]any{"sourceFormat": "json", "source": string(source), "idempotencyKey": "create-" + tc.name}, 201)
			path := fmt.Sprintf("%s/%.0f", base, created["taskId"])
			if tc.want == "rejected" {
				before := requests
				call("POST", path+"/versions", map[string]any{"action": "publish", "versionId": created["versionId"], "expectedRevision": 1}, 400)
				if requests != before {
					t.Fatal("publication made remote requests")
				}
				return
			}
			call("POST", path+"/versions", map[string]any{"action": "publish", "versionId": created["versionId"], "expectedRevision": 1}, 200)
			call("PATCH", path, map[string]any{"status": "active"}, 200)
			invocation := map[string]any{"type": "manual", "occurredAt": time.Now().UTC().Format(time.RFC3339Nano), "inputs": map[string]any{}, "idempotencyKey": "invoke-" + tc.name}
			accepted := call("POST", path+"/runs", invocation, 201)
			runID := int64(accepted["taskRunId"].(float64))
			initialRequests := requests
			if tc.name == "revoked" {
				exec("delete from team_members where team_id=$1", team)
			}
			var status string
			for n := 0; n < 30; n++ {
				if _, err := consumer.ConsumeOnce(context.Background()); err != nil {
					t.Fatal(err)
				}
				if _, err := modelWorker.ProcessNext(context.Background()); err != nil {
					t.Fatal(err)
				}
				if err := f.db.QueryRow("select status from task_runs where id=$1", runID).Scan(&status); err != nil {
					t.Fatal(err)
				}
				if status == "paused" && tc.name == "product template" {
					var assessmentID string
					var evidenceRaw []byte
					if err := f.db.QueryRow(`select a.id::text,e.evidence_json from inspection_assessments a join inspection_evidence_sets e on e.id=a.evidence_set_id where a.task_run_id=$1`, runID).Scan(&assessmentID, &evidenceRaw); err != nil {
						t.Fatal(err)
					}
					var evidence inspection.EvidenceSet
					if err := json.Unmarshal(evidenceRaw, &evidence); err != nil {
						t.Fatal(err)
					}
					call("POST", fmt.Sprintf("/api/projects/%d/inspection/assessments/%s/review", pid, assessmentID), map[string]any{"expectedRevision": 1, "idempotencyKey": "product-template-review", "decisions": []any{map[string]any{"action": "reject", "reason": "human protocol fixture review", "evidenceRefs": evidence.EvidenceRefs, "missingInformation": []string{}}}}, 200)
				}
				if status == "succeeded" || status == "failed" {
					break
				}
			}
			if status != tc.want {
				var reason []byte
				f.db.QueryRow("select jsonb_agg(jsonb_build_object('status',status,'output',output_snapshot_json,'result',result_json)) from task_run_steps where task_run_id=$1", runID).Scan(&reason)
				t.Fatalf("got %s want %s: %v", status, tc.want, string(reason))
			}
			if tc.name == "product template" {
				if modelCalls != 1 {
					t.Fatal("product template model calls", modelCalls)
				}
				if count := id("select count(*) from generated_reports where project_id=$1 and source_id=$2", pid, fmt.Sprint(runID)); count != 1 {
					t.Fatal("product template report missing", count)
				}
				if count := id("select count(*) from issues where project_id=$1", pid); count != 0 {
					t.Fatal("rejected template wrote case", count)
				}
			}
			if tc.want == "failed" {
				var code string
				if err := f.db.QueryRow("select output_snapshot_json->>'errorCode' from task_run_steps where task_run_id=$1 and status='failed'", runID).Scan(&code); err != nil {
					t.Fatal(err)
				}
				if tc.name == "unknown counts" && code != "INSPECTION_MEDIA_INCOMPLETE_CONFIRM_FINITE_SCOPE" {
					t.Fatal("wrong failure", code)
				}
				if strings.HasPrefix(tc.name, "version drift") && code != "INSPECTION_MEDIA_VERSION_CHANGED" {
					t.Fatal("wrong failure", code)
				}
				var count int
				if err := f.db.QueryRow("select count(*) from inspection_observations where task_run_id=$1", runID).Scan(&count); err != nil || count != 0 {
					t.Fatal("failed step sealed observation")
				}
			}
			if tc.name == "revoked" {
				if requests != initialRequests {
					t.Fatal("revoked delegate contacted provider")
				}
				call("POST", path+"/runs", invocation, 403)
				return
			}
			before := requests
			replay := call("POST", path+"/runs", invocation, 200)
			if replay["taskRunId"] != accepted["taskRunId"] {
				t.Fatal("trigger replay changed Run")
			}
			if _, err := consumer.ConsumeOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
			if requests != before {
				t.Fatal("replay reread media")
			}
			if tc.want == "failed" {
				return
			}
			var manifest, evidence []byte
			if err := f.db.QueryRow("select manifest_json from inspection_observations where task_run_id=$1", runID).Scan(&manifest); err != nil {
				t.Fatal(err)
			}
			var observation inspection.Observation
			if err := json.Unmarshal(manifest, &observation); err != nil {
				t.Fatal(err)
			}
			expected := inspection.Complete
			if tc.confirm {
				expected = inspection.Partial
			}
			sum := sha256.Sum256([]byte(imageBytes))
			if len(observation.Assets) != 1 || observation.Assets[0].ChecksumSHA256 != hex.EncodeToString(sum[:]) {
				t.Fatal("media bytes were not frozen")
			}
			if observation.Completeness != expected || len(observation.Assets) != 1 || *observation.Assets[0].SourceRunID != sourceRun {
				t.Fatalf("bad observation %s", manifest)
			}
			if assetMode {
				if observation.Mode != inspection.Assets || observation.Flight != nil || observation.Assets[0].Flight == nil {
					t.Fatal("remote asset scope lost")
				}
				return
			}
			if err := f.db.QueryRow("select evidence_json from inspection_evidence_sets where task_run_id=$1", runID).Scan(&evidence); err != nil {
				t.Fatal(err)
			}
			var set inspection.EvidenceSet
			if err := json.Unmarshal(evidence, &set); err != nil {
				t.Fatal(err)
			}
			if tc.name == "external existing flight" {
				if algorithmCalls != 1 || set.Completeness != inspection.Complete || len(set.ExternalResults) != 1 || !set.TargetAlgorithmConfirmed {
					t.Fatalf("bad external evidence: %s", evidence)
				}
				return
			}
			if set.Completeness != inspection.Unavailable || set.CanConcludeNoIssue() {
				t.Fatal("zero native alerts inferred no_issue")
			}
		})
	}
	var original int64
	if err := f.db.QueryRow("select task_run_id from assets where id=$1", asset).Scan(&original); err != nil || original != sourceRun {
		t.Fatal("source ownership changed")
	}
}
