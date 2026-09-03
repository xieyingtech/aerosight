package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/credentials"
	"aerosight/worker/internal/outbox"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const flightHubLiveTestSecret = "0123456789abcdef0123456789abcdef"

type memoryFlightHubLiveStore struct {
	session           FlightHubLiveSession
	playback          NormalizedLivePlayback
	envelope          credentials.Envelope
	reason            string
	remoteUnconfirmed bool
}

func (store *memoryFlightHubLiveStore) Load(_ context.Context, projectID int, streamID int64) (FlightHubLiveSession, error) {
	if projectID != store.session.ProjectID || streamID != store.session.ID {
		return FlightHubLiveSession{}, &APIError{SafeCode: "scope_forbidden"}
	}
	return store.session, nil
}

func (store *memoryFlightHubLiveStore) BeginStart(_ context.Context, _ FlightHubLiveSession, now time.Time) (bool, error) {
	if store.session.Status != "requested" || store.session.StartAttemptedAt.Valid {
		return false, nil
	}
	store.session.Status = "starting"
	store.session.StartAttemptedAt = sql.NullTime{Time: now, Valid: true}
	return true, nil
}

func (store *memoryFlightHubLiveStore) StoreAuthorization(_ context.Context, _ FlightHubLiveSession, playback NormalizedLivePlayback, envelope credentials.Envelope, now time.Time) error {
	store.playback, store.envelope = playback, envelope
	store.session.StartAcceptedAt = sql.NullTime{Time: now, Valid: true}
	store.session.CredentialExpiresAt = sql.NullTime{Time: playback.Description.ExpiresAt, Valid: true}
	return nil
}

func (store *memoryFlightHubLiveStore) RecordStartUnconfirmed(_ context.Context, _ FlightHubLiveSession, code string, _ time.Time) error {
	store.reason = code
	return nil
}

func (store *memoryFlightHubLiveStore) Fail(_ context.Context, _ FlightHubLiveSession, code string, remoteUnconfirmed bool, _ time.Time) error {
	store.session.Status, store.reason, store.remoteUnconfirmed = "failed", code, remoteUnconfirmed
	return nil
}

type flightHubLiveClientFixture struct {
	calls int
	value LiveStreamAuthorization
	err   error
}

func (client *flightHubLiveClientFixture) StartLiveStream(context.Context, string, string, LiveStreamStartRequest) (LiveStreamAuthorization, error) {
	client.calls++
	return client.value, client.err
}

type flightHubLiveNormalizerFixture struct {
	value NormalizedLivePlayback
	err   error
}

func (normalizer flightHubLiveNormalizerFixture) Normalize(LiveStreamAuthorization) (NormalizedLivePlayback, error) {
	return normalizer.value, normalizer.err
}

type flightHubLiveResolverFixture struct{}

func (flightHubLiveResolverFixture) ResolveToken(context.Context, connector.Instance) (string, error) {
	return "TOKEN_REDACTED", nil
}

func flightHubLiveSessionFixture(now time.Time) FlightHubLiveSession {
	return FlightHubLiveSession{
		ID: 9, ProjectID: 41, TeamID: 7, DeviceID: 12, ConnectorInstanceID: 5,
		CameraIndex: "165-0-7", DeviceSerial: "AIRCRAFT_REDACTED", Status: "requested",
		ConnectorStatus: "connected", ActionEnabled: true, CapabilityVerified: true,
		Instance: connector.Instance{
			ID: 5, ProjectID: 41, ConnectorKey: ConnectorKey, Version: ConnectorVersion,
			DiscoveryScope: json.RawMessage(`{"projectUuid":"11111111-1111-4111-8111-111111111111","projectName":"redacted"}`),
		},
	}
}

func flightHubLiveEvent() outbox.Event {
	return outbox.Event{ProjectID: 41, TeamID: 7, Payload: json.RawMessage(`{"streamId":9}`)}
}

func TestFlightHubLiveStartEncryptsCredentialAndNeverRepeatsWrite(t *testing.T) {
	now := time.Date(2026, 9, 2, 1, 0, 0, 0, time.UTC)
	expiresAt := now.Add(time.Hour)
	store := &memoryFlightHubLiveStore{session: flightHubLiveSessionFixture(now)}
	client := &flightHubLiveClientFixture{value: LiveStreamAuthorization{ExpireTimestamp: expiresAt.Unix(), URL: "supplier-secret", URLType: "volc", ExpiresAt: expiresAt}}
	playback := NormalizedLivePlayback{Description: LivePlaybackDescription{
		Supplier: "volc", Protocol: "volc-rtc", CredentialKind: "sdk-query", AdapterVersion: liveSupplierAdapterVersion,
		ReferenceDigest: strings.Repeat("a", 64), ExpiresAt: expiresAt,
	}, Secret: LivePlaybackSecret{value: "supplier-secret"}}
	handler, err := NewFlightHubLiveStartHandler(store, client, flightHubLiveNormalizerFixture{value: playback}, flightHubLiveResolverFixture{}, flightHubLiveTestSecret, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.Handler(context.Background(), nil, flightHubLiveEvent()); err != nil {
		t.Fatal(err)
	}
	var decrypted map[string]string
	if err := credentials.DecryptJSON(store.envelope, flightHubLiveTestSecret,
		credentials.AAD("flighthub-live-session", store.session.ID, store.session.ProjectID), &decrypted); err != nil {
		t.Fatal(err)
	}
	if decrypted["credential"] != "supplier-secret" || store.session.Status != "starting" || !store.session.StartAcceptedAt.Valid {
		t.Fatalf("session=%#v credential=%v", store.session, decrypted)
	}
	restartedHandler, err := NewFlightHubLiveStartHandler(store, client, flightHubLiveNormalizerFixture{value: playback}, flightHubLiveResolverFixture{}, flightHubLiveTestSecret, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if err := restartedHandler.Handler(context.Background(), nil, flightHubLiveEvent()); err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 {
		t.Fatalf("unsafe live start was repeated %d times", client.calls)
	}
	serialized, _ := json.Marshal(store.playback)
	if strings.Contains(string(serialized), "supplier-secret") {
		t.Fatalf("normalized playback leaked credential: %s", serialized)
	}
}

func TestFlightHubLiveStartUnknownAndUnsupportedSupplierFailClosed(t *testing.T) {
	now := time.Date(2026, 9, 2, 1, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name              string
		clientError       error
		normalizerError   error
		wantStatus        string
		wantReason        string
		remoteUnconfirmed bool
	}{
		{name: "response unknown", clientError: &APIError{SafeCode: "request_timeout", Retryable: true},
			wantStatus: "starting", wantReason: "FLIGHTHUB_LIVE_START_RESPONSE_UNKNOWN"},
		{name: "supplier unsupported", normalizerError: &APIError{SafeCode: "live_supplier_unsupported"},
			wantStatus: "failed", wantReason: "FLIGHTHUB_LIVE_SUPPLIER_UNSUPPORTED_REMOTE_UNCONFIRMED", remoteUnconfirmed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			expiresAt := now.Add(time.Hour)
			store := &memoryFlightHubLiveStore{session: flightHubLiveSessionFixture(now)}
			client := &flightHubLiveClientFixture{value: LiveStreamAuthorization{ExpireTimestamp: expiresAt.Unix(), URL: "opaque", URLType: "future", ExpiresAt: expiresAt}, err: test.clientError}
			handler, _ := NewFlightHubLiveStartHandler(store, client, flightHubLiveNormalizerFixture{err: test.normalizerError}, flightHubLiveResolverFixture{}, flightHubLiveTestSecret, func() time.Time { return now })
			if err := handler.Handler(context.Background(), nil, flightHubLiveEvent()); err != nil {
				t.Fatal(err)
			}
			if store.session.Status != test.wantStatus || store.reason != test.wantReason || store.remoteUnconfirmed != test.remoteUnconfirmed {
				t.Fatalf("status=%s reason=%s unconfirmed=%v", store.session.Status, store.reason, store.remoteUnconfirmed)
			}
			if err := handler.Handler(context.Background(), nil, flightHubLiveEvent()); err != nil {
				t.Fatal(err)
			}
			if client.calls != 1 {
				t.Fatalf("unknown start was repeated %d times", client.calls)
			}
		})
	}
}

func TestFlightHubLiveEvidenceFakeClockConvergesOnlyWithEvidence(t *testing.T) {
	started := time.Date(2026, 9, 2, 2, 0, 0, 0, time.UTC)
	base := FlightHubLiveEvidence{Status: "live", StartedAt: started,
		StartAttemptedAt: sql.NullTime{Time: started, Valid: true}, StartAcceptedAt: sql.NullTime{Time: started, Valid: true}}

	beforeTimeout := decideFlightHubLiveSession(started.Add(5*time.Minute-time.Nanosecond), base)
	if beforeTimeout.Status != "live" || beforeTimeout.Terminal {
		t.Fatalf("no-viewer policy fired early: %#v", beforeTimeout)
	}
	autoStopped := decideFlightHubLiveSession(started.Add(5*time.Minute), base)
	if autoStopped.Status != "stopped" || autoStopped.Reason != "FLIGHTHUB_LIVE_NO_VIEWER_AUTO_STOP" || !autoStopped.Terminal {
		t.Fatalf("no-viewer policy did not converge: %#v", autoStopped)
	}

	offline := decideFlightHubLiveSession(started.Add(time.Minute), FlightHubLiveEvidence{
		Status: "stopping", StartedAt: started, DeviceStatus: "offline",
	})
	if offline.Status != "stopped" || offline.Reason != "FLIGHTHUB_LIVE_DEVICE_OFFLINE" {
		t.Fatalf("offline evidence=%#v", offline)
	}

	credentialExpired := decideFlightHubLiveSession(started.Add(time.Minute), FlightHubLiveEvidence{
		Status: "live", StartedAt: started,
		CredentialExpiresAt: sql.NullTime{Time: started.Add(time.Minute), Valid: true},
	})
	if credentialExpired.Status != "stopped" || credentialExpired.Reason != "FLIGHTHUB_LIVE_CREDENTIAL_EXPIRED" {
		t.Fatalf("credential expiry evidence=%#v", credentialExpired)
	}

	unknown := FlightHubLiveEvidence{Status: "starting", StartedAt: started,
		StartAttemptedAt: sql.NullTime{Time: started, Valid: true}, DeviceStatus: "online"}
	withoutEvidence := decideFlightHubLiveSession(started.Add(time.Hour), unknown)
	if withoutEvidence.Status != "starting" || withoutEvidence.Terminal {
		t.Fatalf("unknown response was falsely completed: %#v", withoutEvidence)
	}
	unknown.LiveAvailable, unknown.LiveCapturedAt = true, sql.NullTime{Time: started.Add(time.Hour), Valid: true}
	confirmedInactive := decideFlightHubLiveSession(started.Add(time.Hour), unknown)
	if confirmedInactive.Status != "failed" || confirmedInactive.Reason != "FLIGHTHUB_LIVE_START_NOT_CONFIRMED" {
		t.Fatalf("fresh inactive evidence=%#v", confirmedInactive)
	}

	contradictory := base
	contradictory.Status = "stopping"
	contradictory.CredentialExpiresAt = sql.NullTime{Time: started.Add(time.Minute), Valid: true}
	contradictory.LiveAvailable, contradictory.LiveActive = true, true
	contradictory.LiveCapturedAt = sql.NullTime{Time: started.Add(2 * time.Minute), Valid: true}
	decision := decideFlightHubLiveSession(started.Add(2*time.Minute), contradictory)
	if decision.Status != "stopping" || decision.Reason != "FLIGHTHUB_LIVE_STOP_REMOTE_ACTIVE" || decision.Terminal {
		t.Fatalf("active evidence lost to expiry inference: %#v", decision)
	}
}

func TestSQLFlightHubLiveReconcilerRecoversAfterWorkerRestart(t *testing.T) {
	databaseURL := os.Getenv("AEROSIGHT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AEROSIGHT_TEST_DATABASE_URL is not configured")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	var teamID, projectID, deviceID int
	var adapterID, definitionID int64
	if err := database.QueryRowContext(ctx, `insert into teams(name) values($1) returning id`, fmt.Sprintf("flighthub-live-%d", suffix)).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.ExecContext(context.Background(), `delete from teams where id=$1`, teamID) })
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, teamID, fmt.Sprintf("flighthub-live-%d", suffix)).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select id from connector_definitions where connector_key=$1 and version=$2`, ConnectorKey, ConnectorVersion).Scan(&definitionID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into device_adapters(project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status)
		values($1,$2,$3,'dji-flighthub2',$4,'2','connected') returning id`, projectID, teamID, fmt.Sprintf("flighthub-live-%d", suffix), definitionID).Scan(&adapterID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into devices(project_id,adapter_id,device_type_id,name,type,status)
		select $1,$2,id,'live test aircraft','drone','online' from device_types
		 where type_key='dji.matrice3td' and status='active' order by version desc limit 1 returning id`, projectID, adapterID).Scan(&deviceID); err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 9, 2, 3, 0, 0, 0, time.UTC)
	var streamID int64
	if err := database.QueryRowContext(ctx, `insert into live_streams(project_id,team_id,device_id,adapter_id,stream_key,source_type,status,
		start_attempted_at,start_accepted_at,supplier_credential_expires_at,started_at)
		values($1,$2,$3,$4,'165-0-7',$5,'starting',$6,$6,$7,$6) returning id`,
		projectID, teamID, deviceID, adapterID, FlightHubLiveSourceType, started, started.Add(time.Hour)).Scan(&streamID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `insert into device_latest_telemetry(device_id,project_id,adapter_id,event_id,telemetry_type,captured_at,received_at,payload_json)
		values($1,$2,$3,$4,'dji.flighthub.state',$5,$5,'{"live":{"available":true,"active":true}}'::jsonb)`,
		deviceID, projectID, adapterID, fmt.Sprintf("live-%d", suffix), started.Add(15*time.Second)); err != nil {
		t.Fatal(err)
	}
	clock := started.Add(15 * time.Second)
	firstWorker, _ := NewFlightHubLiveReconciler(database, func() time.Time { return clock })
	instance := connector.Instance{ID: adapterID, ProjectID: projectID}
	if summary, err := firstWorker.ReconcileLiveSessions(ctx, instance); err != nil || summary.Updated != 1 {
		t.Fatalf("first worker summary=%#v err=%v", summary, err)
	}
	var status string
	if err := database.QueryRowContext(ctx, `select status from live_streams where id=$1`, streamID).Scan(&status); err != nil || status != "live" {
		t.Fatalf("status=%s err=%v", status, err)
	}
	var eventCount int
	var eventPayload []byte
	if err := database.QueryRowContext(ctx, `select count(*),coalesce(max(payload_json::text),'')
		from project_events where project_id=$1 and event_type='live_stream.status_changed'
		  and payload_json->>'streamId'=$2`, projectID, fmt.Sprint(streamID)).Scan(&eventCount, &eventPayload); err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 {
		t.Fatalf("initial live reconcile events=%d payload=%s", eventCount, eventPayload)
	}
	var payload map[string]any
	if err := json.Unmarshal(eventPayload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["status"] != "live" || payload["reason"] != "" || payload["deviceId"] != float64(deviceID) || len(payload) != 4 {
		t.Fatalf("unsafe or incomplete live reconcile payload=%#v", payload)
	}
	if summary, err := firstWorker.ReconcileLiveSessions(ctx, instance); err != nil || summary.Updated != 0 {
		t.Fatalf("idempotent reconcile summary=%#v err=%v", summary, err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from project_events where project_id=$1
		and event_type='live_stream.status_changed' and payload_json->>'streamId'=$2`, projectID, fmt.Sprint(streamID)).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 {
		t.Fatalf("idempotent live reconcile duplicated events=%d", eventCount)
	}
	if _, err := database.ExecContext(ctx, `update live_streams set status='stopping',playback_ref=null,
		local_authorization_revoked_at=$2 where id=$1`, streamID, clock); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(15 * time.Second)
	if _, err := database.ExecContext(ctx, `update device_latest_telemetry set captured_at=$2,received_at=$2,
		payload_json='{"live":{"available":true,"active":false}}'::jsonb where device_id=$1`, deviceID, clock); err != nil {
		t.Fatal(err)
	}
	secondWorker, _ := NewFlightHubLiveReconciler(database, func() time.Time { return clock })
	if summary, err := secondWorker.ReconcileLiveSessions(ctx, instance); err != nil || summary.Updated != 1 {
		t.Fatalf("restarted worker summary=%#v err=%v", summary, err)
	}
	var reason string
	var secretColumnsEmpty bool
	if err := database.QueryRowContext(ctx, `select status,status_reason,
		playback_ref is null and supplier_credential_envelope_json is null from live_streams where id=$1`, streamID).
		Scan(&status, &reason, &secretColumnsEmpty); err != nil {
		t.Fatal(err)
	}
	if status != "stopped" || reason != "FLIGHTHUB_LIVE_REMOTE_INACTIVE" || !secretColumnsEmpty {
		t.Fatalf("status=%s reason=%s secrets-cleared=%v", status, reason, secretColumnsEmpty)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from project_events where project_id=$1
		and event_type='live_stream.status_changed' and payload_json->>'streamId'=$2`, projectID, fmt.Sprint(streamID)).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 2 {
		t.Fatalf("terminal live reconcile events=%d", eventCount)
	}
	if summary, err := secondWorker.ReconcileLiveSessions(ctx, instance); err != nil || summary.Candidates != 0 || summary.Updated != 0 {
		t.Fatalf("terminal reconcile summary=%#v err=%v", summary, err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from project_events where project_id=$1
		and event_type='live_stream.status_changed' and payload_json->>'streamId'=$2`, projectID, fmt.Sprint(streamID)).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 2 {
		t.Fatalf("terminal reconcile duplicated events=%d", eventCount)
	}
}

var _ FlightHubLiveSessionStore = (*memoryFlightHubLiveStore)(nil)
var _ FlightHubLiveStartClient = (*flightHubLiveClientFixture)(nil)
var _ FlightHubLiveNormalizer = flightHubLiveNormalizerFixture{}

func TestFlightHubLiveStartEventRejectsWrongScope(t *testing.T) {
	_, err := parseFlightHubLiveStartEvent(outbox.Event{ProjectID: 41, TeamID: 7, Payload: json.RawMessage(`{"streamId":0}`)})
	if err == nil || !IsSafeCode(err, "request_invalid") {
		t.Fatalf("invalid event error=%v", err)
	}
}
