package flighthub

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/credentials"
	"aerosight/worker/internal/outbox"
)

const (
	OpenModelUploadCredentialEventType = "flighthub.open_model_upload.credential_requested"
	OpenModelUploadCallbackEventType   = "flighthub.open_model_upload.callback_requested"
)

type OpenModelUploadRequest struct {
	ProjectID, RequestedByUserID int
	ConnectorInstanceID          int64
	IdempotencyKey               string
	ResourceUUID                 string
	ResourceName                 string
}

type OpenModelUploadCompletionRequest struct {
	ProjectID    int
	SessionID    string
	ResourceUUID string
	ResourceName string
	Files        []OpenModelUploadedFile
}

type OpenModelUploadSession struct {
	ID, IdempotencyKey, RequestDigest, ResourceUUIDDigest, Status, CallbackDigest, LastErrorCode string
	ProjectID, TeamID, RequestedByUserID, CallbackAttempts, ReconciliationCount                  int
	ConnectorInstanceID                                                                          int64
	RequestEnvelope, CredentialEnvelope, CallbackEnvelope                                        json.RawMessage
	CredentialExpiresAt                                                                          time.Time
	ConnectorStatus                                                                              string
	ActionEnabled                                                                                bool
	Instance                                                                                     connector.Instance
}

type OpenModelUploadStore interface {
	Create(context.Context, OpenModelUploadRequest, json.RawMessage, string, string) (OpenModelUploadSession, error)
	Load(context.Context, int, string) (OpenModelUploadSession, error)
	SaveCredential(context.Context, OpenModelUploadSession, json.RawMessage, time.Time) error
	PrepareCallback(context.Context, OpenModelUploadSession, json.RawMessage, string, string) error
	BeginCallback(context.Context, OpenModelUploadSession) error
	RecordReconciliation(context.Context, OpenModelUploadSession, string) error
	Complete(context.Context, OpenModelUploadSession, string) error
	Finish(context.Context, OpenModelUploadSession, string, string) error
}

type SQLOpenModelUploadStore struct{ db *sql.DB }

func NewSQLOpenModelUploadStore(database *sql.DB) *SQLOpenModelUploadStore {
	return &SQLOpenModelUploadStore{db: database}
}

func openModelUploadHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func normalizeOpenModelUploadRequest(input OpenModelUploadRequest) (OpenModelUploadRequest, error) {
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.ResourceUUID = strings.TrimSpace(input.ResourceUUID)
	input.ResourceName = strings.TrimSpace(input.ResourceName)
	if input.ProjectID <= 0 || input.ConnectorInstanceID <= 0 || input.RequestedByUserID <= 0 ||
		len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 200 ||
		!validModelString(input.ResourceUUID, 256, false) || !validModelString(input.ResourceName, 255, true) {
		return input, &APIError{SafeCode: "request_invalid"}
	}
	return input, nil
}

func (store *SQLOpenModelUploadStore) Create(ctx context.Context, request OpenModelUploadRequest, envelope json.RawMessage, requestDigest, resourceDigest string) (session OpenModelUploadSession, returnedErr error) {
	if store == nil || store.db == nil {
		return session, errors.New("FlightHub open model upload store is unavailable")
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return session, err
	}
	defer func() {
		if returnedErr != nil {
			_ = tx.Rollback()
		}
	}()
	var teamID int
	err = tx.QueryRowContext(ctx, `select adapter.team_id from device_adapters adapter
		join connector_definitions definition on definition.id=adapter.connector_definition_id
		join team_members member on member.team_id=adapter.team_id and member.user_id=$3
		join project_feature_flags flags on flags.project_id=adapter.project_id
		where adapter.id=$1 and adapter.project_id=$2 and definition.connector_key=$4 and definition.version=$5
		and adapter.status in('connecting','connected','degraded')
		and flags.flighthub_action_flags_json @> '{"security.temporary-credential":true,"model.write":true}'::jsonb`,
		request.ConnectorInstanceID, request.ProjectID, request.RequestedByUserID, ConnectorKey, ConnectorVersion).Scan(&teamID)
	if errors.Is(err, sql.ErrNoRows) {
		return session, &APIError{SafeCode: "scope_forbidden"}
	}
	if err != nil {
		return session, err
	}
	err = tx.QueryRowContext(ctx, `insert into connector_open_model_uploads(project_id,team_id,connector_instance_id,
		requested_by_user_id,idempotency_key,request_digest,request_envelope_json,resource_uuid_digest)
		values($1,$2,$3,$4,$5,$6,$7,$8)
		on conflict(project_id,connector_instance_id,idempotency_key) do nothing returning id::text`, request.ProjectID, teamID,
		request.ConnectorInstanceID, request.RequestedByUserID, request.IdempotencyKey, requestDigest, envelope, resourceDigest).Scan(&session.ID)
	if errors.Is(err, sql.ErrNoRows) {
		var existingDigest string
		err = tx.QueryRowContext(ctx, `select id::text,request_digest from connector_open_model_uploads
			where project_id=$1 and connector_instance_id=$2 and idempotency_key=$3`, request.ProjectID,
			request.ConnectorInstanceID, request.IdempotencyKey).Scan(&session.ID, &existingDigest)
		if err != nil {
			return session, err
		}
		if existingDigest != requestDigest {
			return session, &APIError{SafeCode: "idempotency_conflict"}
		}
	} else if err != nil {
		return session, err
	}
	payload, _ := json.Marshal(map[string]string{"sessionId": session.ID})
	_, err = tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,aggregate_type,aggregate_id,payload_json,max_attempts)
		values($1,$2,$3,$4,'flighthub_open_model_upload',$5,$6,16) on conflict(event_id) do nothing`, request.ProjectID, teamID,
		"flighthub-open-model-credential:"+session.ID, OpenModelUploadCredentialEventType, session.ID, payload)
	if err != nil {
		return session, err
	}
	if err = tx.Commit(); err != nil {
		return session, err
	}
	return store.Load(ctx, request.ProjectID, session.ID)
}

func (store *SQLOpenModelUploadStore) Load(ctx context.Context, projectID int, sessionID string) (session OpenModelUploadSession, err error) {
	var requestRaw, credentialRaw, callbackRaw []byte
	err = store.db.QueryRowContext(ctx, `select upload.id::text,upload.project_id,upload.team_id,upload.connector_instance_id,
		upload.requested_by_user_id,upload.idempotency_key,upload.request_digest,upload.resource_uuid_digest,upload.status,
		upload.request_envelope_json,coalesce(upload.credential_envelope_json,'null'::jsonb),
		coalesce(upload.callback_envelope_json,'null'::jsonb),coalesce(upload.callback_digest,''),
		coalesce(upload.credential_expires_at,'epoch'::timestamptz),upload.callback_attempt_count,
		upload.reconciliation_count,coalesce(upload.last_error_code,''),adapter.status,
		coalesce(flags.flighthub_action_flags_json @> '{"security.temporary-credential":true,"model.write":true}'::jsonb,false),
		definition.connector_key,definition.version,adapter.credential_envelope_json,adapter.discovery_scope_json
		from connector_open_model_uploads upload
		join device_adapters adapter on adapter.id=upload.connector_instance_id and adapter.project_id=upload.project_id
		join connector_definitions definition on definition.id=adapter.connector_definition_id
		left join project_feature_flags flags on flags.project_id=upload.project_id
		where upload.id=$1::uuid and upload.project_id=$2`, sessionID, projectID).Scan(&session.ID, &session.ProjectID,
		&session.TeamID, &session.ConnectorInstanceID, &session.RequestedByUserID, &session.IdempotencyKey,
		&session.RequestDigest, &session.ResourceUUIDDigest, &session.Status, &requestRaw, &credentialRaw, &callbackRaw,
		&session.CallbackDigest, &session.CredentialExpiresAt, &session.CallbackAttempts, &session.ReconciliationCount,
		&session.LastErrorCode, &session.ConnectorStatus, &session.ActionEnabled, &session.Instance.ConnectorKey,
		&session.Instance.Version, &session.Instance.CredentialEnvelope, &session.Instance.DiscoveryScope)
	if errors.Is(err, sql.ErrNoRows) {
		return session, &APIError{SafeCode: "scope_forbidden"}
	}
	session.RequestEnvelope, session.CredentialEnvelope, session.CallbackEnvelope = requestRaw, credentialRaw, callbackRaw
	session.Instance.ID, session.Instance.ProjectID = session.ConnectorInstanceID, session.ProjectID
	return session, err
}

func (store *SQLOpenModelUploadStore) updateOne(ctx context.Context, query string, args ...any) error {
	result, err := store.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return errors.New("FlightHub open model upload state changed concurrently")
	}
	return nil
}

func (store *SQLOpenModelUploadStore) SaveCredential(ctx context.Context, session OpenModelUploadSession, envelope json.RawMessage, expiresAt time.Time) error {
	return store.updateOne(ctx, `update connector_open_model_uploads set status='credential_ready',credential_envelope_json=$3,
		credential_expires_at=$4,credential_issued_at=now(),last_error_code=null,updated_at=now()
		where id=$1::uuid and project_id=$2 and status='requested'`, session.ID, session.ProjectID, envelope, expiresAt)
}

func (store *SQLOpenModelUploadStore) PrepareCallback(ctx context.Context, session OpenModelUploadSession, envelope json.RawMessage, digest, resourceDigest string) (returnedErr error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if returnedErr != nil {
			_ = tx.Rollback()
		}
	}()
	var storedResource, status, storedDigest string
	var expiresAt time.Time
	err = tx.QueryRowContext(ctx, `select resource_uuid_digest,status,coalesce(callback_digest,''),
		coalesce(credential_expires_at,'epoch'::timestamptz) from connector_open_model_uploads
		where id=$1::uuid and project_id=$2 for update`, session.ID, session.ProjectID).Scan(&storedResource, &status, &storedDigest, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return &APIError{SafeCode: "scope_forbidden"}
	}
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(storedResource), []byte(resourceDigest)) != 1 {
		return &APIError{SafeCode: "scope_forbidden"}
	}
	if storedDigest != "" && storedDigest != digest {
		return &APIError{SafeCode: "idempotency_conflict"}
	}
	if status == "expired" {
		return &APIError{SafeCode: "credential_expired"}
	}
	if status == "failed" || status == "blocked" {
		return &APIError{SafeCode: "request_invalid"}
	}
	if storedDigest == "" {
		if !expiresAt.After(time.Now()) {
			_, err = tx.ExecContext(ctx, `update connector_open_model_uploads set status='expired',credential_envelope_json=null,
				last_error_code='credential_expired',updated_at=now() where id=$1::uuid and project_id=$2`, session.ID, session.ProjectID)
			if err != nil {
				return err
			}
			if err = tx.Commit(); err != nil {
				return err
			}
			return &APIError{SafeCode: "credential_expired"}
		}
		if status != "credential_ready" {
			return &APIError{SafeCode: "request_invalid"}
		}
		_, err = tx.ExecContext(ctx, `update connector_open_model_uploads set status='callback_pending',callback_digest=$3,
			callback_envelope_json=$4,updated_at=now() where id=$1::uuid and project_id=$2`, session.ID, session.ProjectID, digest, envelope)
		if err != nil {
			return err
		}
	}
	payload, _ := json.Marshal(map[string]string{"sessionId": session.ID})
	_, err = tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,aggregate_type,aggregate_id,payload_json,max_attempts)
		values($1,$2,$3,$4,'flighthub_open_model_upload',$5,$6,16) on conflict(event_id) do nothing`, session.ProjectID,
		session.TeamID, "flighthub-open-model-callback:"+session.ID, OpenModelUploadCallbackEventType, session.ID, payload)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (store *SQLOpenModelUploadStore) BeginCallback(ctx context.Context, session OpenModelUploadSession) error {
	return store.updateOne(ctx, `update connector_open_model_uploads set status='reconciling',callback_attempt_count=1,
		callback_attempted_at=now(),last_error_code=null,updated_at=now()
		where id=$1::uuid and project_id=$2 and status='callback_pending' and callback_attempt_count=0`, session.ID, session.ProjectID)
}

func (store *SQLOpenModelUploadStore) RecordReconciliation(ctx context.Context, session OpenModelUploadSession, code string) error {
	return store.updateOne(ctx, `update connector_open_model_uploads set reconciliation_count=reconciliation_count+1,
		last_error_code=$3,reconciled_at=now(),updated_at=now() where id=$1::uuid and project_id=$2
		and status='reconciling' and reconciliation_count<16`, session.ID, session.ProjectID, code)
}

func (store *SQLOpenModelUploadStore) Complete(ctx context.Context, session OpenModelUploadSession, resourceUUID string) error {
	return store.updateOne(ctx, `update connector_open_model_uploads upload set status='succeeded',remote_resource_id=resource.id,
		asset_id=resource.canonical_target_id::integer,credential_envelope_json=null,callback_envelope_json=null,
		last_error_code=null,reconciled_at=now(),completed_at=now(),updated_at=now()
		from connector_remote_resources resource where upload.id=$1::uuid and upload.project_id=$2 and upload.status='reconciling'
		and resource.project_id=upload.project_id and resource.connector_instance_id=upload.connector_instance_id
		and resource.resource_kind='model-resource' and resource.remote_id=$3 and resource.status='active'
		and resource.canonical_target_type='asset' and resource.canonical_target_id ~ '^[1-9][0-9]*$'`,
		session.ID, session.ProjectID, "resource:"+resourceUUID)
}

func (store *SQLOpenModelUploadStore) Finish(ctx context.Context, session OpenModelUploadSession, status, code string) error {
	if status != "expired" && status != "failed" && status != "blocked" {
		return &APIError{SafeCode: "request_invalid"}
	}
	return store.updateOne(ctx, `update connector_open_model_uploads set status=$3,last_error_code=$4,
		credential_envelope_json=null,callback_envelope_json=null,reconciled_at=now(),updated_at=now()
		where id=$1::uuid and project_id=$2 and status not in('succeeded','expired','failed','blocked')`, session.ID, session.ProjectID, status, code)
}

type OpenModelUploadClient interface {
	ObtainOpenModelUploadCredential(context.Context, string, string) (OpenModelUploadCredential, error)
	NotifyOpenModelUploadComplete(context.Context, string, string, OpenModelUploadCallbackRequest) (OpenModelUploadCallbackResult, error)
	GetOpenModelResource(context.Context, string, string, string) (OpenModelResource, error)
}

type OpenModelUploadHandler struct {
	store      OpenModelUploadStore
	client     OpenModelUploadClient
	resolver   TokenResolver
	projector  ModelJobProjector
	authSecret string
	now        func() time.Time
}

func NewOpenModelUploadHandler(store OpenModelUploadStore, client OpenModelUploadClient, resolver TokenResolver, projector ModelJobProjector, authSecret string, now func() time.Time) (*OpenModelUploadHandler, error) {
	if store == nil || client == nil || resolver == nil || projector == nil || strings.TrimSpace(authSecret) == "" {
		return nil, errors.New("FlightHub open model upload dependencies are required")
	}
	if now == nil {
		now = time.Now
	}
	return &OpenModelUploadHandler{store: store, client: client, resolver: resolver, projector: projector, authSecret: authSecret, now: now}, nil
}

func (handler *OpenModelUploadHandler) RequestCredential(ctx context.Context, input OpenModelUploadRequest) (OpenModelUploadSession, error) {
	input, err := normalizeOpenModelUploadRequest(input)
	if err != nil {
		return OpenModelUploadSession{}, err
	}
	raw, _ := json.Marshal(map[string]string{"resourceUuid": input.ResourceUUID, "resourceName": input.ResourceName})
	envelope, err := credentials.EncryptJSON(json.RawMessage(raw), handler.authSecret,
		credentials.AAD("flighthub-open-model-upload-request", input.ConnectorInstanceID, input.ProjectID))
	if err != nil {
		return OpenModelUploadSession{}, err
	}
	envelopeRaw, _ := json.Marshal(envelope)
	return handler.store.Create(ctx, input, envelopeRaw, openModelUploadHash(raw), openModelUploadHash([]byte(input.ResourceUUID)))
}

func (handler *OpenModelUploadHandler) Credential(ctx context.Context, projectID int, sessionID string) (OpenModelUploadCredential, error) {
	session, err := handler.store.Load(ctx, projectID, sessionID)
	if err != nil {
		return OpenModelUploadCredential{}, err
	}
	if !session.ActionEnabled || !isActiveConnectorStatus(session.ConnectorStatus) {
		return OpenModelUploadCredential{}, &APIError{SafeCode: "scope_forbidden"}
	}
	if session.Status != "credential_ready" || !session.CredentialExpiresAt.After(handler.now()) {
		if session.Status == "credential_ready" {
			_ = handler.store.Finish(ctx, session, "expired", "credential_expired")
		}
		return OpenModelUploadCredential{}, &APIError{SafeCode: "credential_expired"}
	}
	var envelope credentials.Envelope
	if json.Unmarshal(session.CredentialEnvelope, &envelope) != nil {
		return OpenModelUploadCredential{}, &APIError{SafeCode: "credential_unavailable"}
	}
	var result OpenModelUploadCredential
	if err := credentials.DecryptJSON(envelope, handler.authSecret,
		credentials.AAD("flighthub-open-model-upload-credential", session.ID, session.ProjectID), &result); err != nil {
		return OpenModelUploadCredential{}, &APIError{SafeCode: "credential_unavailable"}
	}
	result.ExpiresAt = session.CredentialExpiresAt
	return result, nil
}

func (handler *OpenModelUploadHandler) SubmitCallback(ctx context.Context, input OpenModelUploadCompletionRequest) (OpenModelUploadSession, error) {
	if input.ProjectID <= 0 || strings.TrimSpace(input.SessionID) == "" {
		return OpenModelUploadSession{}, &APIError{SafeCode: "request_invalid"}
	}
	callback := OpenModelUploadCallbackRequest{ResourceUUID: strings.TrimSpace(input.ResourceUUID), ResourceName: strings.TrimSpace(input.ResourceName), Files: append([]OpenModelUploadedFile(nil), input.Files...)}
	callback.Callback = "SERVER_INJECTED_CALLBACK"
	if err := validateOpenModelUploadCallback(&callback); err != nil {
		return OpenModelUploadSession{}, err
	}
	callback.Callback = ""
	session, err := handler.store.Load(ctx, input.ProjectID, input.SessionID)
	if err != nil {
		return OpenModelUploadSession{}, err
	}
	resourceDigest := openModelUploadHash([]byte(callback.ResourceUUID))
	if subtle.ConstantTimeCompare([]byte(session.ResourceUUIDDigest), []byte(resourceDigest)) != 1 {
		return OpenModelUploadSession{}, &APIError{SafeCode: "scope_forbidden"}
	}
	raw, _ := json.Marshal(callback)
	envelope, err := credentials.EncryptJSON(json.RawMessage(raw), handler.authSecret,
		credentials.AAD("flighthub-open-model-upload-callback", session.ID, session.ProjectID))
	if err != nil {
		return OpenModelUploadSession{}, err
	}
	envelopeRaw, _ := json.Marshal(envelope)
	if err := handler.store.PrepareCallback(ctx, session, envelopeRaw, openModelUploadHash(raw), resourceDigest); err != nil {
		return OpenModelUploadSession{}, err
	}
	return handler.store.Load(ctx, input.ProjectID, input.SessionID)
}

func parseOpenModelUploadEvent(event outbox.Event) (string, error) {
	var payload struct {
		SessionID string `json:"sessionId"`
	}
	if event.ProjectID <= 0 || json.Unmarshal(event.Payload, &payload) != nil || payload.SessionID == "" {
		return "", &APIError{SafeCode: "request_invalid"}
	}
	return payload.SessionID, nil
}

func (handler *OpenModelUploadHandler) Handler(ctx context.Context, _ *sql.Tx, event outbox.Event) (returnedErr error) {
	sessionID, err := parseOpenModelUploadEvent(event)
	if err != nil {
		return err
	}
	session, err := handler.store.Load(ctx, event.ProjectID, sessionID)
	if err != nil {
		return err
	}
	if session.Status == "succeeded" || session.Status == "expired" || session.Status == "failed" || session.Status == "blocked" {
		return nil
	}
	finalAttempt := event.MaxAttempts > 0 && event.Attempts >= event.MaxAttempts
	defer func() {
		if returnedErr != nil && finalAttempt {
			if failErr := handler.store.Finish(ctx, session, "blocked", "upload_reconciliation_exhausted"); failErr != nil {
				returnedErr = errors.Join(returnedErr, failErr)
			} else {
				returnedErr = nil
			}
		}
	}()
	if session.ProjectID != event.ProjectID || (event.TeamID > 0 && session.TeamID != event.TeamID) ||
		session.Instance.ConnectorKey != ConnectorKey || session.Instance.Version != ConnectorVersion {
		return handler.store.Finish(ctx, session, "blocked", "scope_forbidden")
	}
	if event.EventType == OpenModelUploadCredentialEventType {
		return handler.acquireCredential(ctx, session)
	}
	if event.EventType != OpenModelUploadCallbackEventType {
		return &APIError{SafeCode: "request_invalid"}
	}
	return handler.callback(ctx, session)
}

func (handler *OpenModelUploadHandler) acquireCredential(ctx context.Context, session OpenModelUploadSession) error {
	if session.Status != "requested" {
		return nil
	}
	if !session.ActionEnabled || !isActiveConnectorStatus(session.ConnectorStatus) {
		return handler.store.Finish(ctx, session, "blocked", "action_disabled")
	}
	scope, err := parseScope(session.Instance.DiscoveryScope)
	if err != nil {
		return handler.store.Finish(ctx, session, "blocked", "scope_forbidden")
	}
	token, err := handler.resolver.ResolveToken(ctx, session.Instance)
	if err != nil {
		return modelJobRetry(SafeCode(err))
	}
	defer func() { token = "" }()
	credential, err := handler.client.ObtainOpenModelUploadCredential(ctx, token, scope.ProjectUUID)
	if err != nil {
		return modelJobRetry(SafeCode(err))
	}
	if !credential.ExpiresAt.After(handler.now().Add(30 * time.Second)) {
		return handler.store.Finish(ctx, session, "expired", "credential_expired")
	}
	expiresAt := credential.ExpiresAt
	envelope, err := credentials.EncryptJSON(credential, handler.authSecret,
		credentials.AAD("flighthub-open-model-upload-credential", session.ID, session.ProjectID))
	credential = OpenModelUploadCredential{}
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(envelope)
	return handler.store.SaveCredential(ctx, session, raw, expiresAt)
}

func (handler *OpenModelUploadHandler) decryptUploadRequestAndCallback(session OpenModelUploadSession) (OpenModelUploadRequest, OpenModelUploadCallbackRequest, error) {
	var requestEnvelope, callbackEnvelope credentials.Envelope
	if json.Unmarshal(session.RequestEnvelope, &requestEnvelope) != nil || json.Unmarshal(session.CallbackEnvelope, &callbackEnvelope) != nil {
		return OpenModelUploadRequest{}, OpenModelUploadCallbackRequest{}, &APIError{SafeCode: "credential_unavailable"}
	}
	var requestData struct {
		ResourceUUID string `json:"resourceUuid"`
		ResourceName string `json:"resourceName"`
	}
	var callback OpenModelUploadCallbackRequest
	if credentials.DecryptJSON(requestEnvelope, handler.authSecret, credentials.AAD("flighthub-open-model-upload-request", session.ConnectorInstanceID, session.ProjectID), &requestData) != nil ||
		credentials.DecryptJSON(callbackEnvelope, handler.authSecret, credentials.AAD("flighthub-open-model-upload-callback", session.ID, session.ProjectID), &callback) != nil {
		return OpenModelUploadRequest{}, OpenModelUploadCallbackRequest{}, &APIError{SafeCode: "credential_unavailable"}
	}
	return OpenModelUploadRequest{ResourceUUID: requestData.ResourceUUID, ResourceName: requestData.ResourceName}, callback, nil
}

func (handler *OpenModelUploadHandler) decryptUploadCredential(session OpenModelUploadSession) (OpenModelUploadCredential, error) {
	var envelope credentials.Envelope
	if json.Unmarshal(session.CredentialEnvelope, &envelope) != nil {
		return OpenModelUploadCredential{}, &APIError{SafeCode: "credential_unavailable"}
	}
	var credential OpenModelUploadCredential
	if credentials.DecryptJSON(envelope, handler.authSecret,
		credentials.AAD("flighthub-open-model-upload-credential", session.ID, session.ProjectID), &credential) != nil {
		return OpenModelUploadCredential{}, &APIError{SafeCode: "credential_unavailable"}
	}
	return credential, nil
}

func (handler *OpenModelUploadHandler) callback(ctx context.Context, session OpenModelUploadSession) error {
	if session.Status != "callback_pending" && session.Status != "reconciling" {
		return &APIError{SafeCode: "request_invalid"}
	}
	request, callback, err := handler.decryptUploadRequestAndCallback(session)
	if err != nil {
		return handler.store.Finish(ctx, session, "blocked", SafeCode(err))
	}
	if request.ResourceUUID != callback.ResourceUUID || openModelUploadHash([]byte(request.ResourceUUID)) != session.ResourceUUIDDigest {
		return handler.store.Finish(ctx, session, "blocked", "resource_scope_mismatch")
	}
	scope, err := parseScope(session.Instance.DiscoveryScope)
	if err != nil {
		return handler.store.Finish(ctx, session, "blocked", "scope_forbidden")
	}
	token, err := handler.resolver.ResolveToken(ctx, session.Instance)
	if err != nil {
		return modelJobRetry(SafeCode(err))
	}
	defer func() { token = "" }()
	if session.CallbackAttempts == 0 {
		if !session.CredentialExpiresAt.After(handler.now()) {
			return handler.store.Finish(ctx, session, "expired", "credential_expired")
		}
		if !session.ActionEnabled || !isActiveConnectorStatus(session.ConnectorStatus) {
			return handler.store.Finish(ctx, session, "blocked", "action_disabled")
		}
		credential, credentialErr := handler.decryptUploadCredential(session)
		if credentialErr != nil {
			return handler.store.Finish(ctx, session, "blocked", SafeCode(credentialErr))
		}
		defer func() { credential = OpenModelUploadCredential{} }()
		if err := handler.store.BeginCallback(ctx, session); err != nil {
			return err
		}
		session.Status, session.CallbackAttempts = "reconciling", 1
		callback.Callback = credential.CallbackParam
		result, callbackErr := handler.client.NotifyOpenModelUploadComplete(ctx, token, scope.ProjectUUID, callback)
		callback.Callback = ""
		if callbackErr != nil {
			if err := handler.store.RecordReconciliation(ctx, session, SafeCode(callbackErr)); err != nil {
				return err
			}
			return modelJobRetry(SafeCode(callbackErr))
		}
		return handler.completeUpload(ctx, session, callback, result)
	}
	resource, err := handler.client.GetOpenModelResource(ctx, token, scope.ProjectUUID, request.ResourceUUID)
	if err != nil || resource.Status == 2 {
		code := SafeCode(err)
		if code == "" {
			code = "upload_callback_pending"
		}
		if persistErr := handler.store.RecordReconciliation(ctx, session, code); persistErr != nil {
			return persistErr
		}
		return modelJobRetry(code)
	}
	if resource.Status != 1 || !sameOpenModelFiles(callback.Files, resource.FileNames) {
		return handler.store.Finish(ctx, session, "failed", "upload_callback_mismatch")
	}
	return handler.projectAndComplete(ctx, session, resource)
}

func sameOpenModelFiles(files []OpenModelUploadedFile, names []string) bool {
	if len(files) != len(names) {
		return false
	}
	want := make(map[string]struct{}, len(files))
	for _, file := range files {
		want[file.Name] = struct{}{}
	}
	for _, name := range names {
		if _, ok := want[name]; !ok {
			return false
		}
	}
	return true
}

func (handler *OpenModelUploadHandler) completeUpload(ctx context.Context, session OpenModelUploadSession, callback OpenModelUploadCallbackRequest, result OpenModelUploadCallbackResult) error {
	if result.ResourceUUID != callback.ResourceUUID {
		return handler.store.Finish(ctx, session, "blocked", "resource_scope_mismatch")
	}
	if !sameOpenModelFiles(callback.Files, result.FileNames) {
		return handler.store.Finish(ctx, session, "failed", "upload_callback_mismatch")
	}
	return handler.projectAndComplete(ctx, session, OpenModelResource{ResourceUUID: result.ResourceUUID, Status: 1, FileNames: result.FileNames})
}

func (handler *OpenModelUploadHandler) projectAndComplete(ctx context.Context, session OpenModelUploadSession, resource OpenModelResource) error {
	if err := handler.projector.ApplyModelCatalog(ctx, session.Instance, ModelCatalogPoll{Resources: []OpenModelResource{resource}, ReceivedAt: handler.now().UTC()}); err != nil {
		return err
	}
	return handler.store.Complete(ctx, session, resource.ResourceUUID)
}
