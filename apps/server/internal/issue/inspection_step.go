package issue

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"aerosight/server/internal/inspection"
	"aerosight/server/internal/mission"
	"github.com/google/uuid"
)

func (processor *TaskStepProcessor) inspectionAssessment(ctx context.Context, tx *sql.Tx, step mission.PreparedStep) error {
	var input struct {
		AssessmentID string `json:"assessmentId"`
		Title        string `json:"title"`
		Priority     string `json:"priority"`
	}
	raw, _ := json.Marshal(step.Parameters)
	if json.Unmarshal(raw, &input) != nil {
		return errors.New("INSPECTION_ISSUE_INPUT_INVALID")
	}
	if _, err := uuid.Parse(input.AssessmentID); err != nil {
		return errors.New("INSPECTION_ASSESSMENT_SCOPE_INVALID")
	}
	if input.Title == "" {
		input.Title = "巡检待核实线索"
	}
	if len(input.Title) > 300 || strings.TrimSpace(input.Title) == "" {
		return errors.New("INSPECTION_ISSUE_INPUT_INVALID")
	}
	if input.Priority == "" {
		input.Priority = "medium"
	}
	if input.Priority != "low" && input.Priority != "medium" && input.Priority != "high" && input.Priority != "critical" {
		return errors.New("INSPECTION_ISSUE_INPUT_INVALID")
	}
	var assessmentStep int64
	var status, source string
	var revision int
	var evidenceRaw, observationRaw, decisionRaw []byte
	err := tx.QueryRowContext(ctx, `select a.task_run_step_id,a.status,a.revision,r.source,r.decisions_json,e.evidence_json,o.manifest_json
 from inspection_assessments a join inspection_assessment_revisions r on r.assessment_id=a.id and r.project_id=a.project_id and r.revision=a.revision
 join inspection_evidence_sets e on e.id=a.evidence_set_id and e.project_id=a.project_id and e.task_run_id=a.task_run_id
 join inspection_observations o on o.id=e.observation_id and o.project_id=e.project_id and o.task_run_id=e.task_run_id
 where a.id=$1 and a.project_id=$2 and a.team_id=$3 and a.task_run_id=$4`, input.AssessmentID, step.ProjectID, step.TeamID, step.RunID).Scan(&assessmentStep, &status, &revision, &source, &decisionRaw, &evidenceRaw, &observationRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("INSPECTION_ASSESSMENT_SCOPE_INVALID")
	}
	if err != nil {
		return err
	}
	if status != "succeeded" {
		return errors.New("INSPECTION_ASSESSMENT_NOT_COMPLETE")
	}
	var evidence inspection.EvidenceSet
	var observation inspection.Observation
	var decisions []inspection.Decision
	if json.Unmarshal(evidenceRaw, &evidence) != nil || json.Unmarshal(observationRaw, &observation) != nil || json.Unmarshal(decisionRaw, &decisions) != nil {
		return errors.New("INSPECTION_ASSESSMENT_INVALID")
	}
	keys, err := inspection.CandidateSourceKeys(ctx, tx, evidence, observation)
	if err != nil {
		return err
	}
	// Serialize source resolution within a project, then take the existing shared
	// issue-number lock when allocating. All preflight checks precede issue writes.
	if _, err = tx.ExecContext(ctx, "select pg_advisory_xact_lock(hashtextextended($1,0))", fmt.Sprintf("inspection-issues:%d", step.ProjectID)); err != nil {
		return err
	}
	allowed := map[int64]bool{}
	for _, d := range decisions {
		if d.IssueID != nil {
			var found bool
			if err = tx.QueryRowContext(ctx, "select exists(select 1 from issues where id=$1 and project_id=$2)", *d.IssueID, step.ProjectID).Scan(&found); err != nil {
				return err
			}
			allowed[*d.IssueID] = found
		}
	}
	assessment := inspection.Assessment{ID: input.AssessmentID, Run: inspection.RunRef{Scope: evidence.Run.Scope, RunID: int64(step.RunID), StepID: assessmentStep}, EvidenceSetID: evidence.ID, Revision: revision, Decisions: decisions}
	if source == "human" {
		err = assessment.ValidateReviewed(evidence, allowed)
	} else {
		err = assessment.Validate(evidence, allowed)
	}
	if err != nil {
		return err
	}
	if assessment.NeedsReview() {
		return errors.New("INSPECTION_ASSESSMENT_NOT_COMPLETE")
	}
	existing := map[string]int64{}
	reviewNeeded := false
	for _, d := range decisions {
		if d.Action != "create" && d.Action != "update" {
			continue
		}
		key := keys[d.CandidateID]
		var linked int64
		err = tx.QueryRowContext(ctx, "select issue_id from inspection_issue_sources where project_id=$1 and source_key=$2", step.ProjectID, key).Scan(&linked)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			existing[key] = linked
			if d.IssueID != nil && *d.IssueID != linked {
				reviewNeeded = true
			}
			continue
		}
		if d.Action == "update" {
			var issueStatus string
			if err = tx.QueryRowContext(ctx, "select status from issues where project_id=$1 and id=$2 for update", step.ProjectID, *d.IssueID).Scan(&issueStatus); err != nil {
				return err
			}
			// New observations may only target an unassociated issue after human review.
			if issueStatus == "closed" || source != "human" {
				reviewNeeded = true
			}
		}
	}
	if reviewNeeded {
		if _, err = tx.ExecContext(ctx, "update inspection_assessments set status='needs_review' where id=$1 and project_id=$2", input.AssessmentID, step.ProjectID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "update task_run_steps set status='paused',finished_at=null where id=$1 and project_id=$2", assessmentStep, step.ProjectID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "update task_run_steps set status='pending',started_at=null where id=$1 and project_id=$2", step.StepID, step.ProjectID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "update task_runs set status='paused',state_reason='inspection_issue_target_needs_review',state_version=state_version+1 where id=$1 and project_id=$2", step.RunID, step.ProjectID)
		return err
	}
	ids := []int64{}
	seen := map[int64]bool{}
	for _, d := range decisions {
		if d.Action != "create" && d.Action != "update" {
			continue
		}
		key := keys[d.CandidateID]
		issueID, exists := existing[key]
		if !exists {
			kind := "issue.created"
			if d.Action == "update" {
				issueID = *d.IssueID
				kind = "issue.updated"
				_, err = tx.ExecContext(ctx, "update issues set occurrence_count=occurrence_count+1,last_seen_at=now(),updated_at=now(),state_version=state_version+1 where id=$1 and project_id=$2", issueID, step.ProjectID)
			} else {
				if _, err = tx.ExecContext(ctx, "select pg_advisory_xact_lock(hashtextextended($1,0))", fmt.Sprintf("issue-number:%d", step.ProjectID)); err != nil {
					return err
				}
				err = tx.QueryRowContext(ctx, `insert into issues(project_id,number,title,description,source_type,source_id,task_run_id,priority,opened_by_user_id) select $1,coalesce(max(number),0)+1,$2,$3,'task',$4,$4,$5,$6 from issues where project_id=$1 returning id`, step.ProjectID, input.Title, d.Reason, step.RunID, input.Priority, step.UserID).Scan(&issueID)
			}
			if err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, "insert into inspection_issue_sources(project_id,source_key,issue_id,assessment_id) values($1,$2,$3,$4)", step.ProjectID, key, issueID, input.AssessmentID); err != nil {
				return err
			}
			metadata := map[string]any{"assessmentId": input.AssessmentID, "revision": revision, "taskRunId": step.RunID, "taskRunStepId": step.StepID, "sourceKey": key}
			if _, err = tx.ExecContext(ctx, "insert into issue_events(project_id,issue_id,event_type,metadata_json) values($1,$2,$3,$4)", step.ProjectID, issueID, kind, metadata); err != nil {
				return err
			}
			eventID := "inspection-issue:" + key
			if _, err = tx.ExecContext(ctx, "insert into project_events(project_id,team_id,event_id,event_type,payload_json) values($1,$2,$3,$4,$5) on conflict(event_id) do nothing", step.ProjectID, step.TeamID, eventID, kind, map[string]any{"issueId": issueID, "assessmentId": input.AssessmentID}); err != nil {
				return err
			}
			existing[key] = issueID
		}
		links := map[string][]string{"inspection_assessment": {input.AssessmentID}, "inspection_evidence_set": {evidence.ID}, "inspection_observation": {observation.ID}, "task_run": {strconv.Itoa(step.RunID)}, "inspection_evidence": d.EvidenceRefs}
		for _, item := range evidence.ExternalResults {
			for _, ref := range d.EvidenceRefs {
				if ref == item.Ref {
					links["asset"] = append(links["asset"], strconv.FormatInt(item.Asset.AssetID, 10))
					links["algorithm_run"] = append(links["algorithm_run"], item.AlgorithmRunID)
				}
			}
		}
		for kind, targets := range links {
			for _, target := range targets {
				if _, err = tx.ExecContext(ctx, "insert into issue_links(project_id,issue_id,link_type,target_id,created_by_user_id) values($1,$2,$3,$4,$5) on conflict(issue_id,link_type,target_id) do nothing", step.ProjectID, issueID, kind, target, step.UserID); err != nil {
					return err
				}
			}
		}
		if !seen[issueID] {
			ids = append(ids, issueID)
			seen[issueID] = true
		}
	}
	output, _ := json.Marshal(map[string]any{"issueIds": ids})
	auditInput, _ := json.Marshal(map[string]any{"assessmentId": input.AssessmentID, "revision": revision, "parameters": step.Parameters})
	inputDigest, resultDigest := sha256.Sum256(auditInput), sha256.Sum256(output)
	if _, err = tx.ExecContext(ctx, `insert into audit_events(project_id,team_id,request_id,actor_user_id,action,resource_type,resource_id,input_hash,result_hash,policy_result_json,status,completed_at) values($1,$2,$3,$4,'inspection.issues.apply','inspection_assessment',$5,$6,$7,'{"permission":"issue:handle","assessmentRequired":true}','completed',now())`, step.ProjectID, step.TeamID, fmt.Sprintf("inspection-issue-step:%d", step.StepID), step.UserID, input.AssessmentID, hex.EncodeToString(inputDigest[:]), hex.EncodeToString(resultDigest[:])); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, "update task_run_steps set status='succeeded',output_snapshot_json=$3,result_json=result_json||$3,finished_at=now() where project_id=$1 and id=$2", step.ProjectID, step.StepID, output); err != nil {
		return err
	}
	return enqueueContinuation(ctx, tx, taskStepRecord{ProjectID: step.ProjectID, TeamID: step.TeamID, TaskRunID: step.RunID, TaskRunStepID: step.StepID}, "inspection-assessment")
}
