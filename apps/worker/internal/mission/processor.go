package mission

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"aerosight/worker/internal/observability"
	"aerosight/worker/internal/orchestration"
	"aerosight/worker/internal/outbox"
)

type eventPayload struct {
	TaskRunID int           `json:"taskRunId"`
	CommandID string        `json:"commandId"`
	Outcome   string        `json:"outcome"`
	Control   ControlAction `json:"control"`
	To        RunStatus     `json:"to"`
}

type Processor struct {
	now func() time.Time
}

func NewProcessor(now func() time.Time) *Processor {
	if now == nil {
		now = time.Now
	}
	return &Processor{now: now}
}

func (processor *Processor) Handler(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
	var payload eventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("decode mission event: %w", err)
	}
	if event.EventType == "task_run.transitioned" && payload.To != RunDispatching && payload.To != RunRunning {
		return nil
	}
	if payload.TaskRunID == 0 && payload.CommandID != "" {
		var taskRunID sql.NullInt64
		if err := tx.QueryRowContext(ctx,
			"select task_run_id from device_commands where project_id = $1 and id::text = $2",
			event.ProjectID, payload.CommandID).Scan(&taskRunID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}
		if !taskRunID.Valid {
			return nil
		}
		payload.TaskRunID = int(taskRunID.Int64)
	}
	if payload.TaskRunID <= 0 {
		return errors.New("mission event is missing taskRunId")
	}
	snapshot, version, err := loadSnapshot(ctx, tx, event.ProjectID, payload.TaskRunID)
	if err != nil {
		return err
	}
	now := processor.now()
	var decision Decision
	if payload.Control != "" {
		decision, err = Control(snapshot, payload.Control, now)
	} else {
		var signal *Signal
		if payload.CommandID != "" {
			signal = &Signal{CommandID: payload.CommandID, Outcome: payload.Outcome}
		}
		decision, err = Advance(snapshot, signal, now)
	}
	if err != nil {
		return err
	}
	if decision.Reason == "awaiting_ack" || decision.Reason == "awaiting_step_result" || decision.Reason == "unknown_ack_ignored" {
		return nil
	}
	return applyDecision(ctx, tx, event.ProjectID, event.TeamID, snapshot, version, decision, now)
}

func loadSnapshot(ctx context.Context, tx *sql.Tx, projectID, runID int) (Snapshot, int, error) {
	var snapshot Snapshot
	var version int
	var rawInputs []byte
	err := tx.QueryRowContext(ctx, `
		select run.id, run.status, run.state_version,run.input_snapshot_json,
		       coalesce(device.status in ('online','degraded'),false) as connected,
		       exists(select 1 from device_capabilities capability
		               where capability.device_id = device.id and capability.capability_code = 'flight.return_home')
		  from task_runs run left join devices device on device.id = run.selected_device_id and device.project_id = run.project_id
		 where run.project_id = $1 and run.id = $2 for update of run`, projectID, runID,
	).Scan(&snapshot.RunID, &snapshot.Status, &version, &rawInputs, &snapshot.DeviceConnected, &snapshot.SupportsReturnHome)
	if err != nil {
		return Snapshot{}, 0, err
	}
	inputs := map[string]any{}
	if err := json.Unmarshal(rawInputs, &inputs); err != nil {
		return Snapshot{}, 0, errors.New("TASK_RUN_INPUT_INVALID")
	}
	if nested, ok := inputs["inputs"].(map[string]any); ok {
		inputs = nested
	}
	stepOutputs := map[string]map[string]any{}
	rows, err := tx.QueryContext(ctx, `
		select run_step.id,run_step.position,run_step.status,step.step_key,step.uses,step.capability_code,step.action,step.parameters_json,
		       greatest(0,coalesce((step.retry_policy_json->>'maxAttempts')::int-1,(step.failure_policy_json->>'maxRetries')::int,0)),
		       greatest(0,coalesce((step.retry_policy_json->>'backoffSeconds')::int,(step.failure_policy_json->>'retryBackoffSeconds')::int,1)),
		       step.timeout_seconds,
		       coalesce(step.failure_policy_json->>'idempotency', 'unsafe') = 'safe',
		       coalesce(step.failure_policy_json->>'onFailure', 'abort') = 'pause',
		       command.id::text, command.status, command.deadline_at,
		       coalesce((select count(*) from command_attempts attempt where attempt.command_id = command.id), 0),
		       run_step.output_snapshot_json
		  from task_run_steps run_step
		  join task_steps step on step.id = run_step.task_step_id and step.project_id = run_step.project_id
		  left join lateral (
		    select candidate.id, candidate.status, candidate.deadline_at
		      from device_commands candidate where candidate.task_run_step_id = run_step.id
		     order by candidate.created_at desc limit 1
		  ) command on true
		 where run_step.project_id = $1 and run_step.task_run_id = $2 order by run_step.position`, projectID, runID)
	if err != nil {
		return Snapshot{}, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var step Step
		var retries, backoff, timeout, attempts int
		var safe, pause bool
		var commandID, commandStatus sql.NullString
		var deadline sql.NullTime
		var rawOutput []byte
		if err := rows.Scan(&step.ID, &step.Position, &step.Status, &step.Key, &step.Uses, &step.CapabilityCode, &step.Action, &step.Parameters,
			&retries, &backoff, &timeout, &safe, &pause, &commandID, &commandStatus, &deadline, &attempts, &rawOutput); err != nil {
			return Snapshot{}, 0, err
		}
		if step.Status == StepSucceeded || step.Status == StepSkipped {
			var output map[string]any
			if err := json.Unmarshal(rawOutput, &output); err != nil {
				return Snapshot{}, 0, errors.New("TASK_STEP_OUTPUT_INVALID")
			}
			stepOutputs[step.Key] = output
		} else if step.Uses == "device.command" || step.Uses == "device.collect" || step.Uses == "" {
			var parameters map[string]any
			if err := json.Unmarshal(step.Parameters, &parameters); err != nil {
				return Snapshot{}, 0, errors.New("TASK_DEVICE_PARAMETERS_INVALID")
			}
			resolved, err := orchestration.ResolveReferences(parameters, orchestration.Context{Inputs: inputs, Steps: stepOutputs})
			if err != nil {
				return Snapshot{}, 0, err
			}
			step.Parameters, err = json.Marshal(resolved)
			if err != nil {
				return Snapshot{}, 0, err
			}
		}
		step.FailurePolicy = FailurePolicy{SafeToRetry: safe, MaxRetries: retries, Backoff: time.Duration(backoff) * time.Second,
			Timeout: time.Duration(timeout) * time.Second, PauseOnFailure: pause}
		if commandID.Valid {
			step.Command = &Command{ID: commandID.String, IdempotencyKey: fmt.Sprintf("task-run:%d:step:%d", runID, step.Position),
				CapabilityCode: step.CapabilityCode, Action: step.Action, Status: CommandStatus(commandStatus.String), Attempts: attempts, Deadline: deadline.Time}
		}
		snapshot.Steps = append(snapshot.Steps, step)
	}
	return snapshot, version, rows.Err()
}

func applyDecision(ctx context.Context, tx *sql.Tx, projectID, teamID int, snapshot Snapshot, version int, decision Decision, now time.Time) error {
	if decision.RevokeOrdinary {
		if _, err := tx.ExecContext(ctx, `update device_commands set status = 'canceled', completed_at = $3
			where project_id = $1 and task_run_id = $2 and priority < 90 and status in ('pending','dispatchable','sent')`, projectID, snapshot.RunID, now); err != nil {
			return err
		}
	}
	if decision.CompleteCommandID != "" {
		if _, err := tx.ExecContext(ctx, `update device_commands set status = $3, completed_at = $4
			where project_id = $1 and id::text = $2`, projectID, decision.CompleteCommandID, decision.CompleteCommandStatus, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `update command_attempts set status = $3, acknowledged_at = $4
			where project_id = $1 and command_id::text = $2 and attempt = (select max(a.attempt) from command_attempts a where a.command_id::text = $2)`,
			projectID, decision.CompleteCommandID, decision.CompleteCommandStatus, now); err != nil {
			return err
		}
	}
	if decision.StepPosition > 0 {
		if _, err := tx.ExecContext(ctx, `update task_run_steps set status = $3,
			started_at = case when $3 = 'running' and started_at is null then $4 else started_at end,
			finished_at = case when $3 in ('succeeded','failed','skipped') then $4 else finished_at end
			where project_id = $1 and task_run_id = $2 and position = $5`, projectID, snapshot.RunID, decision.StepStatus, now, decision.StepPosition); err != nil {
			return err
		}
		for _, step := range snapshot.Steps {
			if step.Position != decision.StepPosition {
				continue
			}
			uses := step.Uses
			if uses == "" {
				uses = "device.command"
			}
			outcome := map[StepStatus]string{StepRunning: "started", StepSucceeded: "succeeded", StepFailed: "failed", StepSkipped: "skipped", StepPaused: "paused"}[decision.StepStatus]
			if decision.Reason == "safe_retry_scheduled" {
				outcome = "retried"
			}
			if outcome != "" {
				_ = observability.DefaultMetrics.Record("aerosight_task_step_transitions_total", 1, map[string]string{"uses": uses, "outcome": outcome})
			}
			break
		}
	}
	if decision.IssueCommand != nil {
		command := decision.IssueCommand
		var commandID string
		err := tx.QueryRowContext(ctx, `insert into device_commands (
			id, project_id, team_id, task_run_id, task_run_step_id, device_id, command_key,
			idempotency_key, capability_code, parameters_json, safety_context_json, status, priority, deadline_at
		) select $3::uuid, run.project_id, run.team_id, run.id, step.id, run.selected_device_id,
			$4, $5, $6, $7, jsonb_build_object('scheduler','mission-v1'), 'dispatchable', $8, $9
		  from task_runs run left join task_run_steps step on step.task_run_id = run.id and step.position = $10
		 where run.project_id = $1 and run.id = $2
		on conflict (device_id, idempotency_key) do update set status = 'dispatchable', deadline_at = excluded.deadline_at
		returning id::text`, projectID, snapshot.RunID, command.ID, command.Action, command.IdempotencyKey,
			command.CapabilityCode, command.Parameters, command.Priority, command.Deadline, decision.StepPosition).Scan(&commandID)
		if err != nil {
			return err
		}
		dispatchPayload, err := json.Marshal(map[string]any{"commandId": commandID})
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json)
			values($1,$2,$3,'device.command.dispatch',$4) on conflict(event_id) do nothing`,
			projectID, teamID, "device.command.dispatch:"+commandID, dispatchPayload); err != nil {
			return err
		}
	}
	if decision.InvokeStep != nil {
		eventType := map[string]string{
			"algorithm.run": "task.step.algorithm.requested", "issue.create-or-update": "task.step.issue.requested",
			"copilot.run": "task.step.copilot.requested", "report.generate": "task.step.report.requested",
		}[decision.InvokeStep.Uses]
		if eventType == "" {
			return fmt.Errorf("unsupported task step uses %q", decision.InvokeStep.Uses)
		}
		payload, err := json.Marshal(map[string]any{"taskRunId": snapshot.RunID, "taskRunStepId": decision.InvokeStep.StepID})
		if err != nil {
			return err
		}
		eventID := fmt.Sprintf("%s:run:%d:step:%d", eventType, snapshot.RunID, decision.InvokeStep.StepID)
		if _, err := tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json,max_attempts)
			values($1,$2,$3,$4,$5,(select greatest(1,coalesce((retry_policy_json->>'maxAttempts')::int,1)) from task_steps where project_id=$1 and id=$6))
			on conflict(event_id) do nothing`, projectID, teamID, eventID, eventType, payload, decision.InvokeStep.StepID); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `update task_runs set status = $3, state_version = state_version + 1,
		state_reason = $4, current_step_position = nullif($5, 0),
		finished_at = case when $3 in ('succeeded','failed','canceled') then $6 else finished_at end,
		output_snapshot_json = case when $3 in ('succeeded','failed') then coalesce((
			select jsonb_object_agg(step.step_key,run_step.output_snapshot_json order by run_step.position)
			from task_run_steps run_step join task_steps step on step.id=run_step.task_step_id and step.project_id=run_step.project_id
			where run_step.project_id=$1 and run_step.task_run_id=$2
		),'{}'::jsonb) else output_snapshot_json end
		where project_id = $1 and id = $2 and state_version = $7`, projectID, snapshot.RunID, decision.RunStatus,
		decision.Reason, decision.StepPosition, now, version)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return errors.New("task run optimistic update conflict")
	}
	if decision.RunStatus == RunSucceeded || decision.RunStatus == RunFailed || decision.RunStatus == RunCanceled {
		payload := map[string]any{"taskRunId": snapshot.RunID, "from": snapshot.Status, "to": decision.RunStatus,
			"stateVersion": version + 1, "reason": decision.Reason}
		eventID := fmt.Sprintf("task-run:%d:state:%d:%s", snapshot.RunID, version+1, decision.RunStatus)
		if _, err := tx.ExecContext(ctx, `insert into project_events(project_id,team_id,event_id,event_type,payload_json)
			values($1,$2,$3,'task_run.transitioned',$4) on conflict(event_id) do nothing`, projectID, teamID, eventID, payload); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json)
			values($1,$2,$3,'task_run.transitioned',$4) on conflict(event_id) do nothing`, projectID, teamID, eventID, payload); err != nil {
			return err
		}
	}
	return nil
}
