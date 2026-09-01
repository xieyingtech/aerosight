package report

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"aerosight/worker/internal/outbox"
)

type taskStepPayload struct {
	TaskRunID     int   `json:"taskRunId"`
	TaskRunStepID int64 `json:"taskRunStepId"`
}

type stepFact struct {
	ID        int64           `json:"id"`
	Key       string          `json:"key"`
	Uses      string          `json:"uses"`
	Status    string          `json:"status"`
	Condition json.RawMessage `json:"condition,omitempty"`
	Output    json.RawMessage `json:"output"`
}

type issueFact struct {
	ID          int    `json:"id"`
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Priority    string `json:"priority"`
	Conclusion  string `json:"conclusion,omitempty"`
	LastEventID int    `json:"lastEventId,omitempty"`
}

type assetFact struct {
	ID       int    `json:"id"`
	Version  int    `json:"version"`
	Kind     string `json:"kind"`
	Checksum string `json:"checksumSha256,omitempty"`
}

type reportContent struct {
	SchemaVersion string         `json:"schemaVersion"`
	TaskRun       map[string]any `json:"taskRun"`
	Steps         []stepFact     `json:"steps"`
	Issues        []issueFact    `json:"issues"`
	Assets        []assetFact    `json:"assets"`
	DataGaps      []string       `json:"dataGaps"`
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
	var payload taskStepPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.TaskRunID <= 0 || payload.TaskRunStepID <= 0 {
		return errors.New("TASK_REPORT_PAYLOAD_INVALID")
	}
	var taskVersionID int64
	var taskName, stepStatus string
	var createdBy sql.NullInt64
	err := tx.QueryRowContext(ctx, `select run.task_version_id,task.name,run_step.status,
		coalesce(run.created_by_user_id,task.created_by_user_id,project.created_by_user_id)
		from task_run_steps run_step
		join task_runs run on run.id=run_step.task_run_id and run.project_id=run_step.project_id
		join tasks task on task.id=run.task_id and task.project_id=run.project_id
		join projects project on project.id=run.project_id
		join task_steps step on step.id=run_step.task_step_id and step.project_id=run_step.project_id and step.uses='report.generate'
		where run_step.project_id=$1 and run_step.team_id=$2 and run_step.id=$3 and run.id=$4`,
		event.ProjectID, event.TeamID, payload.TaskRunStepID, payload.TaskRunID).Scan(&taskVersionID, &taskName, &stepStatus, &createdBy)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("TASK_REPORT_STEP_SCOPE_INVALID")
	}
	if err != nil {
		return err
	}
	if stepStatus == "succeeded" || stepStatus == "skipped" {
		return nil
	}
	if stepStatus == "failed" || stepStatus == "paused" {
		return errors.New("TASK_REPORT_STEP_NOT_EXECUTABLE")
	}
	content, eventIDs, err := loadContent(ctx, tx, event.ProjectID, payload.TaskRunID, payload.TaskRunStepID, taskVersionID)
	if err != nil {
		return err
	}
	completeness := "complete"
	if len(content.DataGaps) > 0 {
		completeness = "incomplete"
	}
	contentJSON, _ := json.Marshal(content)
	gapsJSON, _ := json.Marshal(content.DataGaps)
	var reportID string
	if err := tx.QueryRowContext(ctx, `insert into generated_reports(project_id,team_id,source_type,source_id,title,created_by_user_id)
		values($1,$2,'task_run',$3,$4,$5)
		on conflict(project_id,source_type,source_id) do update set title=excluded.title,updated_at=now()
		returning id::text`, event.ProjectID, event.TeamID, fmt.Sprint(payload.TaskRunID), taskName+" 巡检报告", nullableInt64(createdBy)).Scan(&reportID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `update generated_report_versions set status='retired'
		where project_id=$1 and generated_report_id=$2 and status='draft'`, event.ProjectID, reportID); err != nil {
		return err
	}
	var version int
	if err := tx.QueryRowContext(ctx, `select coalesce(max(version),0)+1 from generated_report_versions where generated_report_id=$1`, reportID).Scan(&version); err != nil {
		return err
	}
	var reportVersionID string
	if err := tx.QueryRowContext(ctx, `insert into generated_report_versions(
		project_id,team_id,generated_report_id,version,completeness,content_json,data_gaps_json,created_by_user_id)
		values($1,$2,$3,$4,$5,$6,$7,$8) returning id::text`, event.ProjectID, event.TeamID, reportID, version,
		completeness, contentJSON, gapsJSON, nullableInt64(createdBy)).Scan(&reportVersionID); err != nil {
		return err
	}
	if err := insertEvidence(ctx, tx, event.ProjectID, reportVersionID, taskVersionID, content, eventIDs); err != nil {
		return err
	}
	output := map[string]any{"reportId": reportID, "reportVersionId": reportVersionID, "version": version,
		"completeness": completeness, "dataGaps": content.DataGaps}
	now := processor.now().UTC()
	if _, err := tx.ExecContext(ctx, `update task_run_steps set status='succeeded',attempt_count=greatest(attempt_count,1),
		output_snapshot_json=$3,result_json=result_json||$3,execution_key=$4,started_at=coalesce(started_at,$5),finished_at=$5
		where project_id=$1 and id=$2`, event.ProjectID, payload.TaskRunStepID, output,
		fmt.Sprintf("task-run:%d:step:%d", payload.TaskRunID, payload.TaskRunStepID), now); err != nil {
		return err
	}
	continuation := map[string]any{"taskRunId": payload.TaskRunID, "to": "running", "completedStepId": payload.TaskRunStepID}
	_, err = tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json)
		values($1,$2,$3,'task_run.transitioned',$4) on conflict(event_id) do nothing`, event.ProjectID, event.TeamID,
		fmt.Sprintf("task-run:%d:report-step:%d:succeeded", payload.TaskRunID, payload.TaskRunStepID), continuation)
	return err
}

func loadContent(ctx context.Context, tx *sql.Tx, projectID, runID int, reportStepID, taskVersionID int64) (reportContent, []int, error) {
	content := reportContent{SchemaVersion: "task-report-v1", TaskRun: map[string]any{"id": runID, "taskVersionId": taskVersionID},
		Steps: []stepFact{}, Issues: []issueFact{}, Assets: []assetFact{}, DataGaps: []string{}}
	rows, err := tx.QueryContext(ctx, `select run_step.id,step.step_key,step.uses,run_step.status,
		coalesce(run_step.condition_result_json,'null'::jsonb),run_step.output_snapshot_json
		from task_run_steps run_step join task_steps step on step.id=run_step.task_step_id and step.project_id=run_step.project_id
		where run_step.project_id=$1 and run_step.task_run_id=$2 order by run_step.position`, projectID, runID)
	if err != nil {
		return content, nil, err
	}
	for rows.Next() {
		var fact stepFact
		if err := rows.Scan(&fact.ID, &fact.Key, &fact.Uses, &fact.Status, &fact.Condition, &fact.Output); err != nil {
			rows.Close()
			return content, nil, err
		}
		content.Steps = append(content.Steps, fact)
		if fact.ID != reportStepID && fact.Status != "succeeded" && fact.Status != "skipped" {
			content.DataGaps = append(content.DataGaps, fmt.Sprintf("step:%s:%s", fact.Key, fact.Status))
		}
	}
	if err := rows.Close(); err != nil {
		return content, nil, err
	}
	rows, err = tx.QueryContext(ctx, `select issue.id,issue.number,issue.title,issue.status,issue.priority,
		coalesce(last_event.body,''),coalesce(last_event.id,0)
		from issues issue left join lateral (
			select event.id,event.body from issue_events event where event.project_id=issue.project_id and event.issue_id=issue.id
			and event.event_type in('comment','status_changed','copilot.completed') order by event.created_at desc,event.id desc limit 1
		) last_event on true where issue.project_id=$1 and issue.task_run_id=$2 order by issue.number`, projectID, runID)
	if err != nil {
		return content, nil, err
	}
	eventIDs := []int{}
	for rows.Next() {
		var fact issueFact
		if err := rows.Scan(&fact.ID, &fact.Number, &fact.Title, &fact.Status, &fact.Priority, &fact.Conclusion, &fact.LastEventID); err != nil {
			rows.Close()
			return content, nil, err
		}
		content.Issues = append(content.Issues, fact)
		if fact.LastEventID > 0 {
			eventIDs = append(eventIDs, fact.LastEventID)
		}
	}
	if err := rows.Close(); err != nil {
		return content, nil, err
	}
	rows, err = tx.QueryContext(ctx, `select id,version,kind,coalesce(checksum_sha256,'') from assets
		where project_id=$1 and task_run_id=$2 and status='available' order by id`, projectID, runID)
	if err != nil {
		return content, nil, err
	}
	for rows.Next() {
		var fact assetFact
		if err := rows.Scan(&fact.ID, &fact.Version, &fact.Kind, &fact.Checksum); err != nil {
			rows.Close()
			return content, nil, err
		}
		content.Assets = append(content.Assets, fact)
	}
	return content, eventIDs, rows.Close()
}

func insertEvidence(ctx context.Context, tx *sql.Tx, projectID int, reportVersionID string, taskVersionID int64, content reportContent, eventIDs []int) error {
	refs := []struct {
		kind, id, version, href string
		assetID                 any
		checksum                any
	}{{"task_run", fmt.Sprint(content.TaskRun["id"]), "current", fmt.Sprintf("/projects/%d/tasks/runs/%v", projectID, content.TaskRun["id"]), nil, nil},
		{"task_version", fmt.Sprint(taskVersionID), "published", fmt.Sprintf("/projects/%d/tasks/versions/%d", projectID, taskVersionID), nil, nil}}
	for _, step := range content.Steps {
		refs = append(refs, struct {
			kind, id, version, href string
			assetID                 any
			checksum                any
		}{"step", fmt.Sprint(step.ID), step.Status, fmt.Sprintf("/projects/%d/tasks/runs/%v#step-%d", projectID, content.TaskRun["id"], step.ID), nil, nil})
	}
	for _, eventID := range eventIDs {
		refs = append(refs, struct {
			kind, id, version, href string
			assetID                 any
			checksum                any
		}{"event", fmt.Sprint(eventID), "immutable", fmt.Sprintf("/projects/%d/issues?event=%d", projectID, eventID), nil, nil})
	}
	for _, asset := range content.Assets {
		refs = append(refs, struct {
			kind, id, version, href string
			assetID                 any
			checksum                any
		}{"asset", fmt.Sprint(asset.ID), fmt.Sprint(asset.Version), fmt.Sprintf("/projects/%d/assets/%d", projectID, asset.ID), asset.ID, nullString(asset.Checksum)})
	}
	for _, ref := range refs {
		if _, err := tx.ExecContext(ctx, `insert into generated_report_evidence(
			project_id,report_version_id,evidence_type,evidence_id,evidence_version,asset_id,checksum_sha256,href)
			values($1,$2,$3,$4,$5,$6,$7,$8) on conflict(report_version_id,evidence_type,evidence_id,evidence_version) do nothing`,
			projectID, reportVersionID, ref.kind, ref.id, ref.version, ref.assetID, ref.checksum, ref.href); err != nil {
			return err
		}
	}
	return nil
}

func nullableInt64(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
