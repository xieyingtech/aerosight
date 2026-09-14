package mission

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"aerosight/server/internal/orchestration"
	"aerosight/server/internal/outbox"
	"aerosight/server/internal/taskdefinition"
	"aerosight/server/internal/tasktrigger"
)

type preparedStepKey struct{}

// PreparedStep is constructed from the authorized Run and committed predecessor
// outputs. Handlers never have to trust project/user fields supplied in with.
type PreparedStep struct {
	ProjectID  int
	TeamID     int
	RunID      int
	StepID     int64
	UserID     int32
	Parameters map[string]any
}

func StepExecution(ctx context.Context) (PreparedStep, bool) {
	value, ok := ctx.Value(preparedStepKey{}).(PreparedStep)
	return value, ok
}

func validateStepOutput(raw, schemaJSON []byte) error {
	var schema map[string]any
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return errors.New("TASK_STEP_OUTPUT_SCHEMA_INVALID")
	}
	compiled, err := taskdefinition.CompileSchema(schema)
	if err != nil {
		return errors.New("TASK_STEP_OUTPUT_SCHEMA_INVALID")
	}
	var value any
	if err = json.Unmarshal(raw, &value); err != nil {
		return errors.New("TASK_STEP_OUTPUT_INVALID")
	}
	if err = compiled.Validate(value); err != nil {
		return fmt.Errorf("TASK_STEP_OUTPUT_INVALID: %w", err)
	}
	return nil
}

// withStepExecutionBoundary runs inside the failure policy savepoint. A bad
// synchronous result rolls back the handler's business writes before failure is
// persisted. Legacy v1 handlers retain their original contracts.
func withStepExecutionBoundary(handler outbox.Handler) outbox.Handler {
	return func(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
		var payload failedStepPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.TaskRunID <= 0 || payload.TaskRunStepID <= 0 {
			return errors.New("TASK_STEP_PAYLOAD_INVALID")
		}
		var dsl, status string
		var version int64
		var user sql.NullInt32
		var rawInputs []byte
		err := tx.QueryRowContext(ctx, `select coalesce(version.dsl_version,'aerosight/v1'),run.status,coalesce(run.task_version_id,0),run.created_by_user_id,run.input_snapshot_json
   from task_runs run left join task_versions version on version.id=run.task_version_id and version.project_id=run.project_id
   where run.project_id=$1 and run.team_id=$2 and run.id=$3 for update of run`, event.ProjectID, event.TeamID, payload.TaskRunID).Scan(&dsl, &status, &version, &user, &rawInputs)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("TASK_STEP_RUN_SCOPE_INVALID")
		}
		if err != nil {
			return err
		}
		if dsl != "aerosight/v2" {
			return handler(ctx, tx, event)
		}
		if status != "running" && status != "dispatching" && status != "queued" {
			return nil
		}
		var stepStatus string
		var position int
		var condition, dependencies, parameters, inputSchema, outputSchema []byte
		err = tx.QueryRowContext(ctx, `select rs.status,rs.position,coalesce(step.condition_json,'null'::jsonb),step.depends_on_json,step.parameters_json,step.input_schema_json,step.output_schema_json
   from task_run_steps rs join task_steps step on step.id=rs.task_step_id and step.project_id=rs.project_id and step.task_version_id=$4
   where rs.project_id=$1 and rs.task_run_id=$2 and rs.id=$3 for update of rs`, event.ProjectID, payload.TaskRunID, payload.TaskRunStepID, version).Scan(&stepStatus, &position, &condition, &dependencies, &parameters, &inputSchema, &outputSchema)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("TASK_STEP_SCOPE_INVALID")
		}
		if err != nil {
			return err
		}
		if stepStatus == "succeeded" || stepStatus == "skipped" || stepStatus == "failed" || stepStatus == "paused" {
			return nil
		}
		if !user.Valid {
			return errors.New("TASK_STEP_DELEGATE_REQUIRED")
		}
		if err = tasktrigger.AuthorizeDelegate(ctx, tx, event.ProjectID, version, user.Int32); err != nil {
			return err
		}
		var snapshot map[string]any
		if err = json.Unmarshal(rawInputs, &snapshot); err != nil {
			return errors.New("TASK_STEP_INPUT_INVALID")
		}
		inputs, _ := snapshot["inputs"].(map[string]any)
		prior := map[string]map[string]any{}
		states := map[string]string{}
		rows, err := tx.QueryContext(ctx, `select step.step_key,rs.status,rs.output_snapshot_json,step.output_schema_json from task_run_steps rs
   join task_steps step on step.id=rs.task_step_id and step.project_id=rs.project_id
   where rs.project_id=$1 and rs.task_run_id=$2 and rs.position<$3 order by rs.position`, event.ProjectID, payload.TaskRunID, position)
		if err != nil {
			return err
		}
		for rows.Next() {
			var key, state string
			var raw, schema []byte
			if err = rows.Scan(&key, &state, &raw, &schema); err != nil {
				rows.Close()
				return err
			}
			states[key] = state
			if state != "succeeded" && state != "skipped" {
				rows.Close()
				return errors.New("TASK_STEP_DEPENDENCY_NOT_COMPLETE")
			}
			if state == "succeeded" {
				if err = validateStepOutput(raw, schema); err != nil {
					rows.Close()
					return fmt.Errorf("TASK_STEP_DEPENDENCY_OUTPUT_INVALID: %w", err)
				}
				var output map[string]any
				if err = json.Unmarshal(raw, &output); err != nil {
					rows.Close()
					return errors.New("TASK_STEP_DEPENDENCY_OUTPUT_INVALID")
				}
				prior[key] = output
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		var deps []string
		if err = json.Unmarshal(dependencies, &deps); err != nil {
			return errors.New("TASK_STEP_DEPENDENCY_INVALID")
		}
		for _, key := range deps {
			if states[key] != "succeeded" {
				return errors.New("TASK_STEP_DEPENDENCY_UNAVAILABLE")
			}
		}
		evaluation := orchestration.ConditionAudit{Result: true, References: []string{}}
		if string(condition) != "null" && len(condition) > 0 {
			evaluation, err = orchestration.EvaluateCondition(condition, orchestration.Context{Inputs: inputs, Steps: prior})
			if err != nil {
				return err
			}
		}
		audit, _ := json.Marshal(evaluation)
		if _, err = tx.ExecContext(ctx, "update task_run_steps set condition_result_json=$3 where project_id=$1 and id=$2", event.ProjectID, payload.TaskRunStepID, audit); err != nil {
			return err
		}
		if !evaluation.Result {
			if _, err = tx.ExecContext(ctx, "update task_run_steps set status='skipped',output_snapshot_json='{}',finished_at=now() where project_id=$1 and id=$2", event.ProjectID, payload.TaskRunStepID); err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json) values($1,$2,$3,'task_run.transitioned',$4) on conflict(event_id) do nothing`, event.ProjectID, event.TeamID, fmt.Sprintf("task-run:%d:step:%d:condition-skipped", payload.TaskRunID, payload.TaskRunStepID), map[string]any{"taskRunId": payload.TaskRunID, "to": "running", "completedStepId": payload.TaskRunStepID})
			return err
		}
		var params map[string]any
		if err = json.Unmarshal(parameters, &params); err != nil {
			return errors.New("TASK_STEP_PARAMETERS_INVALID")
		}
		resolved, err := orchestration.ResolveReferences(params, orchestration.Context{Inputs: inputs, Steps: prior})
		if err != nil {
			return err
		}
		normalized, err := taskdefinition.MergeInputs(inputSchema, nil, resolved.(map[string]any))
		if err != nil {
			return fmt.Errorf("TASK_STEP_INPUT_INVALID: %w", err)
		}
		if _, err = tx.ExecContext(ctx, "update task_run_steps set input_snapshot_json=$3 where project_id=$1 and id=$2", event.ProjectID, payload.TaskRunStepID, map[string]any{"parameters": normalized, "inputs": inputs}); err != nil {
			return err
		}
		prepared := PreparedStep{ProjectID: event.ProjectID, TeamID: event.TeamID, RunID: payload.TaskRunID, StepID: payload.TaskRunStepID, UserID: user.Int32, Parameters: normalized}
		if err = handler(context.WithValue(ctx, preparedStepKey{}, prepared), tx, event); err != nil {
			return err
		}
		var rawOutput []byte
		if err = tx.QueryRowContext(ctx, "select status,output_snapshot_json from task_run_steps where project_id=$1 and id=$2", event.ProjectID, payload.TaskRunStepID).Scan(&stepStatus, &rawOutput); err != nil {
			return err
		}
		if stepStatus == "succeeded" {
			return validateStepOutput(rawOutput, outputSchema)
		}
		return nil
	}
}
