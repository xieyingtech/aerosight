package algorithm

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	ErrCallbackAuthentication = errors.New("algorithm callback authentication failed")
	ErrCallbackReplay         = errors.New("algorithm callback replay detected")
	ErrCallbackOrder          = errors.New("algorithm callback conflicts with terminal run state")
)

var callbackRunIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89aAbB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

type CallbackMetadata struct {
	ProviderID int64
	CallbackID string
	Timestamp  time.Time
	Token      string
	Signature  string
}

type CallbackRunAuth struct {
	ProviderID        int64
	CallbackTokenHash string
}

type CallbackPayload struct {
	ProviderID    int64           `json:"providerId"`
	ExternalJobID string          `json:"externalJobId"`
	Status        string          `json:"status"`
	Result        json.RawMessage `json:"result,omitempty"`
	ErrorCode     string          `json:"errorCode,omitempty"`
	ErrorMessage  string          `json:"errorMessage,omitempty"`
}

type CallbackAuthenticator struct {
	Now     func() time.Time
	MaxSkew time.Duration
}

func (authenticator CallbackAuthenticator) Verify(metadata CallbackMetadata, run CallbackRunAuth, body []byte) error {
	now := time.Now()
	if authenticator.Now != nil {
		now = authenticator.Now()
	}
	maxSkew := authenticator.MaxSkew
	if maxSkew <= 0 {
		maxSkew = 5 * time.Minute
	}
	if metadata.ProviderID <= 0 || metadata.ProviderID != run.ProviderID || metadata.CallbackID == "" || len(metadata.Token) < 32 || metadata.Signature == "" {
		return ErrCallbackAuthentication
	}
	if metadata.Timestamp.IsZero() || now.Sub(metadata.Timestamp) > maxSkew || metadata.Timestamp.Sub(now) > maxSkew {
		return ErrCallbackAuthentication
	}
	tokenHash := sha256.Sum256([]byte(metadata.Token))
	expectedTokenHash, err := hex.DecodeString(run.CallbackTokenHash)
	if err != nil || len(expectedTokenHash) != sha256.Size || subtle.ConstantTimeCompare(tokenHash[:], expectedTokenHash) != 1 {
		return ErrCallbackAuthentication
	}
	message := callbackSignatureMessage(metadata.Timestamp, metadata.CallbackID, body)
	signer := hmac.New(sha256.New, []byte(metadata.Token))
	_, _ = signer.Write(message)
	expectedSignature := signer.Sum(nil)
	providedSignature, err := hex.DecodeString(strings.TrimPrefix(metadata.Signature, "sha256="))
	if err != nil || subtle.ConstantTimeCompare(expectedSignature, providedSignature) != 1 {
		return ErrCallbackAuthentication
	}
	return nil
}

func SignCallback(token, callbackID string, timestamp time.Time, body []byte) string {
	signer := hmac.New(sha256.New, []byte(token))
	_, _ = signer.Write(callbackSignatureMessage(timestamp, callbackID, body))
	return "sha256=" + hex.EncodeToString(signer.Sum(nil))
}

func callbackSignatureMessage(timestamp time.Time, callbackID string, body []byte) []byte {
	return []byte(strconv.FormatInt(timestamp.Unix(), 10) + "." + callbackID + "." + string(body))
}

func NextCallbackRunStatus(current, callbackStatus string) (string, bool, error) {
	terminal := map[string]bool{"succeeded": true, "failed": true, "canceled": true, "timed_out": true}
	targets := map[string]string{"completed": "succeeded", "failed": "failed", "processing": "waiting_callback"}
	target, ok := targets[callbackStatus]
	if !ok {
		return current, false, fmt.Errorf("unsupported callback status %q", callbackStatus)
	}
	if terminal[current] {
		if current == target {
			return current, false, nil
		}
		return current, false, ErrCallbackOrder
	}
	allowed := map[string]bool{"running": true, "polling": true, "waiting_callback": true}
	if !allowed[current] {
		return current, false, fmt.Errorf("run in state %q cannot receive callbacks", current)
	}
	return target, target != current, nil
}

type CallbackHandler struct {
	db            *sql.DB
	store         RawResultStore
	detectionSink DetectionSink
	authenticator CallbackAuthenticator
}

func NewCallbackHandler(db *sql.DB, store RawResultStore, detectionSink ...DetectionSink) *CallbackHandler {
	var sink DetectionSink
	if len(detectionSink) > 0 {
		sink = detectionSink[0]
	}
	return &CallbackHandler{db: db, store: store, detectionSink: sink, authenticator: CallbackAuthenticator{MaxSkew: 5 * time.Minute}}
}

func (handler *CallbackHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	runID := strings.TrimPrefix(request.URL.Path, "/callbacks/algorithms/")
	if !callbackRunIDPattern.MatchString(runID) {
		http.Error(writer, "callback authentication failed", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, 16<<20))
	if err != nil || len(body) == 0 {
		http.Error(writer, "invalid callback body", http.StatusBadRequest)
		return
	}
	metadata, err := callbackMetadataFromHeaders(request.Header)
	if err != nil {
		http.Error(writer, "callback authentication failed", http.StatusUnauthorized)
		return
	}
	status, response, err := handler.process(request.Context(), runID, metadata, body)
	if err != nil {
		http.Error(writer, response, status)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_, _ = writer.Write([]byte(response))
}

func callbackMetadataFromHeaders(headers http.Header) (CallbackMetadata, error) {
	providerID, err := strconv.ParseInt(headers.Get("X-Aerosight-Provider-Id"), 10, 64)
	if err != nil {
		return CallbackMetadata{}, err
	}
	timestampUnix, err := strconv.ParseInt(headers.Get("X-Aerosight-Timestamp"), 10, 64)
	if err != nil {
		return CallbackMetadata{}, err
	}
	return CallbackMetadata{
		ProviderID: providerID, CallbackID: headers.Get("X-Aerosight-Callback-Id"),
		Timestamp: time.Unix(timestampUnix, 0), Token: headers.Get("X-Aerosight-Callback-Token"),
		Signature: headers.Get("X-Aerosight-Signature"),
	}, nil
}

func (handler *CallbackHandler) process(
	ctx context.Context, runID string, metadata CallbackMetadata, body []byte,
) (int, string, error) {
	tx, err := handler.db.BeginTx(ctx, nil)
	if err != nil {
		return 500, "callback persistence failed", err
	}
	defer tx.Rollback()
	var (
		projectID, teamID                       int
		providerID                              int64
		providerType                            string
		tokenHash, externalJobID, currentStatus string
		mappingJSON                             []byte
	)
	err = tx.QueryRowContext(ctx, `
		select run.project_id, run.team_id, provider.id, provider.provider_type, coalesce(run.callback_token_hash, ''),
		       coalesce(run.external_job_id, ''), run.status, version.output_mapping_json
		from algorithm_runs run
		join algorithm_definition_versions version on version.id = run.algorithm_definition_version_id and version.project_id = run.project_id
		join algorithm_definitions definition on definition.id = version.algorithm_definition_id and definition.project_id = run.project_id
		join algorithm_providers provider on provider.id = definition.provider_id and provider.project_id = run.project_id
		where run.id = $1 for update of run`, runID).Scan(
		&projectID, &teamID, &providerID, &providerType, &tokenHash, &externalJobID, &currentStatus, &mappingJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return 401, "callback authentication failed", ErrCallbackAuthentication
	}
	if err != nil {
		return 500, "callback persistence failed", err
	}
	if err := handler.authenticator.Verify(metadata, CallbackRunAuth{ProviderID: providerID, CallbackTokenHash: tokenHash}, body); err != nil {
		return 401, "callback authentication failed", err
	}
	capability, err := RequireEnabled(providerType)
	if err != nil || !capability.SupportsSignedCallbacks {
		return 409, "algorithm adapter does not accept callbacks", ErrAdapterUnavailable
	}
	var payload CallbackPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return 400, "invalid callback payload", err
	}
	if payload.ProviderID != providerID || payload.ProviderID != metadata.ProviderID || payload.ExternalJobID == "" || payload.ExternalJobID != externalJobID {
		return 401, "callback provider or job mismatch", ErrCallbackAuthentication
	}
	payloadHashValue := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(payloadHashValue[:])
	duplicate, err := recordCallbackReceipt(ctx, tx, projectID, teamID, runID, providerID, metadata.CallbackID, payload.ExternalJobID, payloadHash)
	if err != nil {
		return 409, "callback replay conflict", err
	}
	if duplicate {
		if err := tx.Commit(); err != nil {
			return 500, "callback persistence failed", err
		}
		return 200, `{"accepted":true,"duplicate":true}`, nil
	}
	nextStatus, changed, err := NextCallbackRunStatus(currentStatus, payload.Status)
	if err != nil {
		return 409, "callback conflicts with run state", err
	}
	if !changed {
		if err := tx.Commit(); err != nil {
			return 500, "callback persistence failed", err
		}
		return 200, `{"accepted":true,"duplicate":true}`, nil
	}
	if nextStatus == "succeeded" {
		var mapping Mapping
		if err := json.Unmarshal(mappingJSON, &mapping); err != nil {
			return 400, "invalid callback mapping", err
		}
		outcome, err := mapCompleted(payload.Result, mapping)
		if err != nil {
			return 422, "callback result mapping failed", err
		}
		processor := Processor{store: handler.store, detectionSink: handler.detectionSink}
		if err := processor.finishSucceeded(ctx, tx, projectID, runID, outcome); err != nil {
			return 500, "callback result storage failed", err
		}
	} else if nextStatus == "failed" {
		message := payload.ErrorMessage
		if message == "" {
			message = "algorithm provider reported failure"
		}
		_, err = tx.ExecContext(ctx, `update algorithm_runs set status='failed', error_code=nullif($2,''), error_message=left($3,2000), finished_at=now() where id=$1`, runID, payload.ErrorCode, message)
		if err == nil {
			err = completeTaskAlgorithmStep(ctx, tx, projectID, runID, "failed", payload.ErrorCode)
		}
	} else {
		_, err = tx.ExecContext(ctx, `update algorithm_runs set status='waiting_callback' where id=$1`, runID)
	}
	if err != nil {
		return 500, "callback persistence failed", err
	}
	if _, err := tx.ExecContext(ctx, `update algorithm_callback_receipts set disposition='applied' where provider_id=$1 and callback_id=$2`, providerID, metadata.CallbackID); err != nil {
		return 500, "callback persistence failed", err
	}
	if err := tx.Commit(); err != nil {
		return 500, "callback persistence failed", err
	}
	return 200, `{"accepted":true,"duplicate":false}`, nil
}

func recordCallbackReceipt(
	ctx context.Context, tx *sql.Tx, projectID, teamID int, runID string, providerID int64,
	callbackID, externalJobID, payloadHash string,
) (bool, error) {
	result, err := tx.ExecContext(ctx, `
		insert into algorithm_callback_receipts (
		  project_id, team_id, algorithm_run_id, provider_id, callback_id, external_job_id, payload_hash
		) values ($1,$2,$3,$4,$5,$6,$7) on conflict (provider_id, callback_id) do nothing`,
		projectID, teamID, runID, providerID, callbackID, externalJobID, payloadHash)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows == 1 {
		return false, err
	}
	var existingRunID, existingHash string
	if err := tx.QueryRowContext(ctx, `select algorithm_run_id, payload_hash from algorithm_callback_receipts where provider_id=$1 and callback_id=$2`, providerID, callbackID).Scan(&existingRunID, &existingHash); err != nil {
		return false, err
	}
	return classifyCallbackReplay(existingRunID, existingHash, runID, payloadHash)
}

func classifyCallbackReplay(existingRunID, existingHash, runID, payloadHash string) (bool, error) {
	if existingRunID != runID || existingHash != payloadHash {
		return false, ErrCallbackReplay
	}
	return true, nil
}
