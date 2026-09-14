package inspection

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

type ReviewInput struct {
	ExpectedRevision int        `json:"expectedRevision"`
	IdempotencyKey   string     `json:"idempotencyKey"`
	Decisions        []Decision `json:"decisions"`
}
type ReviewResult struct {
	AssessmentID string `json:"assessmentId"`
	Revision     int    `json:"revision"`
	Replayed     bool   `json:"replayed"`
}

// Human rejection records a dismissed proposal; it is not a model claim that
// incomplete imagery proves no_issue. The model validator never accepts reject.
func (a Assessment) ValidateReviewed(e EvidenceSet, allowedIssues map[int64]bool) error {
	copy := a
	copy.Decisions = append([]Decision(nil), a.Decisions...)
	for n, d := range copy.Decisions {
		if d.Action == "reject" {
			if d.IssueID != nil {
				return errors.New("INSPECTION_DECISION_INVALID")
			}
			copy.Decisions[n].Action = "needs_review"
		}
	}
	return copy.Validate(e, allowedIssues)
}

// ReviewAssessment must be invoked in an authorized, audited write transaction.
// Locks the business Run before assessment, matching the worker and dispatcher.
func ReviewAssessment(ctx context.Context, tx *sql.Tx, scope Scope, userID int32, id string, input ReviewInput) (ReviewResult, error) {
	result := ReviewResult{AssessmentID: id}
	if input.ExpectedRevision < 1 || strings.TrimSpace(input.IdempotencyKey) == "" || len(input.IdempotencyKey) > 128 || len(input.Decisions) > 1000 {
		return result, errors.New("INSPECTION_REVIEW_INPUT_INVALID")
	}
	var runID int64
	var runState string
	err := tx.QueryRowContext(ctx, `select run.id,run.status from task_runs run join inspection_assessments a on a.task_run_id=run.id and a.project_id=run.project_id where a.project_id=$1 and a.team_id=$2 and a.id=$3 for update of run`, scope.ProjectID, scope.TeamID, id).Scan(&runID, &runState)
	if err != nil {
		return result, err
	}
	if runState == "canceled" || runState == "canceling" {
		return result, errors.New("INSPECTION_REVIEW_RUN_CANCELED")
	}
	var previousRaw []byte
	var previousUser int32
	err = tx.QueryRowContext(ctx, `select revision,decisions_json,reviewed_by_user_id from inspection_assessment_revisions where assessment_id=$1 and project_id=$2 and idempotency_key=$3 and source='human'`, id, scope.ProjectID, input.IdempotencyKey).Scan(&result.Revision, &previousRaw, &previousUser)
	if err == nil {
		var previous []Decision
		if json.Unmarshal(previousRaw, &previous) != nil || !reflect.DeepEqual(previous, input.Decisions) || previousUser != userID || result.Revision != input.ExpectedRevision+1 {
			return result, errors.New("INSPECTION_REVIEW_IDEMPOTENCY_CONFLICT")
		}
		result.Replayed = true
		return result, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}
	var status string
	var stepID int64
	var evidenceRaw []byte
	var expired bool
	err = tx.QueryRowContext(ctx, `select a.status,a.revision,a.task_run_step_id,e.evidence_json,not exists(select 1 from agent_tool_jobs job where job.project_id=a.project_id and job.args_json->>'assessmentId'=a.id::text and job.context_expires_at>now())
 from inspection_assessments a join inspection_evidence_sets e on e.id=a.evidence_set_id and e.project_id=a.project_id and e.task_run_id=a.task_run_id where a.id=$1 and a.project_id=$2 for update of a`, id, scope.ProjectID).Scan(&status, &result.Revision, &stepID, &evidenceRaw, &expired)
	if err != nil {
		return result, err
	}
	if result.Revision != input.ExpectedRevision {
		return result, errors.New("INSPECTION_REVIEW_REVISION_CONFLICT")
	}
	if status != "needs_review" || runState != "paused" {
		return result, errors.New("INSPECTION_REVIEW_NOT_PENDING")
	}
	if expired {
		return result, errors.New("INSPECTION_REVIEW_EXPIRED")
	}
	var evidence EvidenceSet
	if json.Unmarshal(evidenceRaw, &evidence) != nil {
		return result, errors.New("INSPECTION_EVIDENCE_INVALID")
	}
	allowed := map[int64]bool{}
	for _, decision := range input.Decisions {
		if decision.IssueID != nil {
			var found bool
			if err = tx.QueryRowContext(ctx, `select exists(select 1 from issues where id=$1 and project_id=$2 and status<>'closed')`, *decision.IssueID, scope.ProjectID).Scan(&found); err != nil {
				return result, err
			}
			if !found {
				return result, errors.New("INSPECTION_ISSUE_SCOPE_INVALID")
			}
			allowed[*decision.IssueID] = true
		}
	}
	assessment := Assessment{ID: id, Run: RunRef{Scope: scope, RunID: runID, StepID: stepID}, EvidenceSetID: evidence.ID, Revision: input.ExpectedRevision + 1, Decisions: input.Decisions}
	if err = assessment.ValidateReviewed(evidence, allowed); err != nil {
		return result, err
	}
	if assessment.NeedsReview() {
		return result, errors.New("INSPECTION_REVIEW_DECISIONS_UNRESOLVED")
	}
	raw, err := json.Marshal(input.Decisions)
	if err != nil {
		return result, err
	}
	if _, err = tx.ExecContext(ctx, `insert into inspection_assessment_revisions(assessment_id,project_id,revision,source,decisions_json,reviewed_by_user_id,idempotency_key) values($1,$2,$3,'human',$4,$5,$6)`, id, scope.ProjectID, assessment.Revision, raw, userID, input.IdempotencyKey); err != nil {
		return result, err
	}
	if _, err = tx.ExecContext(ctx, `update inspection_assessments set revision=$3,status='succeeded' where id=$1 and project_id=$2`, id, scope.ProjectID, assessment.Revision); err != nil {
		return result, err
	}
	updated, err := tx.ExecContext(ctx, `update task_run_steps set status='succeeded',finished_at=now() where id=$1 and project_id=$2 and task_run_id=$3 and status='paused'`, stepID, scope.ProjectID, runID)
	if err != nil {
		return result, err
	}
	affected, err := updated.RowsAffected()
	if err != nil {
		return result, err
	}
	if affected != 1 {
		return result, errors.New("INSPECTION_REVIEW_STEP_STATE_INVALID")
	}

	if _, err = tx.ExecContext(ctx, `update task_runs set status='running',state_reason='inspection_review_completed',state_version=state_version+1 where id=$1 and project_id=$2`, runID, scope.ProjectID); err != nil {
		return result, err
	}
	eventID := fmt.Sprintf("inspection-review:%s:%d", id, assessment.Revision)
	payload := map[string]any{"taskRunId": runID, "to": "running", "assessmentId": id, "revision": assessment.Revision}
	if _, err = tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json) values($1,$2,$3,'task_run.transitioned',$4) on conflict(event_id) do nothing`, scope.ProjectID, scope.TeamID, eventID, payload); err != nil {
		return result, err
	}
	result.Revision = assessment.Revision
	return result, nil
}
