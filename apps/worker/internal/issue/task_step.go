package issue

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"aerosight/worker/internal/observability"
	"aerosight/worker/internal/orchestration"
	"aerosight/worker/internal/outbox"
)

type TaskStepProcessor struct {
	now func() time.Time
}

func NewTaskStepProcessor(now func() time.Time) *TaskStepProcessor {
	if now == nil {
		now = time.Now
	}
	return &TaskStepProcessor{now: now}
}

type taskStepPayload struct {
	TaskRunID     int   `json:"taskRunId"`
	TaskRunStepID int64 `json:"taskRunStepId"`
}

type issueParameters struct {
	Title             string   `json:"title"`
	Description       string   `json:"description"`
	Priority          string   `json:"priority"`
	Labels            []string `json:"labels"`
	ConditionScopeKey string   `json:"conditionScopeKey"`
	BusinessObjectKey string   `json:"businessObjectKey"`
	AlgorithmRunID    string   `json:"algorithmRunId"`
	DetectionIDs      []int64  `json:"detectionIds"`
	AssetIDs          []int    `json:"assetIds"`
}

type taskStepRecord struct {
	ProjectID, TeamID, TaskRunID  int
	TaskRunStepID, TaskVersionID  int64
	StepKey, Status               string
	Parameters, Condition, Inputs []byte
}

func (processor *TaskStepProcessor) Handler(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
	var payload taskStepPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.TaskRunID <= 0 || payload.TaskRunStepID <= 0 {
		return errors.New("task issue payload requires taskRunId and taskRunStepId")
	}
	var record taskStepRecord
	record.ProjectID, record.TeamID, record.TaskRunID, record.TaskRunStepID = event.ProjectID, event.TeamID, payload.TaskRunID, payload.TaskRunStepID
	err := tx.QueryRowContext(ctx, `select run.task_version_id,step.step_key,run_step.status,step.parameters_json,step.condition_json,run.input_snapshot_json
		from task_run_steps run_step join task_runs run on run.id=run_step.task_run_id and run.project_id=run_step.project_id
		join task_steps step on step.id=run_step.task_step_id and step.project_id=run_step.project_id and step.uses='issue.create-or-update'
		where run_step.project_id=$1 and run_step.team_id=$2 and run_step.id=$3 and run.id=$4`, event.ProjectID, event.TeamID,
		payload.TaskRunStepID, payload.TaskRunID).Scan(&record.TaskVersionID, &record.StepKey, &record.Status, &record.Parameters, &record.Condition, &record.Inputs)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("TASK_ISSUE_STEP_SCOPE_INVALID")
	}
	if err != nil {
		return err
	}
	if record.Status == "succeeded" || record.Status == "skipped" {
		return nil
	}
	if record.Status == "failed" || record.Status == "paused" {
		return errors.New("TASK_ISSUE_STEP_NOT_EXECUTABLE")
	}
	contextValue, err := loadContext(ctx, tx, record)
	if err != nil {
		return err
	}
	conditionAudit := orchestration.ConditionAudit{Result: true}
	if len(record.Condition) > 0 && string(record.Condition) != "null" {
		conditionAudit, err = orchestration.EvaluateCondition(record.Condition, contextValue)
		if err != nil {
			return err
		}
	}
	auditJSON, err := json.Marshal(conditionAudit)
	if err != nil {
		return err
	}
	if !conditionAudit.Result {
		if _, err := tx.ExecContext(ctx, `update task_run_steps set status='skipped',attempt_count=greatest(attempt_count,1),condition_result_json=$3,
			input_snapshot_json=$5,output_snapshot_json='{"created":false,"reason":"condition_false"}'::jsonb,
			result_json=result_json||'{"created":false,"reason":"condition_false"}'::jsonb,
			started_at=coalesce(started_at,$4),finished_at=$4
			where project_id=$1 and id=$2`, record.ProjectID, record.TaskRunStepID, auditJSON, processor.now(), record.Parameters); err != nil {
			return err
		}
		return enqueueContinuation(ctx, tx, record, "condition-false")
	}
	parameterValue, err := decodeObject(record.Parameters)
	if err != nil {
		return errors.New("TASK_ISSUE_PARAMETERS_INVALID")
	}
	resolved, err := orchestration.ResolveReferences(parameterValue, contextValue)
	if err != nil {
		return err
	}
	resolvedJSON, err := json.Marshal(resolved)
	if err != nil {
		return err
	}
	var parameters issueParameters
	if err := json.Unmarshal(resolvedJSON, &parameters); err != nil {
		return errors.New("TASK_ISSUE_PARAMETERS_INVALID")
	}
	if parameters.Title == "" || parameters.ConditionScopeKey == "" || parameters.BusinessObjectKey == "" {
		return errors.New("TASK_ISSUE_PARAMETERS_INVALID")
	}
	if parameters.Priority == "" {
		parameters.Priority = "medium"
	}
	if parameters.Priority != "low" && parameters.Priority != "medium" && parameters.Priority != "high" && parameters.Priority != "critical" {
		return errors.New("TASK_ISSUE_PRIORITY_INVALID")
	}
	return processor.createOrUpdate(ctx, tx, record, parameters, resolvedJSON, auditJSON)
}

func loadContext(ctx context.Context, tx *sql.Tx, record taskStepRecord) (orchestration.Context, error) {
	inputs, err := decodeObject(record.Inputs)
	if err != nil {
		return orchestration.Context{}, errors.New("TASK_RUN_INPUT_INVALID")
	}
	if nested, ok := inputs["inputs"].(map[string]any); ok {
		inputs = nested
	}
	steps := map[string]map[string]any{}
	rows, err := tx.QueryContext(ctx, `select step.step_key,run_step.output_snapshot_json
		from task_run_steps run_step join task_steps step on step.id=run_step.task_step_id and step.project_id=run_step.project_id
		where run_step.project_id=$1 and run_step.task_run_id=$2 and run_step.position <
		(select position from task_run_steps where project_id=$1 and id=$3) and run_step.status in('succeeded','skipped')`,
		record.ProjectID, record.TaskRunID, record.TaskRunStepID)
	if err != nil {
		return orchestration.Context{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return orchestration.Context{}, err
		}
		output, err := decodeObject(raw)
		if err != nil {
			return orchestration.Context{}, err
		}
		steps[key] = output
	}
	return orchestration.Context{Inputs: inputs, Steps: steps}, rows.Err()
}

func (processor *TaskStepProcessor) createOrUpdate(ctx context.Context, tx *sql.Tx, record taskStepRecord, parameters issueParameters, inputJSON, conditionJSON []byte) error {
	started := time.Now()
	lockKey := taskIssueKey(record.ProjectID, record.TaskVersionID, parameters.ConditionScopeKey, parameters.BusinessObjectKey)
	if _, err := tx.ExecContext(ctx, "select pg_advisory_xact_lock(hashtextextended($1,0))", lockKey); err != nil {
		return err
	}
	now := processor.now().UTC()
	var issueID, number int
	created := false
	err := tx.QueryRowContext(ctx, `select id,number from issues where project_id=$1 and task_version_id=$2
		and condition_scope_key=$3 and business_object_key=$4 for update`, record.ProjectID, record.TaskVersionID,
		parameters.ConditionScopeKey, parameters.BusinessObjectKey).Scan(&issueID, &number)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.ExecContext(ctx, "select pg_advisory_xact_lock(hashtextextended($1,0))", fmt.Sprintf("issue-number:%d", record.ProjectID)); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `select coalesce(max(number),0)+1 from issues where project_id=$1`, record.ProjectID).Scan(&number); err != nil {
			return err
		}
		labels, _ := json.Marshal(parameters.Labels)
		err = tx.QueryRowContext(ctx, `insert into issues(project_id,number,title,description,source_type,source_id,status,priority,
			task_run_id,task_version_id,condition_scope_key,business_object_key,labels_json,first_seen_at,last_seen_at)
			values($1,$2,$3,nullif($4,''),'task',$5,'open',$6,$7,$8,$9,$10,$11,$12,$12) returning id`, record.ProjectID,
			number, parameters.Title, parameters.Description, record.TaskRunID, parameters.Priority, record.TaskRunID,
			record.TaskVersionID, parameters.ConditionScopeKey, parameters.BusinessObjectKey, labels, now).Scan(&issueID)
		created = true
	} else if err == nil {
		_, err = tx.ExecContext(ctx, `update issues set occurrence_count=occurrence_count+1,last_seen_at=$2,updated_at=$2,
			title=$3,description=nullif($4,''),priority=$5 where id=$1`, issueID, now, parameters.Title, parameters.Description, parameters.Priority)
	}
	if err != nil {
		return err
	}
	activityType := "issue.updated"
	if created {
		activityType = "issue.created"
	}
	metadata := map[string]any{"taskRunId": record.TaskRunID, "taskRunStepId": record.TaskRunStepID,
		"taskVersionId": record.TaskVersionID, "condition": json.RawMessage(conditionJSON)}
	if _, err := tx.ExecContext(ctx, `insert into issue_events(project_id,issue_id,event_type,metadata_json)
		values($1,$2,$3,$4)`, record.ProjectID, issueID, activityType, metadata); err != nil {
		return err
	}
	links := []struct{ kind, id string }{{"task_run", strconv.Itoa(record.TaskRunID)}, {"task_version", strconv.FormatInt(record.TaskVersionID, 10)},
		{"task_step", strconv.FormatInt(record.TaskRunStepID, 10)}, {"condition", fmt.Sprintf("%s:%s", record.StepKey, parameters.ConditionScopeKey)}}
	if parameters.AlgorithmRunID != "" {
		var allowed bool
		if err := tx.QueryRowContext(ctx, `select exists(select 1 from algorithm_runs where id=$1 and project_id=$2 and task_run_id=$3)`,
			parameters.AlgorithmRunID, record.ProjectID, record.TaskRunID).Scan(&allowed); err != nil || !allowed {
			return errors.New("TASK_ISSUE_ALGORITHM_SCOPE_INVALID")
		}
		links = append(links, struct{ kind, id string }{"algorithm_run", parameters.AlgorithmRunID})
	}
	for _, detectionID := range parameters.DetectionIDs {
		var allowed bool
		if err := tx.QueryRowContext(ctx, `select exists(select 1 from detections where id=$1 and project_id=$2 and task_run_id=$3)`,
			detectionID, record.ProjectID, record.TaskRunID).Scan(&allowed); err != nil || !allowed {
			return errors.New("TASK_ISSUE_DETECTION_SCOPE_INVALID")
		}
		links = append(links, struct{ kind, id string }{"detection", strconv.FormatInt(detectionID, 10)})
	}
	for _, assetID := range parameters.AssetIDs {
		var allowed bool
		if err := tx.QueryRowContext(ctx, `select exists(select 1 from assets where id=$1 and project_id=$2 and task_run_id=$3)`,
			assetID, record.ProjectID, record.TaskRunID).Scan(&allowed); err != nil || !allowed {
			return errors.New("TASK_ISSUE_ASSET_SCOPE_INVALID")
		}
		links = append(links, struct{ kind, id string }{"asset", strconv.Itoa(assetID)})
	}
	for _, link := range links {
		if _, err := tx.ExecContext(ctx, `insert into issue_links(project_id,issue_id,link_type,target_id)
			values($1,$2,$3,$4) on conflict(issue_id,link_type,target_id) do nothing`, record.ProjectID, issueID, link.kind, link.id); err != nil {
			return err
		}
	}
	output, _ := json.Marshal(map[string]any{"issueId": issueID, "number": number, "created": created})
	executionKey := fmt.Sprintf("task-run:%d:step:%d", record.TaskRunID, record.TaskRunStepID)
	if _, err := tx.ExecContext(ctx, `update task_run_steps set status='succeeded',attempt_count=greatest(attempt_count,1),
		input_snapshot_json=$3,condition_result_json=$4,output_snapshot_json=$5,result_json=result_json||$5,
		execution_key=$6,finished_at=$7 where project_id=$1 and id=$2`, record.ProjectID, record.TaskRunStepID,
		inputJSON, conditionJSON, output, executionKey, now); err != nil {
		return err
	}
	projectEventType := "issue.updated"
	if created {
		projectEventType = "issue.created"
	}
	projectEventID := fmt.Sprintf("%s:%d:step:%d", projectEventType, issueID, record.TaskRunStepID)
	if _, err := tx.ExecContext(ctx, `insert into project_events(project_id,team_id,event_id,event_type,payload_json)
		values($1,$2,$3,$4,jsonb_build_object('issueId',$5::int,'taskRunId',$6::int)) on conflict(event_id) do nothing`,
		record.ProjectID, record.TeamID, projectEventID, projectEventType, issueID, record.TaskRunID); err != nil {
		return err
	}
	outcome := "updated"
	if created {
		outcome = "created"
	}
	_ = observability.DefaultMetrics.Record("aerosight_issue_creation_latency_seconds", time.Since(started).Seconds(), map[string]string{"source": "task", "outcome": outcome})
	return enqueueContinuation(ctx, tx, record, "succeeded")
}

func enqueueContinuation(ctx context.Context, tx *sql.Tx, record taskStepRecord, outcome string) error {
	payload := map[string]any{"taskRunId": record.TaskRunID, "to": "running", "completedStepId": record.TaskRunStepID}
	_, err := tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json)
		values($1,$2,$3,'task_run.transitioned',$4) on conflict(event_id) do nothing`, record.ProjectID, record.TeamID,
		fmt.Sprintf("task-run-continue:%d:step:%d:%s", record.TaskRunID, record.TaskRunStepID, outcome), payload)
	return err
}

func decodeObject(raw []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var object map[string]any
	err := decoder.Decode(&object)
	return object, err
}

func taskIssueKey(projectID int, taskVersionID int64, conditionScope, businessObject string) string {
	return fmt.Sprintf("task-issue:%d:%d:%s:%s", projectID, taskVersionID, conditionScope, businessObject)
}
