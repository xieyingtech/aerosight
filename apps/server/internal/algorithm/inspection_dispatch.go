package algorithm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"aerosight/server/internal/outbox"
	"aerosight/server/internal/tasktrigger"
)

// Lock the business Run before the child algorithm Run, matching aggregation
// and control ordering. Paused jobs stay queued and are reawakened by resume.
func inspectionDispatchAllowed(ctx context.Context, tx *sql.Tx, event outbox.Event, algorithmRunID string) (bool, error) {
	var runID int
	var versionID int64
	var user sql.NullInt32
	var state, stepState, dsl, childState string
	err := tx.QueryRowContext(ctx, `select business.id,coalesce(business.task_version_id,0),business.created_by_user_id,business.status,run_step.status,coalesce(version.dsl_version,''),child.status
 from algorithm_runs child join task_run_steps run_step on run_step.id=child.task_run_step_id and run_step.project_id=child.project_id
 join task_steps step on step.id=run_step.task_step_id and step.project_id=child.project_id and step.uses='inspection.detect'
 join task_runs business on business.id=child.task_run_id and business.project_id=child.project_id and business.id=run_step.task_run_id
 left join task_versions version on version.id=business.task_version_id and version.project_id=business.project_id
 where child.id=$1 and child.project_id=$2 and child.team_id=$3 for update of business`, algorithmRunID, event.ProjectID, event.TeamID).Scan(&runID, &versionID, &user, &state, &stepState, &dsl, &childState)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	} // The ordinary handler checks scope for legacy jobs.
	if err != nil {
		return false, err
	}
	if childState != "queued" {
		return false, nil
	}
	if state == "paused" {
		return false, nil
	}
	if state != "running" && state != "dispatching" || stepState != "running" {
		_, err = tx.ExecContext(ctx, `update algorithm_runs set status='canceled',error_code='INSPECTION_PARENT_NOT_ACTIVE',finished_at=now() where project_id=$1 and id=$2 and status='queued'`, event.ProjectID, algorithmRunID)
		return false, err
	}
	if !user.Valid || dsl != "aerosight/v2" {
		err = errors.New("TASK_TRIGGER_DELEGATE_REQUIRED")
	} else {
		err = tasktrigger.AuthorizeDelegate(ctx, tx, event.ProjectID, versionID, user.Int32)
	}
	if err == nil {
		return true, nil
	}
	if !strings.HasPrefix(err.Error(), "TASK_TRIGGER_DELEGATE") {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `update algorithm_runs set status='failed',error_code='INSPECTION_ALGORITHM_DELEGATE_DENIED',finished_at=now() where project_id=$1 and id=$2 and status='queued'`, event.ProjectID, algorithmRunID); err != nil {
		return false, err
	}
	return false, completeTaskAlgorithmStep(ctx, tx, event.ProjectID, algorithmRunID, "failed", "INSPECTION_ALGORITHM_DELEGATE_DENIED")
}

// ResumeInspectionChildren is called after the normal mission resume handler.
// Existing queued jobs retain their IDs; completed jobs only trigger aggregation.
func ResumeInspectionChildren(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
	var input struct {
		RunID   int    `json:"taskRunId"`
		Control string `json:"control"`
	}
	if err := json.Unmarshal(event.Payload, &input); err != nil {
		return err
	}
	if input.Control != "resume" || input.RunID <= 0 {
		return nil
	}
	var state string
	var version int
	err := tx.QueryRowContext(ctx, `select status,state_version from task_runs where project_id=$1 and team_id=$2 and id=$3 for update`, event.ProjectID, event.TeamID, input.RunID).Scan(&state, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if state != "running" && state != "dispatching" {
		return nil
	}
	_, err = tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json)
 select child.project_id,child.team_id,'inspection-resume:'||child.id::text||':'||$4::int::text,'algorithm.run.requested',jsonb_build_object('runId',child.id)
 from algorithm_runs child join task_run_steps rs on rs.id=child.task_run_step_id and rs.task_run_id=child.task_run_id and rs.project_id=child.project_id
 join task_steps step on step.id=rs.task_step_id and step.project_id=rs.project_id and step.uses='inspection.detect'
 where child.project_id=$1 and child.team_id=$2 and child.task_run_id=$3 and child.status='queued' and rs.status='running' on conflict(event_id) do nothing`, event.ProjectID, event.TeamID, input.RunID, version)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json,max_attempts)
 select rs.project_id,rs.team_id,$4||':'||rs.id::text,'inspection.algorithm.completed',jsonb_build_object('taskRunId',rs.task_run_id,'taskRunStepId',rs.id),greatest(1,coalesce((step.retry_policy_json->>'maxAttempts')::int,1))
 from task_run_steps rs join task_steps step on step.id=rs.task_step_id and step.project_id=rs.project_id and step.uses='inspection.detect'
 where rs.project_id=$1 and rs.team_id=$2 and rs.task_run_id=$3 and rs.status='running' on conflict(event_id) do nothing`, event.ProjectID, event.TeamID, input.RunID, fmt.Sprintf("inspection-resume-aggregate:%d:%d", input.RunID, version))
	return err
}
