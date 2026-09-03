package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/credentials"
	"aerosight/worker/internal/outbox"
)

const (
	FlightHubLiveStartEventType = "flighthub.live.start.requested"
	FlightHubLiveSourceType     = "dji_flighthub"

	flightHubLiveNoViewerTimeout = 5 * time.Minute
	flightHubLiveEvidenceMaxAge  = 2 * time.Minute
	flightHubLiveStartGrace      = 30 * time.Second
)

type FlightHubLiveSession struct {
	ID                  int64
	ProjectID           int
	TeamID              int
	DeviceID            int
	ConnectorInstanceID int64
	CameraIndex         string
	DeviceSerial        string
	Status              string
	ConnectorStatus     string
	ActionEnabled       bool
	CapabilityVerified  bool
	StartAttemptedAt    sql.NullTime
	StartAcceptedAt     sql.NullTime
	CredentialExpiresAt sql.NullTime
	Instance            connector.Instance
}

type FlightHubLiveSessionStore interface {
	Load(context.Context, int, int64) (FlightHubLiveSession, error)
	BeginStart(context.Context, FlightHubLiveSession, time.Time) (bool, error)
	StoreAuthorization(context.Context, FlightHubLiveSession, NormalizedLivePlayback, credentials.Envelope, time.Time) error
	RecordStartUnconfirmed(context.Context, FlightHubLiveSession, string, time.Time) error
	Fail(context.Context, FlightHubLiveSession, string, bool, time.Time) error
}

type SQLFlightHubLiveSessionStore struct{ db *sql.DB }

func NewSQLFlightHubLiveSessionStore(database *sql.DB) *SQLFlightHubLiveSessionStore {
	return &SQLFlightHubLiveSessionStore{db: database}
}

func (store *SQLFlightHubLiveSessionStore) Load(ctx context.Context, projectID int, streamID int64) (FlightHubLiveSession, error) {
	if store == nil || store.db == nil || projectID <= 0 || streamID <= 0 {
		return FlightHubLiveSession{}, &APIError{SafeCode: "request_invalid"}
	}
	var session FlightHubLiveSession
	var credentialRaw, scopeRaw []byte
	err := store.db.QueryRowContext(ctx, `select stream.id,stream.project_id,stream.team_id,stream.device_id,
		stream.adapter_id,stream.stream_key,coalesce(identity.identity_json->'attributes'->>'serialNumber',''),
		stream.status,adapter.status,stream.start_attempted_at,stream.start_accepted_at,
		stream.supplier_credential_expires_at,definition.connector_key,definition.version,
		adapter.config_json,adapter.credential_envelope_json,adapter.discovery_scope_json,
		coalesce((flags.flighthub_action_flags_json->>'live.control')::boolean,false),
		exists(select 1 from connector_capability_snapshots capability
		 where capability.project_id=stream.project_id and capability.connector_instance_id=stream.adapter_id
		   and capability.capability_code='live.control' and capability.status='supported'
		   and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
		   and capability.region='cn' and capability.deployment='cn-public-cloud'
		   and capability.evidence_level='field-write'
		   and capability.device_model=device.device_model and capability.firmware_version is null
		   and (capability.expires_at is null or capability.expires_at>now()))
	 from live_streams stream
	 join devices device on device.id=stream.device_id and device.project_id=stream.project_id
	 join device_adapters adapter on adapter.id=stream.adapter_id and adapter.project_id=stream.project_id
	 join connector_definitions definition on definition.id=adapter.connector_definition_id
	 join device_external_identities identity on identity.project_id=stream.project_id
	   and identity.adapter_id=stream.adapter_id and identity.device_id=stream.device_id
	 left join project_feature_flags flags on flags.project_id=stream.project_id
	 where stream.project_id=$1 and stream.id=$2 and stream.source_type=$3`,
		projectID, streamID, FlightHubLiveSourceType).Scan(
		&session.ID, &session.ProjectID, &session.TeamID, &session.DeviceID,
		&session.ConnectorInstanceID, &session.CameraIndex, &session.DeviceSerial,
		&session.Status, &session.ConnectorStatus, &session.StartAttemptedAt, &session.StartAcceptedAt,
		&session.CredentialExpiresAt, &session.Instance.ConnectorKey, &session.Instance.Version,
		&session.Instance.Config, &credentialRaw, &scopeRaw, &session.ActionEnabled, &session.CapabilityVerified,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return FlightHubLiveSession{}, &APIError{SafeCode: "scope_forbidden"}
	}
	if err != nil {
		return FlightHubLiveSession{}, err
	}
	session.DeviceSerial = strings.TrimSpace(session.DeviceSerial)
	session.CameraIndex = strings.TrimSpace(session.CameraIndex)
	session.Instance.ID = session.ConnectorInstanceID
	session.Instance.ProjectID = session.ProjectID
	session.Instance.CredentialEnvelope = json.RawMessage(credentialRaw)
	session.Instance.DiscoveryScope = json.RawMessage(scopeRaw)
	return session, nil
}

func (store *SQLFlightHubLiveSessionStore) BeginStart(ctx context.Context, session FlightHubLiveSession, now time.Time) (bool, error) {
	result, err := store.db.ExecContext(ctx, `update live_streams set status='starting',
		status_reason='FLIGHTHUB_LIVE_START_DISPATCHED',start_attempted_at=$3,updated_at=now()
	 where id=$1 and project_id=$2 and source_type=$4 and status='requested' and start_attempted_at is null`,
		session.ID, session.ProjectID, now.UTC(), FlightHubLiveSourceType)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (store *SQLFlightHubLiveSessionStore) StoreAuthorization(ctx context.Context, session FlightHubLiveSession, playback NormalizedLivePlayback, envelope credentials.Envelope, now time.Time) error {
	envelopeRaw, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	description := playback.Description
	result, err := store.db.ExecContext(ctx, `update live_streams set
		supplier=$3,supplier_protocol=$4,supplier_adapter_version=$5,supplier_reference_digest=$6,
		supplier_credential_expires_at=$7,supplier_credential_envelope_json=$8::jsonb,
		start_accepted_at=$9,status_reason='FLIGHTHUB_LIVE_START_ACCEPTED',
		playback_ref='flighthub:'||$6,updated_at=now()
	 where id=$1 and project_id=$2 and source_type=$10 and status='starting'
	   and start_attempted_at is not null and start_accepted_at is null`,
		session.ID, session.ProjectID, description.Supplier, description.Protocol, description.AdapterVersion,
		description.ReferenceDigest, description.ExpiresAt.UTC(), string(envelopeRaw), now.UTC(), FlightHubLiveSourceType)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("FlightHub live authorization state changed concurrently")
	}
	return nil
}

func (store *SQLFlightHubLiveSessionStore) RecordStartUnconfirmed(ctx context.Context, session FlightHubLiveSession, code string, now time.Time) error {
	_, err := store.db.ExecContext(ctx, `update live_streams set status='starting',status_reason=$3,
		remote_evidence_at=null,updated_at=now()
	 where id=$1 and project_id=$2 and source_type=$4 and status='starting' and start_attempted_at is not null`,
		session.ID, session.ProjectID, code, FlightHubLiveSourceType)
	return err
}

func (store *SQLFlightHubLiveSessionStore) Fail(ctx context.Context, session FlightHubLiveSession, code string, remoteUnconfirmed bool, now time.Time) error {
	_, err := store.db.ExecContext(ctx, `update live_streams set status='failed',status_reason=$3,ended_at=$4,
		playback_ref=null,playback_locator_expires_at=null,supplier_credential_envelope_json=null,
		local_authorization_revoked_at=case when $5 then $4 else local_authorization_revoked_at end,
		lease_owner=null,lease_expires_at=null,updated_at=now()
	 where id=$1 and project_id=$2 and source_type=$6 and status in('requested','starting')`,
		session.ID, session.ProjectID, code, now.UTC(), remoteUnconfirmed, FlightHubLiveSourceType)
	return err
}

type FlightHubLiveStartClient interface {
	StartLiveStream(context.Context, string, string, LiveStreamStartRequest) (LiveStreamAuthorization, error)
}

type FlightHubLiveNormalizer interface {
	Normalize(LiveStreamAuthorization) (NormalizedLivePlayback, error)
}

type FlightHubLiveStartHandler struct {
	store      FlightHubLiveSessionStore
	client     FlightHubLiveStartClient
	normalizer FlightHubLiveNormalizer
	resolver   TokenResolver
	authSecret string
	now        func() time.Time
}

func NewFlightHubLiveStartHandler(store FlightHubLiveSessionStore, client FlightHubLiveStartClient, normalizer FlightHubLiveNormalizer, resolver TokenResolver, authSecret string, now func() time.Time) (*FlightHubLiveStartHandler, error) {
	if store == nil || client == nil || normalizer == nil || resolver == nil || strings.TrimSpace(authSecret) == "" {
		return nil, errors.New("FlightHub live start dependencies are required")
	}
	if now == nil {
		now = time.Now
	}
	return &FlightHubLiveStartHandler{store: store, client: client, normalizer: normalizer, resolver: resolver, authSecret: authSecret, now: now}, nil
}

func parseFlightHubLiveStartEvent(event outbox.Event) (int64, error) {
	var payload struct {
		StreamID int64 `json:"streamId"`
	}
	if event.ProjectID <= 0 || event.TeamID <= 0 || json.Unmarshal(event.Payload, &payload) != nil || payload.StreamID <= 0 {
		return 0, &APIError{SafeCode: "request_invalid"}
	}
	return payload.StreamID, nil
}

func knownLiveStartRejection(err error) bool {
	switch SafeCode(err) {
	case "request_invalid", "credential_invalid", "scope_forbidden", "capability_not_supported", "configuration_required":
		return true
	default:
		return false
	}
}

func (handler *FlightHubLiveStartHandler) Handler(ctx context.Context, _ *sql.Tx, event outbox.Event) error {
	streamID, err := parseFlightHubLiveStartEvent(event)
	if err != nil {
		return err
	}
	session, err := handler.store.Load(ctx, event.ProjectID, streamID)
	if err != nil {
		return err
	}
	if session.TeamID != event.TeamID || session.Instance.ConnectorKey != ConnectorKey || session.Instance.Version != ConnectorVersion || !isActiveConnectorStatus(session.ConnectorStatus) {
		return handler.store.Fail(ctx, session, "FLIGHTHUB_LIVE_CONNECTOR_UNAVAILABLE", false, handler.now().UTC())
	}
	if !session.ActionEnabled || !session.CapabilityVerified {
		return handler.store.Fail(ctx, session, "FLIGHTHUB_LIVE_ACTION_DISABLED", false, handler.now().UTC())
	}
	if session.Status == "failed" || session.Status == "stopped" || session.Status == "stopping" || session.StartAttemptedAt.Valid {
		return nil
	}
	if session.DeviceSerial == "" || session.CameraIndex == "" {
		return handler.store.Fail(ctx, session, "FLIGHTHUB_LIVE_SCOPE_INVALID", false, handler.now().UTC())
	}
	now := handler.now().UTC()
	begun, err := handler.store.BeginStart(ctx, session, now)
	if err != nil || !begun {
		return err
	}
	session.Status = "starting"
	session.StartAttemptedAt = sql.NullTime{Time: now, Valid: true}
	scope, err := parseScope(session.Instance.DiscoveryScope)
	if err != nil {
		return handler.store.Fail(ctx, session, "FLIGHTHUB_LIVE_SCOPE_INVALID", false, now)
	}
	token, err := handler.resolver.ResolveToken(ctx, session.Instance)
	if err != nil {
		return handler.store.Fail(ctx, session, "FLIGHTHUB_LIVE_CREDENTIAL_UNAVAILABLE", false, now)
	}
	defer func() { token = "" }()
	authorization, err := handler.client.StartLiveStream(ctx, token, scope.ProjectUUID, LiveStreamStartRequest{
		SN: session.DeviceSerial, CameraIndex: session.CameraIndex, VideoExpire: 3600, QualityType: LiveQualityAdaptive,
	})
	if err != nil {
		if knownLiveStartRejection(err) {
			return handler.store.Fail(ctx, session, "FLIGHTHUB_LIVE_START_REJECTED_"+strings.ToUpper(SafeCode(err)), false, now)
		}
		return handler.store.RecordStartUnconfirmed(ctx, session, "FLIGHTHUB_LIVE_START_RESPONSE_UNKNOWN", now)
	}
	playback, err := handler.normalizer.Normalize(authorization)
	if err != nil {
		return handler.store.Fail(ctx, session, "FLIGHTHUB_LIVE_SUPPLIER_UNSUPPORTED_REMOTE_UNCONFIRMED", true, now)
	}
	credential := playback.Secret.Reveal()
	envelope, err := credentials.EncryptJSON(map[string]string{"credential": credential}, handler.authSecret,
		credentials.AAD("flighthub-live-session", session.ID, session.ProjectID))
	credential = ""
	if err != nil {
		return handler.store.Fail(ctx, session, "FLIGHTHUB_LIVE_CREDENTIAL_ENCRYPTION_FAILED_REMOTE_UNCONFIRMED", true, now)
	}
	return handler.store.StoreAuthorization(ctx, session, playback, envelope, now)
}

type FlightHubLiveEvidence struct {
	Status              string
	StartedAt           time.Time
	StartAttemptedAt    sql.NullTime
	StartAcceptedAt     sql.NullTime
	LastPlaybackAt      sql.NullTime
	CredentialExpiresAt sql.NullTime
	DeviceStatus        string
	LiveAvailable       bool
	LiveActive          bool
	LiveCapturedAt      sql.NullTime
}

type FlightHubLiveDecision struct {
	Status     string
	Reason     string
	EvidenceAt sql.NullTime
	Terminal   bool
}

func decideFlightHubLiveSession(now time.Time, evidence FlightHubLiveEvidence) FlightHubLiveDecision {
	decision := FlightHubLiveDecision{Status: evidence.Status, Reason: "FLIGHTHUB_LIVE_EVIDENCE_UNAVAILABLE"}
	freshLive := evidence.LiveAvailable && evidence.LiveCapturedAt.Valid &&
		!evidence.LiveCapturedAt.Time.Before(evidence.StartedAt) &&
		!evidence.LiveCapturedAt.Time.Before(now.Add(-flightHubLiveEvidenceMaxAge)) &&
		!evidence.LiveCapturedAt.Time.After(now.Add(time.Minute))
	if freshLive {
		decision.EvidenceAt = evidence.LiveCapturedAt
		if evidence.LiveActive {
			switch evidence.Status {
			case "starting":
				if evidence.StartAcceptedAt.Valid {
					decision.Status, decision.Reason = "live", ""
				} else {
					decision.Status, decision.Reason, decision.Terminal = "failed", "FLIGHTHUB_LIVE_REMOTE_ACTIVE_CREDENTIAL_UNAVAILABLE", true
				}
			case "live", "degraded":
				decision.Status, decision.Reason = "live", ""
			case "stopping":
				decision.Reason = "FLIGHTHUB_LIVE_STOP_REMOTE_ACTIVE"
			}
			return decision
		}
		switch evidence.Status {
		case "live", "degraded", "stopping":
			decision.Status, decision.Reason, decision.Terminal = "stopped", "FLIGHTHUB_LIVE_REMOTE_INACTIVE", true
		case "starting":
			anchor := evidence.StartAttemptedAt
			if evidence.StartAcceptedAt.Valid {
				anchor = evidence.StartAcceptedAt
			}
			if anchor.Valid && !now.Before(anchor.Time.Add(flightHubLiveStartGrace)) {
				decision.Status, decision.Reason, decision.Terminal = "failed", "FLIGHTHUB_LIVE_START_NOT_CONFIRMED", true
			} else {
				decision.Reason = "FLIGHTHUB_LIVE_START_AWAITING_CONFIRMATION"
			}
		}
		return decision
	}
	if evidence.DeviceStatus == "offline" {
		decision.EvidenceAt = sql.NullTime{Time: now, Valid: true}
		switch evidence.Status {
		case "live", "degraded", "stopping":
			decision.Status, decision.Reason, decision.Terminal = "stopped", "FLIGHTHUB_LIVE_DEVICE_OFFLINE", true
		case "requested", "starting":
			decision.Status, decision.Reason, decision.Terminal = "failed", "FLIGHTHUB_LIVE_DEVICE_OFFLINE", true
		}
		return decision
	}
	credentialExpired := evidence.CredentialExpiresAt.Valid && !now.Before(evidence.CredentialExpiresAt.Time)
	lastViewer := evidence.LastPlaybackAt
	if !lastViewer.Valid {
		lastViewer = evidence.StartAcceptedAt
	}
	noViewerExpired := lastViewer.Valid && !now.Before(lastViewer.Time.Add(flightHubLiveNoViewerTimeout))
	if credentialExpired || noViewerExpired {
		decision.EvidenceAt = sql.NullTime{Time: now, Valid: true}
		reason := "FLIGHTHUB_LIVE_NO_VIEWER_AUTO_STOP"
		if credentialExpired {
			reason = "FLIGHTHUB_LIVE_CREDENTIAL_EXPIRED"
		}
		switch evidence.Status {
		case "live", "degraded", "stopping":
			decision.Status, decision.Reason, decision.Terminal = "stopped", reason, true
		case "starting":
			decision.Status, decision.Reason, decision.Terminal = "failed", reason, true
		}
	}
	return decision
}

type FlightHubLiveReconcileSummary struct {
	Candidates int
	Updated    int
}

type FlightHubLiveReconciler struct {
	db  *sql.DB
	now func() time.Time
}

func NewFlightHubLiveReconciler(database *sql.DB, now func() time.Time) (*FlightHubLiveReconciler, error) {
	if database == nil {
		return nil, errors.New("FlightHub live reconciler database is required")
	}
	if now == nil {
		now = time.Now
	}
	return &FlightHubLiveReconciler{db: database, now: now}, nil
}

func (reconciler *FlightHubLiveReconciler) ReconcileLiveSessions(ctx context.Context, instance connector.Instance) (FlightHubLiveReconcileSummary, error) {
	if reconciler == nil || reconciler.db == nil || instance.ID <= 0 || instance.ProjectID <= 0 {
		return FlightHubLiveReconcileSummary{}, errors.New("FlightHub live reconcile scope is invalid")
	}
	now := reconciler.now().UTC()
	rows, err := reconciler.db.QueryContext(ctx, `select stream.id,stream.team_id,stream.device_id,stream.status,
		coalesce(stream.status_reason,''),stream.started_at,
		stream.start_attempted_at,stream.start_accepted_at,stream.last_playback_at,
		stream.supplier_credential_expires_at,device.status,
		latest.captured_at,
		coalesce((latest.payload_json->'live'->>'available')::boolean,false),
		coalesce((latest.payload_json->'live'->>'active')::boolean,false)
	 from live_streams stream
	 join devices device on device.id=stream.device_id and device.project_id=stream.project_id
	 left join device_latest_telemetry latest on latest.device_id=stream.device_id
	   and latest.project_id=stream.project_id and latest.adapter_id=stream.adapter_id
	   and latest.telemetry_type='dji.flighthub.state'
	 where stream.project_id=$1 and stream.adapter_id=$2 and stream.source_type=$3
	   and stream.status in('requested','starting','live','degraded','stopping')
	 order by stream.started_at limit 100`, instance.ProjectID, instance.ID, FlightHubLiveSourceType)
	if err != nil {
		return FlightHubLiveReconcileSummary{}, err
	}
	type candidate struct {
		id            int64
		teamID        int
		deviceID      int
		currentReason string
		evidence      FlightHubLiveEvidence
	}
	candidates := make([]candidate, 0)
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.teamID, &item.deviceID, &item.evidence.Status, &item.currentReason,
			&item.evidence.StartedAt,
			&item.evidence.StartAttemptedAt, &item.evidence.StartAcceptedAt, &item.evidence.LastPlaybackAt,
			&item.evidence.CredentialExpiresAt, &item.evidence.DeviceStatus,
			&item.evidence.LiveCapturedAt, &item.evidence.LiveAvailable, &item.evidence.LiveActive); err != nil {
			rows.Close()
			return FlightHubLiveReconcileSummary{}, err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Close(); err != nil {
		return FlightHubLiveReconcileSummary{}, err
	}
	summary := FlightHubLiveReconcileSummary{Candidates: len(candidates)}
	for _, item := range candidates {
		decision := decideFlightHubLiveSession(now, item.evidence)
		tx, err := reconciler.db.BeginTx(ctx, nil)
		if err != nil {
			return summary, err
		}
		result, err := tx.ExecContext(ctx, `update live_streams set status=$4,status_reason=nullif($5,''),
			remote_evidence_at=case when $6::boolean then $7 else remote_evidence_at end,
			last_active_at=case when $4='live' then $3 else last_active_at end,
			ended_at=case when $8 then $3 else ended_at end,
			playback_ref=case when $8 then null else playback_ref end,
			playback_locator_expires_at=case when $8 then null else playback_locator_expires_at end,
			supplier_credential_envelope_json=case when $8 then null else supplier_credential_envelope_json end,
			local_authorization_revoked_at=case when $8 then coalesce(local_authorization_revoked_at,$3) else local_authorization_revoked_at end,
			lease_owner=case when $8 then null else lease_owner end,
			lease_expires_at=case when $8 then null else lease_expires_at end,updated_at=now()
			 where id=$1 and project_id=$2 and source_type=$9 and status=$10
			   and status_reason is not distinct from nullif($11,'')
			   and (status<>$4 or status_reason is distinct from nullif($5,'')
			        or ($6::boolean and remote_evidence_at is distinct from $7))`,
			item.id, instance.ProjectID, now, decision.Status, decision.Reason,
			decision.EvidenceAt.Valid, nullableTime(decision.EvidenceAt), decision.Terminal,
			FlightHubLiveSourceType, item.evidence.Status, item.currentReason)
		if err != nil {
			_ = tx.Rollback()
			return summary, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			_ = tx.Rollback()
			return summary, err
		}
		if count != 1 {
			_ = tx.Rollback()
			continue
		}
		if decision.Status != item.evidence.Status || decision.Reason != item.currentReason {
			payload, marshalErr := json.Marshal(map[string]any{
				"streamId": item.id, "deviceId": item.deviceID,
				"status": decision.Status, "reason": decision.Reason,
			})
			if marshalErr != nil {
				_ = tx.Rollback()
				return summary, marshalErr
			}
			eventID := fmt.Sprintf("flighthub-live-reconcile:%d:%s:%s", item.id, decision.Status, decision.Reason)
			if _, err := tx.ExecContext(ctx, `insert into project_events(project_id,team_id,event_id,event_type,payload_json,occurred_at)
				values($1,$2,$3,'live_stream.status_changed',$4,$5) on conflict(event_id) do nothing`,
				instance.ProjectID, item.teamID, eventID, payload, now); err != nil {
				_ = tx.Rollback()
				return summary, err
			}
		}
		if err := tx.Commit(); err != nil {
			return summary, err
		}
		summary.Updated++
	}
	return summary, nil
}

func nullableTime(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time.UTC()
}

func (summary FlightHubLiveReconcileSummary) Cursor() map[string]any {
	return map[string]any{"liveCandidates": summary.Candidates, "liveUpdated": summary.Updated}
}

func (session FlightHubLiveSession) String() string {
	return fmt.Sprintf("FlightHubLiveSession{id:%d,project:%d,status:%s}", session.ID, session.ProjectID, session.Status)
}
