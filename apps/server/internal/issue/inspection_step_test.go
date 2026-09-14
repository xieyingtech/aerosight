package issue

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"aerosight/server/internal/algorithm"
	"aerosight/server/internal/inspection"
	"aerosight/server/internal/migrations"
	"aerosight/server/internal/mission"
	"aerosight/server/internal/testdb"
	"github.com/google/uuid"
)

func TestInspectionIssueConcurrencyUpdateAndReview(t *testing.T) {
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
	uid := id("insert into users(name,email,password) values('inspection','issue-inspection@example.com','unused') returning id")
	team := id("insert into teams(name) values('issue inspection') returning id")
	project := id("insert into projects(team_id,name) values($1,'issue inspection') returning id", team)
	assetID := id("insert into assets(project_id,team_id,kind,mime_type,storage_key,logical_key) values($1,$2,'image','image/jpeg','fixture','fixture') returning id", project, team)
	scope := inspection.Scope{ProjectID: int(project), TeamID: int(team)}
	fixture := func(key, source, action string, target *int64) mission.PreparedStep {
		task := id("insert into tasks(project_id,team_id,name,trigger_type,script) values($1,$2,'fixture '||gen_random_uuid(),'manual','typed-task-v2') returning id", project, team)
		version := id("insert into task_versions(project_id,team_id,task_id,version,status,script,dsl_version) values($1,$2,$3,1,'published','typed-task-v2','aerosight/v2') returning id", project, team, task)
		run := id("insert into task_runs(project_id,team_id,task_id,task_version_id,trigger_source,status,created_by_user_id) values($1,$2,$3,$4,'manual','running',$5) returning id", project, team, task, version, uid)
		steps := []int64{}
		for n, uses := range []string{"inspection.observe", "inspection.detect", "copilot.run", "issue.create-or-update"} {
			step := id("insert into task_steps(project_id,team_id,task_version_id,position,step_key,name,action,uses) values($1,$2,$3,$4,$5,$5,$5,$5) returning id", project, team, version, n+1, uses)
			status := "succeeded"
			if n == 3 {
				status = "running"
			}
			steps = append(steps, id("insert into task_run_steps(project_id,team_id,task_run_id,task_step_id,position,status) values($1,$2,$3,$4,$5,$6) returning id", project, team, run, step, n+1, status))
		}
		observation := inspection.Observation{ID: uuid.NewString(), ContractVersion: inspection.ContractVersion, Run: inspection.RunRef{Scope: scope, RunID: run, StepID: steps[0]}, Mode: inspection.Assets, Completeness: inspection.Complete, ScopeDescription: "fixture", ObservedFrom: time.Now(), ObservedTo: time.Now(), Assets: []inspection.AssetRef{{Scope: scope, AssetID: assetID, Version: 1, ChecksumSHA256: strings.Repeat("a", 64)}}}
		child := uuid.NewString()
		ref := "algorithm:" + child
		candidate := "external:" + child + ":0"
		evidence := inspection.EvidenceSet{ID: uuid.NewString(), Run: inspection.RunRef{Scope: scope, RunID: run, StepID: steps[1]}, ObservationID: observation.ID, Source: "external", ModelVersion: "fixture", Completeness: inspection.Complete, TargetAlgorithmConfirmed: true, EvidenceRefs: []string{ref}, Candidates: []inspection.Candidate{{ID: candidate, EvidenceRefs: []string{ref}, Position: inspection.Position{Source: "unknown", Quality: "image-only"}}}, ExternalResults: []inspection.ExternalEvidence{{Ref: ref, AlgorithmRunID: child, Asset: observation.Assets[0], Result: algorithm.CanonicalResult{Kind: algorithm.ResultDetection, Detections: []algorithm.Detection{{DetectionKey: key, Label: "candidate", Confidence: 0.9}}}}}}
		manifest, _ := json.Marshal(observation)
		raw, _ := json.Marshal(evidence)
		exec("insert into inspection_observations(id,project_id,team_id,task_run_id,task_run_step_id,source_mode,completeness,scope_description,observed_from,observed_to,manifest_json,sealed_at) values($1,$2,$3,$4,$5,'assets','complete','fixture',now(),now(),$6,now())", observation.ID, project, team, run, steps[0], manifest)
		exec("insert into inspection_evidence_sets(id,project_id,team_id,task_run_id,task_run_step_id,observation_id,source,model_version,completeness,target_algorithm_confirmed,evidence_json) values($1,$2,$3,$4,$5,$6,'external','fixture','complete',true,$7)", evidence.ID, project, team, run, steps[1], observation.ID, raw)
		assessmentID := uuid.NewString()
		exec("insert into inspection_assessments(id,project_id,team_id,task_run_id,task_run_step_id,evidence_set_id,status,revision) values($1,$2,$3,$4,$5,$6,'succeeded',1)", assessmentID, project, team, run, steps[2], evidence.ID)
		decisions, _ := json.Marshal([]inspection.Decision{{CandidateID: candidate, Action: action, Reason: "fixture reason", EvidenceRefs: []string{ref}, IssueID: target}})
		var reviewer any
		if source == "human" {
			reviewer = uid
		}
		exec("insert into inspection_assessment_revisions(assessment_id,project_id,revision,source,decisions_json,reviewed_by_user_id,idempotency_key) values($1,$2,1,$3,$4,$5,'fixture')", assessmentID, project, source, decisions, reviewer)
		return mission.PreparedStep{ProjectID: int(project), TeamID: int(team), RunID: int(run), StepID: steps[3], UserID: int32(uid), Parameters: map[string]any{"assessmentId": assessmentID}}
	}
	processor := NewTaskStepProcessor(nil)
	invoke := func(step mission.PreparedStep) error {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		// Same lock held by the real mission execution boundary.
		if _, err = tx.ExecContext(ctx, "select id from task_runs where id=$1 for update", step.RunID); err != nil {
			return err
		}
		if err = processor.inspectionAssessment(ctx, tx, step); err != nil {
			return err
		}
		return tx.Commit()
	}
	first, second := fixture("same-detection", "model", "create", nil), fixture("same-detection", "model", "create", nil)
	ready := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, step := range []mission.PreparedStep{first, second} {
		workers.Add(1)
		go func(step mission.PreparedStep) { defer workers.Done(); <-ready; results <- invoke(step) }(step)
	}
	close(ready)
	workers.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	var issueID int64
	var count, total int
	if err := db.QueryRow("select min(id),count(*),sum(occurrence_count) from issues where project_id=$1", project).Scan(&issueID, &count, &total); err != nil || count != 1 || total != 1 {
		t.Fatal("concurrent duplicate", count, total, err)
	}
	if err := db.QueryRow("select count(*) from issue_events where project_id=$1", project).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate activity", count, err)
	}
	update := fixture("new-detection", "human", "update", &issueID)
	if err := invoke(update); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("select occurrence_count from issues where id=$1", issueID).Scan(&total); err != nil || total != 2 {
		t.Fatal("human update missing", total, err)
	}
	// Another business Run with the same new source must not count it again.
	if err := invoke(fixture("new-detection", "human", "update", &issueID)); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("select occurrence_count from issues where id=$1", issueID).Scan(&total); err != nil || total != 2 {
		t.Fatal("source counted twice", total, err)
	}
	for _, name := range []string{"ambiguous model update", "closed human update"} {
		source := "model"
		if name == "closed human update" {
			source = "human"
			exec("update issues set status='closed' where id=$1", issueID)
		}
		step := fixture(name, source, "update", &issueID)
		if err := invoke(step); err != nil {
			t.Fatal(err)
		}
		var state string
		if err := db.QueryRow("select status from task_runs where id=$1", step.RunID).Scan(&state); err != nil || state != "paused" {
			t.Fatal("unsafe target did not pause", state, err)
		}
		if err := db.QueryRow("select status from inspection_assessments where task_run_id=$1", step.RunID).Scan(&state); err != nil || state != "needs_review" {
			t.Fatal("review context missing", state, err)
		}
		if err := db.QueryRow("select occurrence_count from issues where id=$1", issueID).Scan(&total); err != nil || total != 2 {
			t.Fatal("review changed case", total, err)
		}
	}
}
