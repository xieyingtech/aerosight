package mission

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"aerosight/worker/internal/outbox"
)

type failedStepPayload struct {
	TaskRunID     int   `json:"taskRunId"`
	TaskRunStepID int64 `json:"taskRunStepId"`
}

// WithTaskStepFailurePolicy lets transient handler errors use the durable
// outbox retry budget, then applies the published step's abort/pause/continue
// policy on the final attempt instead of leaving the run stuck in running.
func WithTaskStepFailurePolicy(handler outbox.Handler) outbox.Handler {
	return func(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
		if _, err := tx.ExecContext(ctx, "savepoint task_step_handler"); err != nil {
			return err
		}
		if err := handler(ctx, tx, event); err == nil {
			_, releaseErr := tx.ExecContext(ctx, "release savepoint task_step_handler")
			return releaseErr
		} else if event.Attempts < event.MaxAttempts {
			return err
		} else {
			if _, rollbackErr := tx.ExecContext(ctx, "rollback to savepoint task_step_handler"); rollbackErr != nil {
				return errors.Join(err, rollbackErr)
			}
			return finalizeTaskStepFailure(ctx, tx, event, err)
		}
	}
}

func finalizeTaskStepFailure(ctx context.Context, tx *sql.Tx, event outbox.Event, cause error) error {
	var payload failedStepPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.TaskRunID <= 0 || payload.TaskRunStepID <= 0 {
		return errors.New("TASK_STEP_FAILURE_PAYLOAD_INVALID")
	}
	var onFailure string
	if err := tx.QueryRowContext(ctx, `select coalesce(step.failure_policy_json->>'onFailure','abort')
		from task_run_steps run_step join task_steps step on step.id=run_step.task_step_id and step.project_id=run_step.project_id
		where run_step.project_id=$1 and run_step.team_id=$2 and run_step.id=$3 and run_step.task_run_id=$4 for update of run_step`,
		event.ProjectID, event.TeamID, payload.TaskRunStepID, payload.TaskRunID).Scan(&onFailure); err != nil {
		return err
	}
	code := stableFailureCode(cause)
	stepStatus, runStatus, reason := "failed", "failed", "task_step_failed"
	if onFailure == "pause" {
		stepStatus, runStatus, reason = "paused", "paused", "task_step_paused_after_retries"
	} else if onFailure == "continue" {
		stepStatus, runStatus, reason = "skipped", "running", "task_step_failure_continued"
	}
	output := map[string]any{"errorCode": code, "attempts": event.Attempts, "continued": onFailure == "continue"}
	if _, err := tx.ExecContext(ctx, `update task_run_steps set status=$3,attempt_count=greatest(attempt_count,$4),
		output_snapshot_json=$5,result_json=result_json||$5,finished_at=now() where project_id=$1 and id=$2`,
		event.ProjectID, payload.TaskRunStepID, stepStatus, event.Attempts, output); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `update task_runs set status=$3,state_version=state_version+1,state_reason=$4,
		finished_at=case when $3='failed' then now() else finished_at end where project_id=$1 and id=$2`,
		event.ProjectID, payload.TaskRunID, runStatus, reason); err != nil {
		return err
	}
	eventID := fmt.Sprintf("task-run:%d:step:%d:%s", payload.TaskRunID, payload.TaskRunStepID, stepStatus)
	nextPayload := map[string]any{"taskRunId": payload.TaskRunID, "to": runStatus, "failedStepId": payload.TaskRunStepID,
		"failureCode": code, "reason": reason}
	if _, err := tx.ExecContext(ctx, `insert into project_events(project_id,team_id,event_id,event_type,payload_json)
		values($1,$2,$3,'task_run.transitioned',$4) on conflict(event_id) do nothing`, event.ProjectID, event.TeamID, eventID, nextPayload); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json)
		values($1,$2,$3,'task_run.transitioned',$4) on conflict(event_id) do nothing`, event.ProjectID, event.TeamID, eventID, nextPayload)
	return err
}

func stableFailureCode(err error) string {
	value := strings.SplitN(err.Error(), ":", 2)[0]
	if value == "" || len(value) > 96 {
		return "TASK_STEP_EXECUTION_FAILED"
	}
	for _, character := range value {
		if character != '_' && !unicode.IsUpper(character) && !unicode.IsDigit(character) {
			return "TASK_STEP_EXECUTION_FAILED"
		}
	}
	return value
}
