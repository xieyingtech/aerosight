package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aerosight/server/internal/algorithm"
	"aerosight/server/internal/credentials"
	"aerosight/server/internal/inspection"
	"aerosight/server/internal/migrations"
	"aerosight/server/internal/mission"
	"aerosight/server/internal/outbox"
	"aerosight/server/internal/testdb"
	"github.com/google/uuid"
)

func TestInspectionAssessmentQueueAndModelWithoutIssue(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if _, err := migrations.Embedded(ctx, db); err != nil {
		t.Fatal(err)
	}
	id := func(q string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := db.QueryRow(q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	uid := id("insert into users(name,email,password) values('assessment','assessment@example.com','unused') returning id")
	team := id("insert into teams(name) values('assessment') returning id")
	exec("insert into team_members(team_id,user_id,role) values($1,$2,'owner')", team, uid)
	project := id("insert into projects(team_id,name) values($1,'assessment') returning id", team)
	task := id("insert into tasks(project_id,team_id,name,trigger_type,script) values($1,$2,'assessment','manual','typed-task-v2') returning id", project, team)
	version := id("insert into task_versions(project_id,team_id,task_id,version,status,script,dsl_version) values($1,$2,$3,1,'published','typed-task-v2','aerosight/v2') returning id", project, team, task)
	run := id("insert into task_runs(project_id,team_id,task_id,task_version_id,trigger_source,status,created_by_user_id) values($1,$2,$3,$4,'manual','running',$5) returning id", project, team, task, version, uid)
	runSteps := []int64{}
	for i, uses := range []string{"inspection.observe", "inspection.detect", "copilot.run"} {
		step := id("insert into task_steps(project_id,team_id,task_version_id,position,step_key,name,action,uses) values($1,$2,$3,$4,$5,$5,$5,$5) returning id", project, team, version, i+1, uses)
		runSteps = append(runSteps, id("insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position,status) values($1,$2,$3,$4,$5,'running') returning id", project, team, run, step, i+1))
	}
	scope := inspection.Scope{ProjectID: int(project), TeamID: int(team)}
	observation := inspection.Observation{ID: uuid.NewString(), ContractVersion: inspection.ContractVersion, Run: inspection.RunRef{Scope: scope, RunID: run, StepID: runSteps[0]}, Mode: inspection.Assets, Completeness: inspection.Complete, ScopeDescription: "protocol fixture only", ObservedFrom: time.Now(), ObservedTo: time.Now(), Assets: []inspection.AssetRef{{Scope: scope, AssetID: 1, Version: 1, ChecksumSHA256: strings.Repeat("a", 64)}}}
	evidence := inspection.EvidenceSet{ID: uuid.NewString(), Run: inspection.RunRef{Scope: scope, RunID: run, StepID: runSteps[1]}, ObservationID: observation.ID, Source: "external", ModelVersion: "fixture-v1", Completeness: inspection.Complete, TargetAlgorithmConfirmed: true, EvidenceRefs: []string{"observation:" + observation.ID}}
	manifest, _ := json.Marshal(observation)
	evidenceRaw, _ := json.Marshal(evidence)
	exec("insert into inspection_observations(id,project_id,team_id,task_run_id,task_run_step_id,source_mode,completeness,scope_description,observed_from,observed_to,manifest_json,sealed_at) values($1,$2,$3,$4,$5,'assets','complete','fixture',now(),now(),$6,now())", observation.ID, project, team, run, runSteps[0], manifest)
	exec("insert into inspection_evidence_sets(id,project_id,team_id,task_run_id,task_run_step_id,observation_id,source,model_version,completeness,target_algorithm_confirmed,evidence_json) values($1,$2,$3,$4,$5,$6,'external','fixture-v1','complete',true,$7)", evidence.ID, project, team, run, runSteps[1], observation.ID, evidenceRaw)
	exec(`update agents set status='active' where project_id=$1 and config_json->>'kind'='copilot'`, project)
	prepared := mission.PreparedStep{ProjectID: int(project), TeamID: int(team), RunID: int(run), StepID: runSteps[2], UserID: int32(uid), Parameters: map[string]any{"mode": "assessment", "evidenceSetId": evidence.ID}}
	for repeat := 0; repeat < 2; repeat++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err = queueInspectionAssessment(ctx, tx, prepared); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"inspection_assessments", "agent_tool_jobs", "agent_sessions"} {
		var count int
		if err := db.QueryRow("select count(*) from " + table).Scan(&count); err != nil || count != 1 {
			t.Fatal(table, count, err)
		}
	}
	var issues int
	if err := db.QueryRow("select count(*) from issues").Scan(&issues); err != nil || issues != 0 {
		t.Fatal("assessment created issue", issues, err)
	}
	prepared.Parameters["evidenceSetId"] = uuid.NewString()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = queueInspectionAssessment(ctx, tx, prepared); err == nil {
		t.Fatal("wrong evidence accepted")
	}
	tx.Rollback()

	responseText := ""
	observedPrompt := ""
	modelCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelCalls++
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("provider credential missing")
		}
		var request struct {
			Messages []map[string]string `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if len(request.Messages) > 0 {
			observedPrompt = request.Messages[0]["content"]
		}
		if len(request.Messages) != 2 || request.Messages[0]["role"] != "system" || request.Messages[1]["role"] != "user" {
			t.Error("evidence and policy roles mixed")
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": responseText}}}})
	}))
	defer server.Close()
	provider := id("insert into ai_providers(name,provider_type,base_url,model_id,credential_envelope_json,enabled,is_default,created_by_user_id,updated_by_user_id) values('fixture','openai',$1,'fixture-model','{}',true,true,$2,$2) returning id", server.URL, uid)
	secret := strings.Repeat("s", 64)
	envelope, err := credentials.EncryptJSON(map[string]string{"apiKey": "test-key"}, secret, credentials.AAD("ai-provider", fmt.Sprint(provider), nil))
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(envelope)
	exec("update ai_providers set credential_envelope_json=$2 where id=$1", provider, encoded)
	processor := JobProcessor{Database: db, AuthSecret: secret, HTTPClient: server.Client()}
	assessment := inspection.Assessment{ID: uuid.NewString(), Run: inspection.RunRef{Scope: scope, RunID: run, StepID: runSteps[2]}, EvidenceSetID: evidence.ID, Revision: 1}
	valid := fmt.Sprintf(`{"decisions":[{"action":"no_issue","reason":"only selected images","evidenceRefs":[%q],"missingInformation":[]}]}`, evidence.EvidenceRefs[0])
	for _, tc := range []struct {
		name, output    string
		incomplete, bad bool
	}{
		{"zero", valid, false, false},
		{"incomplete cannot conclude", valid, true, true},
		{"missing evidence", `{"decisions":[{"action":"no_issue","reason":"x","evidenceRefs":["invented"]}]}`, false, true},
		{"unknown field", `{"decisions":[],"execute":"device.takeoff"}`, false, true},
		{"trailing JSON", valid + ` {}`, false, true},
		{"invalid JSON", "not JSON", false, true},
		{"needs review", fmt.Sprintf(`{"decisions":[{"action":"needs_review","reason":"missing baseline","evidenceRefs":[%q],"missingInformation":["historical imagery"]}]}`, evidence.EvidenceRefs[0]), true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			responseText = tc.output
			input := evidence
			if tc.incomplete {
				input.Completeness = inspection.Partial
				input.TargetAlgorithmConfirmed = false
			}
			result, err := processor.assessEvidence(ctx, assessment, input, observation, nil)
			if (err != nil) != tc.bad {
				t.Fatalf("error=%v wantBad=%v", err, tc.bad)
			}
			if result.RawOutput != tc.output || result.ModelID != "fixture-model" || result.ProviderID != fmt.Sprint(provider) {
				t.Fatal("original model provenance lost")
			}
		})
	}
	// Unknown versions must fail before provider I/O.
	beforeUnknown := modelCalls
	if _, err = processor.assessEvidenceWithPromptVersion(ctx, assessment, evidence, observation, nil, 0.2, "future-version"); err == nil || modelCalls != beforeUnknown {
		t.Fatal("unknown prompt version was executed")
	}
	// Recover a queued v1 job after v2 became the default.
	exec("update inspection_assessments set prompt_version='inspection-assessment-v1'")
	exec(`update agent_tool_jobs set args_json=jsonb_set(args_json,'{promptVersion}','"inspection-assessment-v1"'::jsonb)`)
	// Exercise the registered job consumer on the queued empty-issue Run.
	responseText = valid
	worked, err := processor.ProcessNext(ctx)
	if err != nil || !worked {
		t.Fatal("worker did not execute", worked, err)
	}
	if observedPrompt != inspectionAssessmentInstructionsV1 || strings.Contains(observedPrompt, inspectionAssessmentV2Addition) {
		t.Fatal("queued v1 job used a different prompt")
	}
	var jobState, assessmentState, original string
	if err = db.QueryRow("select status from agent_tool_jobs").Scan(&jobState); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("select status,original_output from inspection_assessments").Scan(&assessmentState, &original); err != nil {
		t.Fatal(err)
	}
	if jobState != "succeeded" || assessmentState != "succeeded" || original != valid {
		t.Fatal("result not persisted", jobState, assessmentState)
	}
	if worked, err = processor.ProcessNext(ctx); err != nil || worked {
		t.Fatal("completed job replayed", worked, err)
	}
	var revisions int
	if err = db.QueryRow("select count(*) from inspection_assessment_revisions").Scan(&revisions); err != nil || revisions != 1 {
		t.Fatal("model revision missing", revisions, err)
	}
	// Each additional job uses a separate step. Resetting this fixture Run's
	// status isolates cancellation/review/error cases without replaying jobs.
	for n, name := range []string{"paused", "canceled", "revoked", "invalid", "prompt-mismatch", "prompt-unknown", "review"} {
		t.Run("worker "+name, func(t *testing.T) {
			exec("update task_runs set status='running',finished_at=null where id=$1", run)
			nextStep := id("insert into task_steps(project_id,team_id,task_version_id,position,step_key,name,action,uses) values($1,$2,$3,$4,$5,$5,'copilot.run','copilot.run') returning id", project, team, version, n+4, name)
			nextRunStep := id("insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position,status) values($1,$2,$3,$4,$5,'running') returning id", project, team, run, nextStep, n+4)
			queued := prepared
			queued.StepID = nextRunStep
			queued.Parameters = map[string]any{"mode": "assessment", "evidenceSetId": evidence.ID}
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err = queueInspectionAssessment(ctx, tx, queued); err != nil {
				tx.Rollback()
				t.Fatal(err)
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			responseText = valid
			beforeCalls := modelCalls
			want := "succeeded"
			if name == "paused" {
				exec("update task_runs set status='paused' where id=$1", run)
				if worked, err := processor.ProcessNext(ctx); err != nil || worked {
					t.Fatal("paused run consumed job", worked, err)
				}
				exec("update task_runs set status='running' where id=$1", run)
			}
			if name == "canceled" {
				want = "canceled"
				exec("update task_runs set status='canceled' where id=$1", run)
			}
			if name == "revoked" {
				want = "failed"
				exec("delete from team_members where team_id=$1 and user_id=$2", team, uid)
			}

			if name == "prompt-mismatch" || name == "prompt-unknown" {
				want = "failed"
				exec("update inspection_assessments set prompt_version='unknown-future' where task_run_step_id=$1", nextRunStep)
				if name == "prompt-unknown" {
					exec(`update agent_tool_jobs set args_json=jsonb_set(args_json,'{promptVersion}','"unknown-future"'::jsonb) where (args_json->>'taskRunStepId')::bigint=$1`, nextRunStep)
				}
			}
			if name == "invalid" {
				want = "failed"
				responseText = "not JSON"
			}
			if name == "review" {
				want = "needs_review"
				responseText = fmt.Sprintf(`{"decisions":[{"action":"needs_review","reason":"needs baseline","evidenceRefs":[%q],"missingInformation":["history"]}]}`, evidence.EvidenceRefs[0])
			}
			if worked, err := processor.ProcessNext(ctx); err != nil || !worked {
				t.Fatal("job not consumed", worked, err)
			}
			if err = db.QueryRow("select status,coalesce(original_output,'') from inspection_assessments where task_run_step_id=$1", nextRunStep).Scan(&assessmentState, &original); err != nil || assessmentState != want {
				t.Fatal("wrong assessment state", assessmentState, want, err)
			}
			if (name == "canceled" || name == "revoked" || strings.HasPrefix(name, "prompt-")) && modelCalls != beforeCalls {
				t.Fatal("inactive or unauthorized job called model")
			}

			if strings.HasPrefix(name, "prompt-") {
				var code string
				if err = db.QueryRow("select failure_code from inspection_assessments where task_run_step_id=$1", nextRunStep).Scan(&code); err != nil {
					t.Fatal(err)
				}
				expected := "INSPECTION_ASSESSMENT_PROMPT_VERSION_MISMATCH"
				if name == "prompt-unknown" {
					expected = "INSPECTION_ASSESSMENT_PROMPT_VERSION_UNSUPPORTED"
				}
				if code != expected || original != "" {
					t.Fatal("prompt failure lost", code, original)
				}
			} else if name != "canceled" && name != "revoked" && observedPrompt != inspectionAssessmentInstructions {
				t.Fatal("new job did not use frozen v2 prompt")
			}
			if name == "invalid" && original != responseText {
				t.Fatal("invalid model original lost")
			}
			if name == "revoked" {
				exec("insert into team_members(team_id,user_id,role) values($1,$2,'owner')", team, uid)
			}
			if name == "review" {
				var state string
				if err = db.QueryRow("select status from task_runs where id=$1", run).Scan(&state); err != nil || state != "paused" {
					t.Fatal("review did not pause batch", state, err)
				}
				tx, err := db.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				payload, _ := json.Marshal(map[string]any{"taskRunId": run, "control": "resume"})
				err = mission.NewProcessor(nil).Handler(ctx, tx, outbox.Event{ProjectID: int(project), TeamID: int(team), EventType: "mission.control", Payload: payload})
				tx.Rollback()
				if err == nil || err.Error() != "INSPECTION_REVIEW_REQUIRED" {
					t.Fatal("normal resume bypassed review", err)
				}
			}
		})
	}

	var reviewID string
	if err = db.QueryRow("select id::text from inspection_assessments where status='needs_review'").Scan(&reviewID); err != nil {
		t.Fatal(err)
	}
	reviewInput := inspection.ReviewInput{ExpectedRevision: 1, IdempotencyKey: "human-reject", Decisions: []inspection.Decision{{Action: "reject", Reason: "human dismissed fixture", EvidenceRefs: evidence.EvidenceRefs, MissingInformation: []string{}}}}
	// A model may never use the human-only reject action.
	modelAssessment := assessment
	modelAssessment.Decisions = reviewInput.Decisions
	if modelAssessment.Validate(evidence, nil) == nil {
		t.Fatal("model accepted human rejection")
	}
	for _, name := range []string{"stale", "canceled", "expired"} {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		input := reviewInput
		if name == "stale" {
			input.ExpectedRevision = 2
		}
		if name == "canceled" {
			if _, err = tx.Exec("update task_runs set status='canceled' where id=$1", run); err != nil {
				t.Fatal(err)
			}
		}
		if name == "expired" {
			if _, err = tx.Exec("update agent_tool_jobs set created_at=now()-interval '2 days',context_expires_at=now()-interval '1 day' where args_json->>'assessmentId'=$1", reviewID); err != nil {
				t.Fatal(err)
			}
		}
		_, err = inspection.ReviewAssessment(ctx, tx, scope, int32(uid), reviewID, input)
		tx.Rollback()
		if err == nil {
			t.Fatal("invalid review accepted", name)
		}
	}
	for repeat := 0; repeat < 2; repeat++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		result, err := inspection.ReviewAssessment(ctx, tx, scope, int32(uid), reviewID, reviewInput)
		if err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if result.Revision != 2 || result.Replayed != (repeat == 1) {
			t.Fatal("wrong review replay", result)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	changed := reviewInput
	changed.Decisions = append([]inspection.Decision(nil), reviewInput.Decisions...)
	changed.Decisions[0].Reason = "changed"
	if _, err = inspection.ReviewAssessment(ctx, tx, scope, int32(uid), reviewID, changed); err == nil || err.Error() != "INSPECTION_REVIEW_IDEMPOTENCY_CONFLICT" {
		t.Fatal("changed replay accepted", err)
	}
	tx.Rollback()
	var modelOriginal string
	if err = db.QueryRow("select original_output from inspection_assessments where id=$1", reviewID).Scan(&modelOriginal); err != nil || modelOriginal != responseText {
		t.Fatal("review rewrote original", err)
	}
	if err = db.QueryRow("select count(*) from outbox_events where event_id like 'inspection-review:%'").Scan(&revisions); err != nil || revisions != 1 {
		t.Fatal("review duplicated continuation", revisions, err)
	}

	t.Run("stable linked issue context", func(t *testing.T) {
		batch := evidence
		batch.ExternalResults = []inspection.ExternalEvidence{{AlgorithmRunID: "fixture-child", Asset: observation.Assets[0], Result: algorithm.CanonicalResult{Kind: algorithm.ResultDetection, Detections: []algorithm.Detection{{DetectionKey: "stable-key", Label: "candidate", Confidence: 0.9}}}}}
		batch.Candidates = []inspection.Candidate{{ID: "external:fixture-child:0", EvidenceRefs: []string{"algorithm:fixture-child"}, Position: inspection.Position{Source: "unknown", Quality: "image-only"}}}
		batch.EvidenceRefs = []string{"algorithm:fixture-child"}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		keys, err := inspection.CandidateSourceKeys(ctx, tx, batch, observation)
		if err != nil {
			t.Fatal(err)
		}
		next := batch
		next.ExternalResults = append([]inspection.ExternalEvidence(nil), batch.ExternalResults...)
		next.ExternalResults[0].AlgorithmRunID = "another-child"
		next.Candidates = []inspection.Candidate{{ID: "external:another-child:0", EvidenceRefs: []string{"algorithm:another-child"}, Position: inspection.Position{Source: "unknown", Quality: "image-only"}}}
		next.EvidenceRefs = []string{"algorithm:another-child"}
		nextKeys, err := inspection.CandidateSourceKeys(ctx, tx, next, observation)
		if err != nil || keys[batch.Candidates[0].ID] != nextKeys[next.Candidates[0].ID] {
			t.Fatal("child run changed source identity", err)
		}
		var issueID int64
		if err = tx.QueryRow("insert into issues(project_id,number,title,source_type) values($1,1,'linked fixture','manual') returning id", project).Scan(&issueID); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec("insert into inspection_issue_sources(project_id,source_key,issue_id,assessment_id) values($1,$2,$3,$4)", project, keys[batch.Candidates[0].ID], issueID, reviewID); err != nil {
			t.Fatal(err)
		}
		linked, err := inspection.LinkedIssues(ctx, tx, batch, observation)
		if err != nil || len(linked) != 1 || linked[0].IssueID != issueID || !linked[0].CanUpdate {
			t.Fatal("linked scope missing", linked, err)
		}
		responseText = fmt.Sprintf(`{"decisions":[{"candidateId":"external:fixture-child:0","action":"update","issueId":%d,"reason":"known source","evidenceRefs":["algorithm:fixture-child"],"missingInformation":[]}]}`, issueID)
		if _, err = processor.assessEvidence(ctx, assessment, batch, observation, map[int64]bool{issueID: true}, linked...); err != nil {
			t.Fatal(err)
		}
		wrong := append([]inspection.LinkedIssue(nil), linked...)
		wrong[0].CandidateID = "other-candidate"
		if _, err = processor.assessEvidence(ctx, assessment, batch, observation, map[int64]bool{issueID: true}, wrong...); err == nil {
			t.Fatal("update crossed candidate association")
		}
		if _, err = tx.Exec("update issues set status='closed' where id=$1", issueID); err != nil {
			t.Fatal(err)
		}
		linked, err = inspection.LinkedIssues(ctx, tx, batch, observation)
		if err != nil || len(linked) != 1 || linked[0].CanUpdate {
			t.Fatal("closed issue is updateable", linked, err)
		}
	})

}
