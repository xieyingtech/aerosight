package flighthub

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/credentials"
	"aerosight/worker/internal/outbox"
)

const (
	WaylineUploadEventType  = "flighthub.wayline_upload.requested"
	maxNotificationAttempts = 2
	missesBeforeRetry       = 2
)

type WaylineUploadRequest struct {
	ProjectID           int
	ConnectorInstanceID int64
	SourceAssetID       int
	RequestedByUserID   int
	IdempotencyKey      string
	Name                string
}

type WaylineUploadJob struct {
	ID                       string
	ProjectID                int
	TeamID                   int
	ConnectorInstanceID      int64
	OperationKind            string
	SourceAssetID            int
	RequestedByUserID        int
	IdempotencyKey           string
	RequestedName            string
	ReconciliationName       string
	Status                   string
	ObjectKeyEnvelope        json.RawMessage
	NotificationAttemptCount int
	ReconciliationMissCount  int
	LastErrorCode            string
	SourceStorageKey         string
	SourceContentType        string
	SourceStatus             string
	ConnectorStatus          string
	ActionEnabled            bool
	Instance                 connector.Instance
}

type WaylineUploadStore interface {
	Create(context.Context, WaylineUploadRequest) (WaylineUploadJob, error)
	Load(context.Context, int, string) (WaylineUploadJob, error)
	MarkUploading(context.Context, WaylineUploadJob) error
	MarkUploaded(context.Context, WaylineUploadJob, json.RawMessage, string) error
	BeginNotification(context.Context, WaylineUploadJob) (int, error)
	RecordError(context.Context, WaylineUploadJob, string) error
	RecordReconciliationMiss(context.Context, WaylineUploadJob) (int, error)
	Complete(context.Context, WaylineUploadJob, WaylineUploadResult) error
	Fail(context.Context, WaylineUploadJob, string) error
}

type SQLWaylineUploadStore struct{ db *sql.DB }

func NewSQLWaylineUploadStore(database *sql.DB) *SQLWaylineUploadStore {
	return &SQLWaylineUploadStore{db: database}
}

func normalizeWaylineUploadRequest(input WaylineUploadRequest) (WaylineUploadRequest, error) {
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.Name = strings.TrimSpace(input.Name)
	if input.ProjectID <= 0 || input.ConnectorInstanceID <= 0 || input.SourceAssetID <= 0 || input.RequestedByUserID <= 0 ||
		len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 200 || len(input.Name) < 1 || len(input.Name) > 200 ||
		strings.ContainsAny(input.IdempotencyKey+input.Name, "\x00\r\n") {
		return input, &APIError{SafeCode: "request_invalid"}
	}
	return input, nil
}

func reconciliationWaylineName(input WaylineUploadRequest) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s", input.ProjectID, input.ConnectorInstanceID, input.IdempotencyKey)))
	name := input.Name
	if len(name) > 210 {
		name = name[:210]
	}
	return fmt.Sprintf("%s · AeroSight-%s", name, hex.EncodeToString(digest[:6]))
}

func (store *SQLWaylineUploadStore) Create(ctx context.Context, request WaylineUploadRequest) (job WaylineUploadJob, returnedErr error) {
	if store == nil || store.db == nil {
		return job, errors.New("FlightHub wayline upload store is unavailable")
	}
	request, err := normalizeWaylineUploadRequest(request)
	if err != nil {
		return job, err
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return job, err
	}
	defer func() {
		if returnedErr != nil {
			_ = tx.Rollback()
		}
	}()
	var teamID int
	err = tx.QueryRowContext(ctx, `select adapter.team_id
		from device_adapters adapter
		join connector_definitions definition on definition.id=adapter.connector_definition_id
		join assets asset on asset.id=$3 and asset.project_id=adapter.project_id and asset.status='available'
		join team_members member on member.team_id=adapter.team_id and member.user_id=$4
		join project_feature_flags flags on flags.project_id=adapter.project_id
		where adapter.id=$1 and adapter.project_id=$2 and definition.connector_key=$5 and definition.version=$6
		  and adapter.status in('connecting','connected','degraded')
		  and flags.flighthub_action_flags_json @> '{"security.temporary-credential":true,"flight.execute":true}'::jsonb`,
		request.ConnectorInstanceID, request.ProjectID, request.SourceAssetID, request.RequestedByUserID,
		ConnectorKey, ConnectorVersion).Scan(&teamID)
	if errors.Is(err, sql.ErrNoRows) {
		return job, &APIError{SafeCode: "scope_forbidden"}
	}
	if err != nil {
		return job, err
	}
	name := reconciliationWaylineName(request)
	err = tx.QueryRowContext(ctx, `insert into connector_object_upload_jobs(
		project_id,team_id,connector_instance_id,operation_kind,source_asset_id,requested_by_user_id,
		idempotency_key,requested_name,reconciliation_name
	) values($1,$2,$3,'wayline',$4,$5,$6,$7,$8)
	 on conflict(project_id,connector_instance_id,operation_kind,idempotency_key) do nothing returning id::text`,
		request.ProjectID, teamID, request.ConnectorInstanceID, request.SourceAssetID, request.RequestedByUserID,
		request.IdempotencyKey, request.Name, name).Scan(&job.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `select id::text,source_asset_id,requested_by_user_id,requested_name
			from connector_object_upload_jobs where project_id=$1 and connector_instance_id=$2 and operation_kind='wayline' and idempotency_key=$3`,
			request.ProjectID, request.ConnectorInstanceID, request.IdempotencyKey).
			Scan(&job.ID, &job.SourceAssetID, &job.RequestedByUserID, &job.RequestedName)
		if err != nil {
			return job, err
		}
		if job.SourceAssetID != request.SourceAssetID || job.RequestedByUserID != request.RequestedByUserID || job.RequestedName != request.Name {
			return job, &APIError{SafeCode: "idempotency_conflict"}
		}
	} else if err != nil {
		return job, err
	}
	payload, _ := json.Marshal(map[string]string{"jobId": job.ID})
	if _, err = tx.ExecContext(ctx, `insert into outbox_events(
		project_id,team_id,event_id,event_type,aggregate_type,aggregate_id,payload_json,max_attempts
	) values($1,$2,$3,$4,'flighthub_wayline_upload',$5,$6,16)
	 on conflict(event_id) do nothing`, request.ProjectID, teamID, "flighthub-wayline-upload:"+job.ID,
		WaylineUploadEventType, job.ID, payload); err != nil {
		return job, err
	}
	if err = tx.Commit(); err != nil {
		return job, err
	}
	return store.Load(ctx, request.ProjectID, job.ID)
}

func (store *SQLWaylineUploadStore) Load(ctx context.Context, projectID int, jobID string) (job WaylineUploadJob, err error) {
	if store == nil || store.db == nil || projectID <= 0 || strings.TrimSpace(jobID) == "" {
		return job, &APIError{SafeCode: "request_invalid"}
	}
	var envelope []byte
	err = store.db.QueryRowContext(ctx, `select job.id::text,job.project_id,job.team_id,job.connector_instance_id,job.operation_kind,
		job.source_asset_id,job.requested_by_user_id,job.idempotency_key,job.requested_name,
		job.reconciliation_name,job.status,coalesce(job.object_key_envelope_json,'null'::jsonb),
		job.notification_attempt_count,job.reconciliation_miss_count,coalesce(job.last_error_code,''),
		asset.storage_key,coalesce(asset.mime_type,'application/vnd.google-earth.kmz'),asset.status,adapter.status,
		coalesce(flags.flighthub_action_flags_json @> '{"security.temporary-credential":true,"flight.execute":true}'::jsonb,false),
		definition.connector_key,definition.version,adapter.credential_envelope_json,adapter.discovery_scope_json
	 from connector_object_upload_jobs job
	 join assets asset on asset.id=job.source_asset_id and asset.project_id=job.project_id
	 join device_adapters adapter on adapter.id=job.connector_instance_id and adapter.project_id=job.project_id
	 join connector_definitions definition on definition.id=adapter.connector_definition_id
	 left join project_feature_flags flags on flags.project_id=job.project_id
	 where job.id=$1::uuid and job.project_id=$2`, jobID, projectID).Scan(
		&job.ID, &job.ProjectID, &job.TeamID, &job.ConnectorInstanceID, &job.OperationKind, &job.SourceAssetID,
		&job.RequestedByUserID, &job.IdempotencyKey, &job.RequestedName, &job.ReconciliationName,
		&job.Status, &envelope, &job.NotificationAttemptCount, &job.ReconciliationMissCount,
		&job.LastErrorCode, &job.SourceStorageKey, &job.SourceContentType, &job.SourceStatus, &job.ConnectorStatus, &job.ActionEnabled,
		&job.Instance.ConnectorKey, &job.Instance.Version, &job.Instance.CredentialEnvelope, &job.Instance.DiscoveryScope,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return job, &APIError{SafeCode: "scope_forbidden"}
	}
	job.ObjectKeyEnvelope = json.RawMessage(envelope)
	job.Instance.ID, job.Instance.ProjectID = job.ConnectorInstanceID, job.ProjectID
	return job, err
}

func (store *SQLWaylineUploadStore) updateStatus(ctx context.Context, job WaylineUploadJob, query string, args ...any) error {
	result, err := store.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("FlightHub wayline upload state changed concurrently")
	}
	return nil
}

func (store *SQLWaylineUploadStore) MarkUploading(ctx context.Context, job WaylineUploadJob) error {
	return store.updateStatus(ctx, job, `update connector_object_upload_jobs set status='uploading',updated_at=now()
		where id=$1::uuid and project_id=$2 and status in('queued','uploading')`, job.ID, job.ProjectID)
}

func (store *SQLWaylineUploadStore) MarkUploaded(ctx context.Context, job WaylineUploadJob, envelope json.RawMessage, digest string) error {
	return store.updateStatus(ctx, job, `update connector_object_upload_jobs
		set status='notifying',object_key_envelope_json=$3,object_key_digest=$4,uploaded_at=now(),last_error_code=null,updated_at=now()
		where id=$1::uuid and project_id=$2 and status='uploading'`, job.ID, job.ProjectID, envelope, digest)
}

func (store *SQLWaylineUploadStore) BeginNotification(ctx context.Context, job WaylineUploadJob) (int, error) {
	var attempts int
	err := store.db.QueryRowContext(ctx, `update connector_object_upload_jobs
		set status='reconciling',notification_attempt_count=notification_attempt_count+1,
		    notification_attempted_at=now(),last_error_code=null,updated_at=now()
		where id=$1::uuid and project_id=$2 and status in('notifying','reconciling')
		  and notification_attempt_count<$3 returning notification_attempt_count`,
		job.ID, job.ProjectID, maxNotificationAttempts).Scan(&attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, &APIError{SafeCode: "notification_retry_exhausted"}
	}
	return attempts, err
}

func (store *SQLWaylineUploadStore) RecordError(ctx context.Context, job WaylineUploadJob, code string) error {
	return store.updateStatus(ctx, job, `update connector_object_upload_jobs set last_error_code=$3,updated_at=now()
		where id=$1::uuid and project_id=$2 and status not in('succeeded','failed')`, job.ID, job.ProjectID, code)
}

func (store *SQLWaylineUploadStore) RecordReconciliationMiss(ctx context.Context, job WaylineUploadJob) (int, error) {
	var misses int
	err := store.db.QueryRowContext(ctx, `update connector_object_upload_jobs
		set reconciliation_miss_count=reconciliation_miss_count+1,reconciled_at=now(),updated_at=now()
		where id=$1::uuid and project_id=$2 and status='reconciling' and reconciliation_miss_count<8
		returning reconciliation_miss_count`, job.ID, job.ProjectID).Scan(&misses)
	return misses, err
}

func (store *SQLWaylineUploadStore) Complete(ctx context.Context, job WaylineUploadJob, result WaylineUploadResult) (returnedErr error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if returnedErr != nil {
			_ = tx.Rollback()
		}
	}()
	summary, _ := json.Marshal(map[string]any{"name": strings.TrimSpace(result.Name), "source": "wayline-upload"})
	var resourceID int64
	err = tx.QueryRowContext(ctx, `insert into connector_remote_resources(
		project_id,team_id,connector_instance_id,resource_kind,remote_id,status,summary_json
	) values($1,$2,$3,'wayline',$4,'active',$5)
	 on conflict(project_id,connector_instance_id,resource_kind,remote_id) do update set
		status='active',summary_json=excluded.summary_json,last_seen_at=now(),missing_at=null,updated_at=now()
	 returning id`, job.ProjectID, job.TeamID, job.ConnectorInstanceID, strings.TrimSpace(result.UUID), summary).Scan(&resourceID)
	if err != nil {
		return err
	}
	resultSQL, err := tx.ExecContext(ctx, `update connector_object_upload_jobs
		set status='succeeded',remote_resource_id=$3,last_error_code=null,reconciled_at=now(),completed_at=now(),updated_at=now()
		where id=$1::uuid and project_id=$2 and status in('notifying','reconciling')`, job.ID, job.ProjectID, resourceID)
	if err != nil {
		return err
	}
	if count, _ := resultSQL.RowsAffected(); count != 1 {
		return errors.New("FlightHub wayline upload completion state changed")
	}
	return tx.Commit()
}

func (store *SQLWaylineUploadStore) Fail(ctx context.Context, job WaylineUploadJob, code string) error {
	return store.updateStatus(ctx, job, `update connector_object_upload_jobs
		set status='failed',last_error_code=$3,updated_at=now()
		where id=$1::uuid and project_id=$2 and status not in('succeeded','failed')`, job.ID, job.ProjectID, code)
}

type WaylineUploadClient interface {
	CreateStorageSTS(context.Context, string, string, StorageSTSRequest) (StorageSTS, error)
	NotifyWaylineUploadComplete(context.Context, string, string, WaylineUploadCompleteRequest) (WaylineUploadResult, error)
	ListWaylines(context.Context, string, string) ([]WaylineSummary, error)
}

type WaylineSourceObject struct {
	Body        []byte
	ContentType string
}

type WaylineSourceReader interface {
	ReadWaylineSource(context.Context, string) (WaylineSourceObject, error)
}

type WaylineObjectUploader interface {
	Upload(context.Context, StorageSTS, string, io.Reader, int64, string) error
}

type WaylineUploadHandler struct {
	store      WaylineUploadStore
	client     WaylineUploadClient
	resolver   TokenResolver
	source     WaylineSourceReader
	uploader   WaylineObjectUploader
	authSecret string
}

func NewWaylineUploadHandler(store WaylineUploadStore, client WaylineUploadClient, resolver TokenResolver, source WaylineSourceReader, uploader WaylineObjectUploader, authSecret string) (*WaylineUploadHandler, error) {
	if store == nil || client == nil || resolver == nil || source == nil || uploader == nil || strings.TrimSpace(authSecret) == "" {
		return nil, errors.New("FlightHub wayline upload dependencies are required")
	}
	return &WaylineUploadHandler{store: store, client: client, resolver: resolver, source: source, uploader: uploader, authSecret: authSecret}, nil
}

func parseWaylineUploadEvent(event outbox.Event) (string, error) {
	var payload struct {
		JobID string `json:"jobId"`
	}
	if event.ProjectID <= 0 || json.Unmarshal(event.Payload, &payload) != nil || strings.TrimSpace(payload.JobID) == "" {
		return "", &APIError{SafeCode: "request_invalid"}
	}
	return payload.JobID, nil
}

func safeWorkflowCode(err error) string {
	code := SafeCode(err)
	if code == "" {
		return "wayline_upload_failed"
	}
	return code
}

func retryableWorkflowError(code string) error {
	return &APIError{SafeCode: code, Retryable: true}
}

func terminalNotificationError(code string) bool {
	switch code {
	case "credential_invalid", "scope_forbidden", "scope_not_found", "request_invalid", "configuration_required":
		return true
	default:
		return false
	}
}

func waylineObjectKey(prefix string) (string, error) {
	prefix = strings.TrimSpace(strings.TrimSuffix(prefix, "/"))
	if prefix == "" || len(prefix) > 1000 || strings.HasPrefix(prefix, "/") || strings.ContainsAny(prefix, "\x00\r\n\\") {
		return "", &APIError{SafeCode: "temporary_link_invalid"}
	}
	for _, segment := range strings.Split(prefix, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", &APIError{SafeCode: "temporary_link_invalid"}
		}
	}
	key := prefix + "/wayline.kmz"
	if !strings.HasPrefix(key, prefix+"/") {
		return "", &APIError{SafeCode: "temporary_link_invalid"}
	}
	return key, nil
}

func (handler *WaylineUploadHandler) Handler(ctx context.Context, _ *sql.Tx, event outbox.Event) error {
	jobID, err := parseWaylineUploadEvent(event)
	if err != nil {
		return err
	}
	job, err := handler.store.Load(ctx, event.ProjectID, jobID)
	if err != nil {
		return err
	}
	if job.Status == "succeeded" || job.Status == "failed" {
		return nil
	}
	if job.ProjectID != event.ProjectID || job.TeamID != event.TeamID || job.OperationKind != "wayline" || job.Instance.ConnectorKey != ConnectorKey ||
		job.Instance.Version != ConnectorVersion || !isActiveConnectorStatus(job.ConnectorStatus) {
		return handler.store.Fail(ctx, job, "connector_disabled")
	}
	if job.SourceStatus != "available" {
		return handler.store.Fail(ctx, job, "source_asset_unavailable")
	}
	if !job.ActionEnabled {
		return handler.store.Fail(ctx, job, "action_disabled")
	}
	scope, err := parseScope(job.Instance.DiscoveryScope)
	if err != nil {
		return handler.store.Fail(ctx, job, "scope_forbidden")
	}
	token, err := handler.resolver.ResolveToken(ctx, job.Instance)
	if err != nil {
		code := safeWorkflowCode(err)
		if persistErr := handler.store.RecordError(ctx, job, code); persistErr != nil {
			return persistErr
		}
		return retryableWorkflowError(code)
	}
	defer func() { token = "" }()

	if job.Status == "queued" || job.Status == "uploading" {
		if err := handler.store.MarkUploading(ctx, job); err != nil {
			return err
		}
		source, err := handler.source.ReadWaylineSource(ctx, job.SourceStorageKey)
		if err != nil || len(source.Body) == 0 {
			if persistErr := handler.store.RecordError(ctx, job, "source_asset_unavailable"); persistErr != nil {
				return persistErr
			}
			return retryableWorkflowError("source_asset_unavailable")
		}
		sts, err := handler.client.CreateStorageSTS(ctx, token, scope.ProjectUUID, StorageSTSRequest{
			SpecifyPath: "waylines/" + job.ID + "/wayline.kmz", FileUUID: job.ID,
		})
		if err != nil {
			code := safeWorkflowCode(err)
			if persistErr := handler.store.RecordError(ctx, job, code); persistErr != nil {
				return persistErr
			}
			return retryableWorkflowError(code)
		}
		objectKey, err := waylineObjectKey(sts.ObjectKeyPrefix)
		if err != nil {
			return handler.store.Fail(ctx, job, safeWorkflowCode(err))
		}
		contentType := strings.TrimSpace(source.ContentType)
		if contentType == "" {
			contentType = "application/vnd.google-earth.kmz"
		}
		if err := handler.uploader.Upload(ctx, sts, objectKey, bytes.NewReader(source.Body), int64(len(source.Body)), contentType); err != nil {
			if persistErr := handler.store.RecordError(ctx, job, "object_upload_failed"); persistErr != nil {
				return persistErr
			}
			return retryableWorkflowError("object_upload_failed")
		}
		envelope, err := credentials.EncryptJSON(map[string]string{"objectKey": objectKey}, handler.authSecret,
			credentials.AAD("flighthub-wayline-upload", job.ID, job.ProjectID))
		if err != nil {
			return errors.New("FlightHub wayline object reference encryption failed")
		}
		envelopeJSON, err := json.Marshal(envelope)
		if err != nil {
			return err
		}
		digest := sha256.Sum256([]byte(objectKey))
		if err := handler.store.MarkUploaded(ctx, job, envelopeJSON, hex.EncodeToString(digest[:])); err != nil {
			return err
		}
		job.Status = "notifying"
		job.ObjectKeyEnvelope = envelopeJSON
		job.LastErrorCode = ""
	}
	if job.Status == "reconciling" {
		return handler.reconcile(ctx, job, token, scope.ProjectUUID)
	}
	return handler.notify(ctx, job, token, scope.ProjectUUID)
}

func isActiveConnectorStatus(status string) bool {
	return status == "connecting" || status == "connected" || status == "degraded"
}

func (handler *WaylineUploadHandler) decryptObjectKey(job WaylineUploadJob) (string, error) {
	var envelope credentials.Envelope
	if len(job.ObjectKeyEnvelope) == 0 || string(job.ObjectKeyEnvelope) == "null" || json.Unmarshal(job.ObjectKeyEnvelope, &envelope) != nil {
		return "", errors.New("FlightHub wayline object reference is unavailable")
	}
	var payload struct {
		ObjectKey string `json:"objectKey"`
	}
	if err := credentials.DecryptJSON(envelope, handler.authSecret,
		credentials.AAD("flighthub-wayline-upload", job.ID, job.ProjectID), &payload); err != nil || strings.TrimSpace(payload.ObjectKey) == "" {
		return "", errors.New("FlightHub wayline object reference is unavailable")
	}
	return payload.ObjectKey, nil
}

func (handler *WaylineUploadHandler) notify(ctx context.Context, job WaylineUploadJob, token, projectUUID string) error {
	objectKey, err := handler.decryptObjectKey(job)
	if err != nil {
		return handler.store.Fail(ctx, job, "object_reference_unavailable")
	}
	attempts, err := handler.store.BeginNotification(ctx, job)
	if err != nil {
		if IsSafeCode(err, "notification_retry_exhausted") {
			return handler.store.Fail(ctx, job, "notification_retry_exhausted")
		}
		return err
	}
	job.NotificationAttemptCount = attempts
	job.Status = "reconciling"
	result, err := handler.client.NotifyWaylineUploadComplete(ctx, token, projectUUID, WaylineUploadCompleteRequest{
		Name: job.ReconciliationName, ObjectKey: objectKey,
	})
	if err != nil {
		code := safeWorkflowCode(err)
		if persistErr := handler.store.RecordError(ctx, job, code); persistErr != nil {
			return persistErr
		}
		return retryableWorkflowError(code)
	}
	return handler.store.Complete(ctx, job, result)
}

func (handler *WaylineUploadHandler) reconcile(ctx context.Context, job WaylineUploadJob, token, projectUUID string) error {
	items, err := handler.client.ListWaylines(ctx, token, projectUUID)
	if err != nil {
		return retryableWorkflowError(safeWorkflowCode(err))
	}
	matches := make([]WaylineSummary, 0, 1)
	for _, item := range items {
		if strings.TrimSpace(item.Name) == job.ReconciliationName {
			matches = append(matches, item)
		}
	}
	if len(matches) > 1 {
		return handler.store.Fail(ctx, job, "reconciliation_ambiguous")
	}
	if len(matches) == 1 {
		return handler.store.Complete(ctx, job, WaylineUploadResult{Name: matches[0].Name, UUID: matches[0].ID})
	}
	misses, err := handler.store.RecordReconciliationMiss(ctx, job)
	if err != nil {
		return err
	}
	if terminalNotificationError(job.LastErrorCode) {
		return handler.store.Fail(ctx, job, job.LastErrorCode)
	}
	if job.NotificationAttemptCount >= maxNotificationAttempts {
		return handler.store.Fail(ctx, job, "notification_result_unknown")
	}
	if misses < missesBeforeRetry {
		return retryableWorkflowError("reconciliation_pending")
	}
	job.ReconciliationMissCount = misses
	return handler.notify(ctx, job, token, projectUUID)
}
