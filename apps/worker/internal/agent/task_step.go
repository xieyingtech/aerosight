package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"aerosight/worker/internal/orchestration"
	"aerosight/worker/internal/outbox"
)

type copilotStepPayload struct {
	TaskRunID     int   `json:"taskRunId"`
	TaskRunStepID int64 `json:"taskRunStepId"`
}

// TaskStepHandler turns an explicit copilot.run step into the same protected,
// revalidated job used by issue mentions and assignments.
func TaskStepHandler(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
	var payload copilotStepPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.TaskRunID <= 0 || payload.TaskRunStepID <= 0 {
		return errors.New("TASK_COPILOT_PAYLOAD_INVALID")
	}
	var userID, issueID, copilotID int
	var status string
	var rawParameters, rawInputs []byte
	err := tx.QueryRowContext(ctx, `select coalesce(run.created_by_user_id,task.created_by_user_id,project.created_by_user_id),run_step.status,
		step.parameters_json,run.input_snapshot_json,
		(select agent.id from agents agent where agent.project_id=run.project_id and agent.status='active'
		  and agent.config_json->>'kind'='copilot' limit 1)
		from task_run_steps run_step join task_runs run on run.id=run_step.task_run_id and run.project_id=run_step.project_id
		join tasks task on task.id=run.task_id and task.project_id=run.project_id
		join projects project on project.id=run.project_id
		join task_steps step on step.id=run_step.task_step_id and step.project_id=run_step.project_id and step.uses='copilot.run'
		where run_step.project_id=$1 and run_step.team_id=$2 and run_step.id=$3 and run.id=$4`,
		event.ProjectID, event.TeamID, payload.TaskRunStepID, payload.TaskRunID).Scan(&userID, &status, &rawParameters, &rawInputs, &copilotID)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("TASK_COPILOT_STEP_SCOPE_INVALID")
	}
	if err != nil {
		return err
	}
	if status == "succeeded" || status == "skipped" {
		return nil
	}
	parameters := map[string]any{}
	inputs := map[string]any{}
	if err := json.Unmarshal(rawParameters, &parameters); err != nil || json.Unmarshal(rawInputs, &inputs) != nil {
		return errors.New("TASK_COPILOT_INPUT_INVALID")
	}
	if nested, ok := inputs["inputs"].(map[string]any); ok {
		inputs = nested
	}
	outputs := map[string]map[string]any{}
	rows, err := tx.QueryContext(ctx, `select step.step_key,run_step.output_snapshot_json
		from task_run_steps run_step join task_steps step on step.id=run_step.task_step_id and step.project_id=run_step.project_id
		where run_step.project_id=$1 and run_step.task_run_id=$2 and run_step.position <
		(select position from task_run_steps where project_id=$1 and id=$3) and run_step.status in('succeeded','skipped')`,
		event.ProjectID, payload.TaskRunID, payload.TaskRunStepID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			rows.Close()
			return err
		}
		var output map[string]any
		if err := json.Unmarshal(raw, &output); err != nil {
			rows.Close()
			return errors.New("TASK_COPILOT_STEP_OUTPUT_INVALID")
		}
		outputs[key] = output
	}
	if err := rows.Close(); err != nil {
		return err
	}
	resolved, err := orchestration.ResolveReferences(parameters, orchestration.Context{Inputs: inputs, Steps: outputs})
	if err != nil {
		return err
	}
	resolvedJSON, _ := json.Marshal(resolved)
	var resolvedParameters struct {
		IssueID int `json:"issueId"`
	}
	if err := json.Unmarshal(resolvedJSON, &resolvedParameters); err != nil {
		return errors.New("TASK_COPILOT_INPUT_INVALID")
	}
	issueID = resolvedParameters.IssueID
	if issueID == 0 {
		if err := tx.QueryRowContext(ctx, `select coalesce((select issue.id from issues issue where issue.project_id=$1 and issue.task_run_id=$2 order by issue.id desc limit 1),0)`,
			event.ProjectID, payload.TaskRunID).Scan(&issueID); err != nil {
			return err
		}
	}
	if issueID == 0 {
		return errors.New("TASK_COPILOT_ISSUE_REQUIRED")
	}
	var allowed bool
	if err := tx.QueryRowContext(ctx, `select exists(select 1 from team_members member
		join projects project on project.team_id=member.team_id and project.id=$1
		left join project_permissions permission on permission.project_id=project.id and permission.team_id=project.team_id
		 and permission.user_id=member.user_id and permission.permission='agent:use'
		where member.user_id=$2 and (member.role in('owner','admin') or permission.permission is not null))`, event.ProjectID, userID).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return errors.New("TASK_COPILOT_PERMISSION_DENIED")
	}
	var sessionID int
	if err := tx.QueryRowContext(ctx, `insert into agent_sessions(project_id,agent_id,task_run_id,issue_id,started_by_user_id,summary)
		values($1,$2,$3,$4,$5,$6) returning id`, event.ProjectID, copilotID, payload.TaskRunID, issueID, userID,
		fmt.Sprintf("Copilot · Task Run #%d", payload.TaskRunID)).Scan(&sessionID); err != nil {
		return err
	}
	idempotencyKey := fmt.Sprintf("task-copilot:%d", payload.TaskRunStepID)
	var jobID string
	if err := tx.QueryRowContext(ctx, `insert into agent_tool_jobs(project_id,team_id,session_id,requested_by_user_id,issue_id,
		trigger_type,idempotency_key,tool_name,required_permission,args_json,context_expires_at)
		values($1,$2,$3,$4,$5,'task_step',$6,'issue_copilot','agent:use',
		jsonb_build_object('issueId',$5::int,'taskRunId',$7::int,'taskRunStepId',$8::bigint),now()+interval '24 hours')
		on conflict(project_id,idempotency_key) where idempotency_key is not null do update set idempotency_key=excluded.idempotency_key
		returning id`, event.ProjectID, event.TeamID, sessionID, userID, issueID, idempotencyKey, payload.TaskRunID, payload.TaskRunStepID).Scan(&jobID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `insert into issue_events(project_id,issue_id,event_type,metadata_json)
		values($1,$2,'copilot.requested',jsonb_build_object('jobId',$3::text,'sessionId',$4::int,'triggerType','task_step','taskRunStepId',$5::bigint))`,
		event.ProjectID, issueID, jobID, sessionID, payload.TaskRunStepID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `update task_run_steps set status='running',attempt_count=greatest(attempt_count,1),started_at=coalesce(started_at,now())
		where project_id=$1 and id=$2`, event.ProjectID, payload.TaskRunStepID)
	return err
}
