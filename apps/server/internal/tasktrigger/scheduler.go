package tasktrigger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"aerosight/server/internal/outbox"
	"aerosight/server/internal/taskdefinition"
)

type scheduleTrigger struct {
	Type     string `json:"type"`
	Cron     string `json:"cron"`
	Timezone string `json:"timezone"`
	Enabled  bool   `json:"enabled"`
}

type upstreamTrigger struct {
	Type     string   `json:"type"`
	TaskID   int      `json:"taskId"`
	Statuses []string `json:"statuses"`
}

type candidate struct {
	ProjectID        int
	TeamID           int
	TaskID           int
	TaskVersionID    int64
	ConcurrencyLimit int
	TriggerJSON      []byte
	InputSchemaJSON  []byte
	DSLVersion       string
}

type Scheduler struct {
	db       *sql.DB
	now      func() time.Time
	interval time.Duration
	logger   *slog.Logger
}

func NewScheduler(database *sql.DB, now func() time.Time, interval time.Duration, logger *slog.Logger) *Scheduler {
	if now == nil {
		now = time.Now
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{db: database, now: now, interval: interval, logger: logger}
}

func parseFieldPart(part string, value, minimum, maximum int) (bool, error) {
	base, stepText, hasStep := strings.Cut(part, "/")
	step := 1
	if hasStep {
		parsed, err := strconv.Atoi(stepText)
		if err != nil || parsed <= 0 {
			return false, fmt.Errorf("invalid cron step %q", stepText)
		}
		step = parsed
	}
	start, end := minimum, maximum
	if base != "*" {
		left, right, ranged := strings.Cut(base, "-")
		parsed, err := strconv.Atoi(left)
		if err != nil {
			return false, fmt.Errorf("invalid cron field %q", part)
		}
		start, end = parsed, parsed
		if hasStep && !ranged {
			end = maximum
		}
		if ranged {
			parsedEnd, endErr := strconv.Atoi(right)
			if endErr != nil {
				return false, fmt.Errorf("invalid cron range %q", part)
			}
			end = parsedEnd
		}
	}
	if start < minimum || end > maximum || start > end {
		return false, fmt.Errorf("cron field %q is outside %d-%d", part, minimum, maximum)
	}
	return value >= start && value <= end && (value-start)%step == 0, nil
}

func fieldMatches(spec string, value, minimum, maximum int) (bool, error) {
	result := false
	for _, part := range strings.Split(spec, ",") {
		matched, err := parseFieldPart(strings.TrimSpace(part), value, minimum, maximum)
		if err != nil {
			return false, err
		}
		result = result || matched
	}
	return result, nil
}

func CronMatches(expression string, moment time.Time) (bool, error) {
	fields := strings.Fields(expression)
	if len(fields) != 5 {
		return false, errors.New("cron expression must have five fields")
	}
	values := []int{moment.Minute(), moment.Hour(), moment.Day(), int(moment.Month()), int(moment.Weekday())}
	minima := []int{0, 0, 1, 1, 0}
	maxima := []int{59, 23, 31, 12, 7}
	matches := make([]bool, 5)
	for i, field := range fields {
		matched, err := fieldMatches(field, values[i], minima[i], maxima[i])
		if err != nil {
			return false, err
		}
		if i == 4 && values[i] == 0 {
			sunday, err := fieldMatches(field, 7, 0, 7)
			if err != nil {
				return false, err
			}
			matched = matched || sunday
		}
		matches[i] = matched
	}
	day := matches[2] && matches[4]
	if fields[2] != "*" && fields[4] != "*" {
		day = matches[2] || matches[4]
	}
	return matches[0] && matches[1] && matches[3] && day, nil
}

func (scheduler *Scheduler) scheduleCandidates(ctx context.Context) ([]candidate, error) {
	rows, err := scheduler.db.QueryContext(ctx, `
		select version.project_id,version.team_id,version.task_id,version.id,version.concurrency_limit,version.trigger_json,version.input_schema_json,version.dsl_version
		  from task_versions version
		  join tasks task on task.id=version.task_id and task.project_id=version.project_id
		 where version.status='published' and task.status='active'
		   and task.current_published_version_id=version.id
		   and version.trigger_json->>'type'='schedule'
		   and coalesce(version.trigger_json->>'enabled','true')<>'false'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.ProjectID, &item.TeamID, &item.TaskID, &item.TaskVersionID, &item.ConcurrencyLimit, &item.TriggerJSON, &item.InputSchemaJSON, &item.DSLVersion); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func activeRunCount(ctx context.Context, tx *sql.Tx, item candidate) (int, error) {
	var count int
	err := tx.QueryRowContext(ctx, `select count(*) from task_runs
		where project_id=$1 and task_id=$2
		  and status in ('queued','blocked','ready','dispatching','running','paused','canceling')`, item.ProjectID, item.TaskID).Scan(&count)
	return count, err
}

func snapshotInputs(snapshot map[string]any) map[string]any {
	inputs, _ := snapshot["inputs"].(map[string]any)
	if inputs == nil {
		return map[string]any{}
	}
	return inputs
}

func validateInputs(schemaJSON []byte, inputs map[string]any) error {
	var schema struct {
		Properties           map[string]any `json:"properties"`
		Required             []string       `json:"required"`
		AdditionalProperties *bool          `json:"additionalProperties"`
	}
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return fmt.Errorf("TASK_TRIGGER_INPUT_SCHEMA_INVALID: %w", err)
	}
	for _, key := range schema.Required {
		if _, ok := inputs[key]; !ok {
			return fmt.Errorf("TASK_TRIGGER_INPUT_REQUIRED:%s", key)
		}
	}
	if schema.AdditionalProperties != nil && !*schema.AdditionalProperties {
		for key := range inputs {
			if _, ok := schema.Properties[key]; !ok {
				return fmt.Errorf("TASK_TRIGGER_INPUT_UNKNOWN:%s", key)
			}
		}
	}
	return nil
}

func createRun(ctx context.Context, tx *sql.Tx, item candidate, source, triggerKey string, snapshot map[string]any) (int, bool, error) {
	if _, err := tx.ExecContext(ctx, "select pg_advisory_xact_lock($1,$2)", item.ProjectID, item.TaskID); err != nil {
		return 0, false, err
	}
	var existing int
	err := tx.QueryRowContext(ctx, `select id from task_runs where project_id=$1 and task_id=$2 and trigger_key=$3 order by id limit 1`, item.ProjectID, item.TaskID, triggerKey).Scan(&existing)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	// Candidate discovery happens outside this transaction. Lock and recheck the
	// current task/version so disable or publish cannot race a stale submission.
	var currentVersion int64
	var taskStatus, versionStatus, dsl string
	var delegate sql.NullInt32
	var currentTrigger []byte
	err = tx.QueryRowContext(ctx, `select task.status,version.id,version.status,
		version.input_schema_json,version.concurrency_limit,version.dsl_version,task.authorized_by_user_id,version.trigger_json
		from tasks task join task_versions version on version.id=task.current_published_version_id and version.project_id=task.project_id
		where task.project_id=$1 and task.id=$2 for update of task,version`, item.ProjectID, item.TaskID).
		Scan(&taskStatus, &currentVersion, &versionStatus, &item.InputSchemaJSON, &item.ConcurrencyLimit, &dsl, &delegate, &currentTrigger)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if taskStatus != "active" || versionStatus != "published" || currentVersion != item.TaskVersionID {
		return 0, false, nil
	}
	if dsl == "aerosight/v2" || source == "schedule" {
		if !delegate.Valid {
			return 0, false, errors.New("TASK_TRIGGER_DELEGATE_REQUIRED")
		}
		if err := authorizeDelegate(ctx, tx, item, delegate.Int32); err != nil {
			return 0, false, err
		}
		var trigger map[string]any
		if err := json.Unmarshal(currentTrigger, &trigger); err != nil {
			return 0, false, err
		}
		defaults, _ := trigger["inputs"].(map[string]any)
		inputs, err := taskdefinition.MergeInputs(item.InputSchemaJSON, defaults, snapshotInputs(snapshot))
		if err != nil {
			return 0, false, err
		}
		snapshot["inputs"] = inputs
		snapshot["authorizedByUserId"] = delegate.Int32
	} else if err := validateInputs(item.InputSchemaJSON, snapshotInputs(snapshot)); err != nil {
		return 0, false, err
	}
	count, err := activeRunCount(ctx, tx, item)
	if err != nil {
		return 0, false, err
	}
	if count >= item.ConcurrencyLimit {
		return 0, false, nil
	}
	var runID int
	err = tx.QueryRowContext(ctx, `insert into task_runs(
		project_id,team_id,task_id,task_version_id,trigger_source,trigger_key,status,input_snapshot_json,state_reason,created_by_user_id)
		values($1,$2,$3,$4,$5,$6,'queued',$7,'trigger-accepted',$8) returning id`,
		item.ProjectID, item.TeamID, item.TaskID, item.TaskVersionID, source, triggerKey, snapshot, delegate).Scan(&runID)
	if err != nil {
		return 0, false, err
	}
	if _, err := tx.ExecContext(ctx, `insert into task_run_steps(
		project_id,team_id,task_run_id,task_step_id,position,status,execution_key)
		select project_id,team_id,$3,id,position,'pending',$4||':step:'||step_key
		  from task_steps where project_id=$1 and task_version_id=$2 order by position`,
		item.ProjectID, item.TaskVersionID, runID, triggerKey); err != nil {
		return 0, false, err
	}
	payload, _ := json.Marshal(map[string]any{"taskRunId": runID, "taskVersionId": item.TaskVersionID, "triggerType": source})
	if _, err := tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json)
		values($1,$2,$3,'task_run.triggered',$4) on conflict(event_id) do nothing`,
		item.ProjectID, item.TeamID, fmt.Sprintf("task-run-triggered:%d:%s", item.TaskVersionID, triggerKey), payload); err != nil {
		return 0, false, err
	}
	return runID, true, nil
}

func (scheduler *Scheduler) ReconcileOnce(ctx context.Context) (int, error) {
	items, err := scheduler.scheduleCandidates(ctx)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, item := range items {
		inserted, err := scheduler.reconcileSchedule(ctx, item)
		if err != nil {
			scheduler.logger.Error("task schedule rejected", "taskId", item.TaskID, "error", err.Error())
			continue
		}
		created += inserted
	}

	return created, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (scheduler *Scheduler) UpstreamHandler(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
	var payload struct {
		TaskRunID int    `json:"taskRunId"`
		To        string `json:"to"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	if payload.To != "succeeded" && payload.To != "failed" {
		return nil
	}
	var sourceTaskID int
	var output map[string]any
	var rawOutput []byte
	if err := tx.QueryRowContext(ctx, `select task_id,output_snapshot_json from task_runs where project_id=$1 and id=$2`, event.ProjectID, payload.TaskRunID).Scan(&sourceTaskID, &rawOutput); err != nil {
		return err
	}
	if err := json.Unmarshal(rawOutput, &output); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `select version.project_id,version.team_id,version.task_id,version.id,version.concurrency_limit,version.trigger_json,version.input_schema_json
		from task_versions version join tasks task on task.id=version.task_id and task.project_id=version.project_id
		where version.project_id=$1 and version.status='published' and task.status='active'
		  and task.current_published_version_id=version.id and version.trigger_json->>'type'='upstream'`, event.ProjectID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var items []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.ProjectID, &item.TeamID, &item.TaskID, &item.TaskVersionID, &item.ConcurrencyLimit, &item.TriggerJSON, &item.InputSchemaJSON); err != nil {
			return err
		}
		items = append(items, item)
	}
	for _, item := range items {
		var trigger upstreamTrigger
		if err := json.Unmarshal(item.TriggerJSON, &trigger); err != nil {
			return err
		}
		if trigger.TaskID != sourceTaskID || !contains(trigger.Statuses, payload.To) {
			continue
		}
		triggerKey := fmt.Sprintf("upstream:%d:%s", payload.TaskRunID, payload.To)
		snapshot := map[string]any{"trigger": map[string]any{
			"type": "upstream", "idempotencyKey": fmt.Sprintf("%d:%s", payload.TaskRunID, payload.To),
			"occurredAt": scheduler.now().UTC().Format(time.RFC3339Nano), "upstreamProjectId": event.ProjectID,
			"upstreamTaskId": sourceTaskID, "upstreamRunId": payload.TaskRunID, "status": payload.To,
			"actor": map[string]any{"type": "service", "id": "upstream-task"},
		}, "inputs": output}
		if _, _, err := createRun(ctx, tx, item, "upstream", triggerKey, snapshot); err != nil {
			return err
		}
	}
	return rows.Err()
}

func CreateCopilotRuns(ctx context.Context, tx *sql.Tx, projectID int, jobID, triggerType string, inputs map[string]any, now time.Time) error {
	delegation := map[string]string{"issue_mention": "issue-mention", "issue_assignment": "issue-assignment", "chat": "chat"}[triggerType]
	if delegation == "" {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `select version.project_id,version.team_id,version.task_id,version.id,version.concurrency_limit,version.trigger_json,version.input_schema_json
		from task_versions version join tasks task on task.id=version.task_id and task.project_id=version.project_id
		where version.project_id=$1 and version.status='published' and task.status='active'
		  and task.current_published_version_id=version.id and version.trigger_json->>'type'='copilot'
		  and version.trigger_json->>'delegation'=$2`, projectID, delegation)
	if err != nil {
		return err
	}
	defer rows.Close()
	var items []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.ProjectID, &item.TeamID, &item.TaskID, &item.TaskVersionID, &item.ConcurrencyLimit, &item.TriggerJSON, &item.InputSchemaJSON); err != nil {
			return err
		}
		items = append(items, item)
	}
	for _, item := range items {
		triggerKey := "copilot:" + jobID
		snapshot := map[string]any{"trigger": map[string]any{
			"type": "copilot", "idempotencyKey": jobID, "occurredAt": now.UTC().Format(time.RFC3339Nano),
			"delegation": delegation, "agentJobId": jobID,
			"actor": map[string]any{"type": "service", "id": "copilot-job"},
		}, "inputs": inputs}
		if _, _, err := createRun(ctx, tx, item, "copilot", triggerKey, snapshot); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (scheduler *Scheduler) Run(ctx context.Context) error {
	ticker := time.NewTicker(scheduler.interval)
	defer ticker.Stop()
	for {
		if _, err := scheduler.ReconcileOnce(ctx); err != nil && ctx.Err() == nil {
			scheduler.logger.Error("task schedule reconciliation failed", "error", err.Error())
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
