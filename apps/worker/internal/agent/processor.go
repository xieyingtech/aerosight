package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"aerosight/worker/internal/credentials"
)

const issueCopilotPromptVersion = "issue-copilot-v1"

type JobProcessor struct {
	Database   *sql.DB
	AuthSecret string
	HTTPClient *http.Client
	Now        func() time.Time
}

type copilotJob struct {
	ID                                    string
	ProjectID, TeamID, IssueID, TaskRunID int
	TaskRunStepID                         int64
	ToolName, TriggerType                 string
}

type evidenceRef struct {
	Type, ID, Version, ObservedAt, Quality string
}

type issueContext struct {
	Number      int               `json:"number"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      string            `json:"status"`
	Priority    string            `json:"priority"`
	Labels      json.RawMessage   `json:"labels"`
	Detections  []json.RawMessage `json:"detections"`
}

type providerConfig struct {
	ID, BaseURL, ModelID, APIKey string
}

func (processor JobProcessor) Run(ctx context.Context, interval time.Duration) error {
	if processor.HTTPClient == nil {
		processor.HTTPClient = &http.Client{Timeout: 45 * time.Second}
	}
	if processor.Now == nil {
		processor.Now = func() time.Time { return time.Now().UTC() }
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		worked, err := processor.ProcessNext(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			// Individual jobs are finalized as failed; only database lifecycle errors escape.
			return err
		}
		if worked {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (processor JobProcessor) ProcessNext(ctx context.Context) (bool, error) {
	tx, err := processor.Database.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var job copilotJob
	err = tx.QueryRowContext(ctx, `select job.id,job.project_id,job.team_id,coalesce(job.issue_id,0),job.tool_name,
		coalesce(job.trigger_type,''),coalesce(session.task_run_id,0),coalesce((job.args_json->>'taskRunStepId')::bigint,0)
		from agent_tool_jobs job join agent_sessions session on session.id=job.session_id and session.project_id=job.project_id
		where job.status='queued' order by job.created_at,job.id for update of job skip locked limit 1`).Scan(
		&job.ID, &job.ProjectID, &job.TeamID, &job.IssueID, &job.ToolName, &job.TriggerType, &job.TaskRunID, &job.TaskRunStepID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	now := processor.now()
	if err := RevalidateQueuedJob(ctx, tx, job.ID, now); err != nil {
		return false, err
	}
	var status, failureCode string
	if err := tx.QueryRowContext(ctx, `select status,coalesce(failure_code,'') from agent_tool_jobs where id=$1`, job.ID).Scan(&status, &failureCode); err != nil {
		return false, err
	}
	if status == "failed" {
		if err := appendIssueActivity(ctx, tx, job, "copilot.failed", map[string]any{"jobId": job.ID, "code": failureCode}); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	if err := appendIssueActivity(ctx, tx, job, "copilot.accepted", map[string]any{"jobId": job.ID}); err != nil {
		return false, err
	}
	if err := appendIssueActivity(ctx, tx, job, "copilot.progress", map[string]any{"jobId": job.ID, "stage": "collecting_evidence"}); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}

	draftID, result, refs, modelID, evidenceHash, executeErr := processor.executeIssueCopilot(ctx, job)
	if executeErr != nil {
		return true, processor.fail(ctx, job, failureCodeFor(executeErr))
	}
	return true, processor.succeed(ctx, job, draftID, result, refs, modelID, evidenceHash)
}

func (processor JobProcessor) executeIssueCopilot(ctx context.Context, job copilotJob) (string, string, []evidenceRef, string, string, error) {
	if job.ToolName != "issue_copilot" || job.IssueID == 0 {
		return "", "", nil, "", "", errors.New("UNSUPPORTED_AGENT_JOB")
	}
	issue, refs, err := loadIssueEvidence(ctx, processor.Database, job.ProjectID, job.IssueID)
	if err != nil {
		return "", "", nil, "", "", err
	}
	if len(refs) == 0 {
		return "", "", nil, "", "", errors.New("COPILOT_EVIDENCE_REQUIRED")
	}
	provider, err := processor.loadProvider(ctx)
	if err != nil {
		return "", "", nil, "", "", err
	}
	contextJSON, _ := json.Marshal(issue)
	prompt := "你是项目案件 Copilot。仅根据给定证据总结事实、风险、数据缺口和建议的后续人工复核步骤。不得声称已执行设备命令、算法或任务。输出简洁中文。\n案件证据：" + string(contextJSON)
	content, err := processor.complete(ctx, provider, prompt)
	if err != nil {
		return "", "", nil, "", "", err
	}
	hash := evidenceVersionHash(refs)
	return randomID(), content, refs, provider.ModelID, hash, nil
}

func loadIssueEvidence(ctx context.Context, database *sql.DB, projectID, issueID int) (issueContext, []evidenceRef, error) {
	var issue issueContext
	if err := database.QueryRowContext(ctx, `select number,title,coalesce(description,''),status,priority,labels_json
		from issues where project_id=$1 and id=$2`, projectID, issueID).Scan(
		&issue.Number, &issue.Title, &issue.Description, &issue.Status, &issue.Priority, &issue.Labels); err != nil {
		return issue, nil, err
	}
	rows, err := database.QueryContext(ctx, `select detection.id::text,coalesce(detection.captured_at,issue.created_at)::text,
		jsonb_build_object('id',detection.id,'label',detection.label,'confidence',detection.confidence,
		'locationQuality',detection.location_quality,'assetId',detection.input_asset_id),
		coalesce(asset.version,1)::text,asset.id::text
		from issues issue join issue_links link on link.project_id=issue.project_id and link.issue_id=issue.id and link.link_type='detection'
		join detections detection on detection.project_id=link.project_id and detection.id=link.target_id::bigint
		left join assets asset on asset.project_id=detection.project_id and asset.id=detection.input_asset_id
		where issue.project_id=$1 and issue.id=$2 order by detection.id`, projectID, issueID)
	if err != nil {
		return issue, nil, err
	}
	defer rows.Close()
	refs := make([]evidenceRef, 0)
	for rows.Next() {
		var detectionID, observedAt, assetVersion, assetID string
		var detection json.RawMessage
		if err := rows.Scan(&detectionID, &observedAt, &detection, &assetVersion, &assetID); err != nil {
			return issue, nil, err
		}
		issue.Detections = append(issue.Detections, detection)
		refs = append(refs, evidenceRef{Type: "detection", ID: detectionID, Version: "canonical-v1", ObservedAt: observedAt, Quality: "recorded"})
		if assetID != "" {
			refs = append(refs, evidenceRef{Type: "asset", ID: assetID, Version: assetVersion, ObservedAt: observedAt, Quality: "immutable"})
		}
	}
	return issue, refs, rows.Err()
}

func (processor JobProcessor) loadProvider(ctx context.Context) (providerConfig, error) {
	var provider providerConfig
	var envelopeJSON []byte
	var providerType string
	err := processor.Database.QueryRowContext(ctx, `select id::text,provider_type,coalesce(base_url,''),model_id,credential_envelope_json
		from ai_providers where enabled and is_default limit 2`).Scan(
		&provider.ID, &providerType, &provider.BaseURL, &provider.ModelID, &envelopeJSON)
	if err != nil {
		return provider, errors.New("AI_PROVIDER_UNAVAILABLE")
	}
	if providerType != "openai" {
		return provider, errors.New("AI_PROVIDER_CONFIGURATION_INVALID")
	}
	var envelope credentials.Envelope
	if err := json.Unmarshal(envelopeJSON, &envelope); err != nil {
		return provider, errors.New("AI_PROVIDER_CREDENTIAL_UNAVAILABLE")
	}
	var secret struct {
		APIKey string `json:"apiKey"`
	}
	if err := credentials.DecryptJSON(envelope, processor.AuthSecret, credentials.AAD("ai-provider", provider.ID, nil), &secret); err != nil || secret.APIKey == "" {
		return provider, errors.New("AI_PROVIDER_CREDENTIAL_UNAVAILABLE")
	}
	provider.APIKey = secret.APIKey
	if provider.BaseURL == "" {
		provider.BaseURL = "https://api.openai.com/v1"
	}
	return provider, nil
}

func (processor JobProcessor) complete(ctx context.Context, provider providerConfig, prompt string) (string, error) {
	body, _ := json.Marshal(map[string]any{"model": provider.ModelID, "messages": []map[string]string{{"role": "user", "content": prompt}}, "temperature": 0.2})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(provider.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("authorization", "Bearer "+provider.APIKey)
	request.Header.Set("content-type", "application/json")
	response, err := processor.HTTPClient.Do(request)
	if err != nil {
		return "", errors.New("MODEL_REQUEST_FAILED")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, response.Body)
		return "", errors.New("MODEL_REQUEST_FAILED")
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&decoded); err != nil || len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return "", errors.New("MODEL_RESPONSE_INVALID")
	}
	return strings.TrimSpace(decoded.Choices[0].Message.Content), nil
}

func (processor JobProcessor) succeed(ctx context.Context, job copilotJob, draftID, content string, refs []evidenceRef, modelID, evidenceHash string) error {
	tx, err := processor.Database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	resultPayload, _ := json.Marshal(map[string]any{"issueId": job.IssueID, "analysis": content, "requiresConfirmation": true})
	if _, err := tx.ExecContext(ctx, `insert into agent_drafts(id,project_id,team_id,session_id,created_by_user_id,draft_type,status,title,payload_json,
		model_id,prompt_template_version,generation_tool_calls_json,evidence_version_hash,generated_at)
		select $1,$2,$3,session_id,requested_by_user_id,'report','draft',$4,$5,$6,$7,'[]'::jsonb,$8,now()
		from agent_tool_jobs where id=$9 and project_id=$2 and status='running'`,
		draftID, job.ProjectID, job.TeamID, fmt.Sprintf("案件 #%d Copilot 分析草案", job.IssueID), resultPayload, modelID, issueCopilotPromptVersion, evidenceHash, job.ID); err != nil {
		return err
	}
	for _, ref := range refs {
		if _, err := tx.ExecContext(ctx, `insert into agent_draft_evidence(project_id,agent_draft_id,reference_type,reference_id,reference_version,observed_at,quality)
			values($1,$2,$3,$4,$5,$6,$7) on conflict do nothing`, job.ProjectID, draftID, ref.Type, ref.ID, ref.Version, ref.ObservedAt, ref.Quality); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `update agent_tool_jobs set status='succeeded',result_json=jsonb_build_object('draftId',$2::text),finished_at=now()
		where id=$1 and status='running'`, job.ID, draftID); err != nil {
		return err
	}
	if job.TriggerType == "task_step" && job.TaskRunStepID > 0 {
		output, _ := json.Marshal(map[string]any{"draftId": draftID, "issueId": job.IssueID, "status": "draft"})
		if _, err := tx.ExecContext(ctx, `update task_run_steps set status='succeeded',output_snapshot_json=$3,result_json=result_json||$3,
			execution_key=$4,finished_at=now() where project_id=$1 and id=$2 and status='running'`,
			job.ProjectID, job.TaskRunStepID, output, fmt.Sprintf("task-run:%d:step:%d", job.TaskRunID, job.TaskRunStepID)); err != nil {
			return err
		}
		if err := enqueueTaskContinuation(ctx, tx, job, "succeeded"); err != nil {
			return err
		}
	}
	if err := appendIssueActivity(ctx, tx, job, "copilot.completed", map[string]any{"jobId": job.ID, "draftId": draftID, "modelId": modelID,
		"promptTemplateVersion": issueCopilotPromptVersion, "evidenceVersionHash": evidenceHash, "toolCalls": []any{}}); err != nil {
		return err
	}
	return tx.Commit()
}

func (processor JobProcessor) fail(ctx context.Context, job copilotJob, code string) error {
	tx, err := processor.Database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `update agent_tool_jobs set status='failed',failure_code=$2,finished_at=now() where id=$1 and status='running'`, job.ID, code); err != nil {
		return err
	}
	if job.TriggerType == "task_step" && job.TaskRunStepID > 0 {
		if _, err := tx.ExecContext(ctx, `update task_run_steps set status='failed',result_json=result_json||jsonb_build_object('failureCode',$3::text),finished_at=now()
			where project_id=$1 and id=$2 and status='running'`, job.ProjectID, job.TaskRunStepID, code); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `update task_runs set status='failed',state_reason='copilot_step_failed',state_version=state_version+1,finished_at=now()
			where project_id=$1 and id=$2 and status in('running','dispatching')`, job.ProjectID, job.TaskRunID); err != nil {
			return err
		}
	}
	if err := appendIssueActivity(ctx, tx, job, "copilot.failed", map[string]any{"jobId": job.ID, "code": code}); err != nil {
		return err
	}
	return tx.Commit()
}

func enqueueTaskContinuation(ctx context.Context, tx *sql.Tx, job copilotJob, outcome string) error {
	payload, _ := json.Marshal(map[string]any{"taskRunId": job.TaskRunID, "to": "running", "completedStepId": job.TaskRunStepID})
	_, err := tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json)
		values($1,$2,$3,'task_run.transitioned',$4) on conflict(event_id) do nothing`, job.ProjectID, job.TeamID,
		fmt.Sprintf("task-run:%d:copilot-step:%d:%s", job.TaskRunID, job.TaskRunStepID, outcome), payload)
	return err
}

func appendIssueActivity(ctx context.Context, tx *sql.Tx, job copilotJob, eventType string, metadata map[string]any) error {
	if job.IssueID == 0 {
		return nil
	}
	encoded, _ := json.Marshal(metadata)
	if _, err := tx.ExecContext(ctx, `insert into issue_events(project_id,issue_id,event_type,metadata_json)
		values($1,$2,$3,$4)`, job.ProjectID, job.IssueID, eventType, encoded); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `insert into project_events(project_id,team_id,event_id,event_type,payload_json)
		values($1,$2,$3,'issue.updated',jsonb_build_object('issueId',$4::int,'copilotEvent',$5::text)) on conflict(event_id) do nothing`,
		job.ProjectID, job.TeamID, fmt.Sprintf("agent-job:%s:%s", job.ID, eventType), job.IssueID, eventType)
	return err
}

func evidenceVersionHash(refs []evidenceRef) string {
	values := make([]string, 0, len(refs))
	for _, ref := range refs {
		values = append(values, ref.Type+":"+ref.ID+":"+ref.Version)
	}
	sort.Strings(values)
	digest := sha256.Sum256([]byte(strings.Join(values, "\n")))
	return hex.EncodeToString(digest[:])
}

func failureCodeFor(err error) string {
	value := err.Error()
	for _, code := range []string{"UNSUPPORTED_AGENT_JOB", "COPILOT_EVIDENCE_REQUIRED", "AI_PROVIDER_UNAVAILABLE", "AI_PROVIDER_CONFIGURATION_INVALID", "AI_PROVIDER_CREDENTIAL_UNAVAILABLE", "MODEL_REQUEST_FAILED", "MODEL_RESPONSE_INVALID"} {
		if value == code {
			return code
		}
	}
	return "COPILOT_EXECUTION_FAILED"
}

func (processor JobProcessor) now() time.Time {
	if processor.Now != nil {
		return processor.Now()
	}
	return time.Now().UTC()
}

func randomID() string {
	value := make([]byte, 16)
	_, _ = rand.Read(value)
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value)
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}
