package algorithm

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"aerosight/worker/internal/outbox"
)

type RawResultObject struct {
	Key            string
	ChecksumSHA256 string
}

type RawResultStore interface {
	PutRawResult(context.Context, string, io.Reader, string) (RawResultObject, error)
}

type Processor struct {
	client          HTTPDoer
	breaker         *CircuitBreaker
	store           RawResultStore
	callbackBaseURL string
}

func NewProcessor(client HTTPDoer, breaker *CircuitBreaker, store RawResultStore, callbackBaseURL ...string) *Processor {
	baseURL := ""
	if len(callbackBaseURL) > 0 {
		baseURL = strings.TrimRight(callbackBaseURL[0], "/")
	}
	return &Processor{client: client, breaker: breaker, store: store, callbackBaseURL: baseURL}
}

type transactionRecorder struct {
	tx        *sql.Tx
	projectID int
	teamID    int
}

func (recorder transactionRecorder) RecordAttempt(ctx context.Context, attempt Attempt) error {
	var responseStatus any
	if attempt.ResponseStatus != nil {
		responseStatus = *attempt.ResponseStatus
	}
	_, err := recorder.tx.ExecContext(ctx, `
		insert into algorithm_run_attempts (
		  project_id, team_id, algorithm_run_id, attempt, status, request_hash,
		  response_status, external_job_id, duration_ms, error_category, started_at, finished_at
		) values ($1, $2, $3, $4, $5, $6, $7, nullif($8, ''), $9, nullif($10, ''), $11, $12)
		on conflict (algorithm_run_id, attempt) do update
		set status = excluded.status, response_status = excluded.response_status,
		    external_job_id = excluded.external_job_id, duration_ms = excluded.duration_ms,
		    error_category = excluded.error_category, finished_at = excluded.finished_at`,
		recorder.projectID, recorder.teamID, attempt.RunID, attempt.Number, attempt.Status,
		attempt.RequestHash, responseStatus, attempt.ExternalJobID, attempt.Duration.Milliseconds(),
		attempt.ErrorCategory, attempt.StartedAt, attempt.FinishedAt)
	return err
}

func (processor *Processor) Handler(ctx context.Context, tx *sql.Tx, event outbox.Event) error {
	var payload struct {
		RunID string `json:"runId"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.RunID == "" {
		return errors.New("algorithm.run.requested payload requires runId")
	}
	var (
		endpoint       string
		providerType   string
		providerStatus string
		timeoutSeconds int
		inputJSON      []byte
		mappingJSON    []byte
		status         string
	)
	err := tx.QueryRowContext(ctx, `
		select provider.base_url, provider.provider_type, provider.status, provider.timeout_seconds,
		       run.input_snapshot_json, version.output_mapping_json, run.status
		from algorithm_runs run
		join algorithm_definition_versions version
		  on version.id = run.algorithm_definition_version_id and version.project_id = run.project_id
		join algorithm_definitions definition
		  on definition.id = version.algorithm_definition_id and definition.project_id = run.project_id
		join algorithm_providers provider
		  on provider.id = definition.provider_id and provider.project_id = run.project_id
		where run.id = $1 and run.project_id = $2 and run.team_id = $3
		for update of run`, payload.RunID, event.ProjectID, event.TeamID).Scan(
		&endpoint, &providerType, &providerStatus, &timeoutSeconds, &inputJSON, &mappingJSON, &status,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("algorithm run scope does not match outbox event")
	}
	if err != nil {
		return err
	}
	if status != "queued" {
		return nil
	}
	if _, err := RequireEnabled(providerType); err != nil {
		return processor.failRun(ctx, tx, payload.RunID, "provider_unavailable", err.Error())
	}
	if providerStatus != "active" {
		return processor.failRun(ctx, tx, payload.RunID, "provider_unavailable", "algorithm provider is not active")
	}
	var input Input
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return processor.failRun(ctx, tx, payload.RunID, "invalid_input_snapshot", err.Error())
	}
	if input.Definition.ExecutionMode == "callback" {
		var tokenHash string
		input, tokenHash, err = issueCallbackCredentials(input, processor.callbackBaseURL)
		if err != nil {
			return processor.failRun(ctx, tx, payload.RunID, "callback_unavailable", err.Error())
		}
		if _, err := tx.ExecContext(ctx, `update algorithm_runs set callback_token_hash=$2 where id=$1`, payload.RunID, tokenHash); err != nil {
			return err
		}
	}
	var mapping Mapping
	if err := json.Unmarshal(mappingJSON, &mapping); err != nil {
		return processor.failRun(ctx, tx, payload.RunID, "invalid_output_mapping", err.Error())
	}
	if _, err := tx.ExecContext(ctx, `
		update algorithm_runs set status = 'running', started_at = coalesce(started_at, now()),
		       error_code = null, error_message = null
		where id = $1`, payload.RunID); err != nil {
		return err
	}
	recorder := transactionRecorder{tx: tx, projectID: event.ProjectID, teamID: event.TeamID}
	adapter := NewHTTPJSONAdapter(processor.client, recorder, processor.breaker)
	outcome, executeErr := adapter.Execute(ctx, Request{
		Endpoint: endpoint, Input: input, Mapping: mapping, Timeout: time.Duration(timeoutSeconds) * time.Second,
	})
	if executeErr != nil {
		code := "provider_execution_failed"
		terminalStatus := "failed"
		if errors.Is(executeErr, context.DeadlineExceeded) {
			code, terminalStatus = "provider_timeout", "timed_out"
		} else if errors.Is(executeErr, ErrFormatDrift) {
			code = "provider_format_drift"
		} else if errors.Is(executeErr, ErrCircuitOpen) {
			code = "provider_circuit_open"
		}
		return processor.finishFailed(ctx, tx, event.ProjectID, payload.RunID, terminalStatus, code, executeErr.Error(), outcome)
	}
	if outcome.Kind == "accepted" || outcome.Kind == "waiting_callback" {
		nextStatus := "polling"
		if outcome.Kind == "waiting_callback" {
			nextStatus = "waiting_callback"
		}
		_, err := tx.ExecContext(ctx, `
			update algorithm_runs set status = $2, external_job_id = $3
			where id = $1 and status = 'running'`, payload.RunID, nextStatus, outcome.ExternalJobID)
		return err
	}
	return processor.finishSucceeded(ctx, tx, event.ProjectID, payload.RunID, outcome)
}

func issueCallbackCredentials(input Input, callbackBaseURL string) (Input, string, error) {
	callbackBaseURL = strings.TrimRight(callbackBaseURL, "/")
	if callbackBaseURL == "" || !strings.HasPrefix(callbackBaseURL, "https://") {
		return Input{}, "", errors.New("CALLBACK_PUBLIC_BASE_URL must be configured with HTTPS for callback runs")
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return Input{}, "", fmt.Errorf("generate callback token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	digest := sha256.Sum256([]byte(token))
	input.Callback = map[string]string{
		"url":   callbackBaseURL + "/callbacks/algorithms/" + input.RunID,
		"token": token,
	}
	return input, hex.EncodeToString(digest[:]), nil
}

func (processor *Processor) finishSucceeded(
	ctx context.Context, tx *sql.Tx, projectID int, runID string, outcome Outcome,
) error {
	object, err := processor.storeRaw(ctx, projectID, runID, outcome.Raw)
	if err != nil {
		return processor.failRun(ctx, tx, runID, "raw_result_storage_failed", err.Error())
	}
	canonical, err := json.Marshal(map[string]any{
		"kind": "completed", "detections": outcome.Detections,
		"mappingDiagnostics": outcome.MappingDiagnostics,
		"rawResult":          map[string]any{"objectKey": object.Key, "checksumSha256": object.ChecksumSHA256, "contentType": "application/json"},
	})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		update algorithm_runs
		set status = 'succeeded', raw_result_object_key = $2, raw_result_checksum_sha256 = $3,
		    canonical_result_json = $4, finished_at = now()
		where id = $1 and status in ('running', 'polling', 'waiting_callback')`, runID, object.Key, object.ChecksumSHA256, canonical)
	return err
}

func (processor *Processor) finishFailed(
	ctx context.Context, tx *sql.Tx, projectID int, runID, status, code, message string, outcome Outcome,
) error {
	canonical, err := json.Marshal(map[string]any{"mappingDiagnostics": outcome.MappingDiagnostics})
	if err != nil {
		return err
	}
	var object RawResultObject
	if len(outcome.Raw) > 0 {
		object, err = processor.storeRaw(ctx, projectID, runID, outcome.Raw)
		if err != nil {
			message += "; raw result storage failed: " + err.Error()
		}
	}
	_, err = tx.ExecContext(ctx, `
		update algorithm_runs
		set status = $2, error_code = $3, error_message = left($4, 2000),
		    canonical_result_json = $5, raw_result_object_key = nullif($6, ''),
		    raw_result_checksum_sha256 = nullif($7, ''), finished_at = now()
		where id = $1 and status = 'running'`, runID, status, code, message, canonical, object.Key, object.ChecksumSHA256)
	return err
}

func (processor *Processor) failRun(ctx context.Context, tx *sql.Tx, runID, code, message string) error {
	_, err := tx.ExecContext(ctx, `
		update algorithm_runs set status = 'failed', error_code = $2,
		       error_message = left($3, 2000), finished_at = now()
		where id = $1 and status in ('queued', 'running')`, runID, code, message)
	return err
}

func (processor *Processor) storeRaw(ctx context.Context, projectID int, runID string, raw []byte) (RawResultObject, error) {
	if len(raw) == 0 {
		return RawResultObject{}, errors.New("provider returned no raw result")
	}
	key := fmt.Sprintf("projects/%d/algorithm-runs/%s/raw-result.json", projectID, runID)
	if processor.store != nil {
		return processor.store.PutRawResult(ctx, key, bytes.NewReader(raw), "application/json")
	}
	return RawResultObject{}, errors.New("raw result object storage is unavailable")
}

func DefaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("algorithm provider redirects must be explicitly revalidated")
		},
	}
}
