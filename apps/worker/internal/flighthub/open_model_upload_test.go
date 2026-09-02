package flighthub

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/outbox"
)

type memoryOpenModelUploadStore struct {
	session OpenModelUploadSession
	now     func() time.Time
}

func (store *memoryOpenModelUploadStore) Create(_ context.Context, request OpenModelUploadRequest, envelope json.RawMessage, requestDigest, resourceDigest string) (OpenModelUploadSession, error) {
	if store.session.ID != "" {
		if store.session.RequestDigest != requestDigest {
			return OpenModelUploadSession{}, &APIError{SafeCode: "idempotency_conflict"}
		}
		return store.session, nil
	}
	store.session = OpenModelUploadSession{
		ID: "UPLOAD_SESSION_REDACTED", ProjectID: request.ProjectID, TeamID: 2, ConnectorInstanceID: request.ConnectorInstanceID,
		RequestedByUserID: request.RequestedByUserID, IdempotencyKey: request.IdempotencyKey, RequestDigest: requestDigest,
		ResourceUUIDDigest: resourceDigest, Status: "requested", RequestEnvelope: envelope, ConnectorStatus: "connected", ActionEnabled: true,
		Instance: connector.Instance{ID: request.ConnectorInstanceID, ProjectID: request.ProjectID, ConnectorKey: ConnectorKey,
			Version: ConnectorVersion, DiscoveryScope: json.RawMessage(`{"projectUuid":"` + runtimeProjectUUID + `","projectName":"脱敏项目"}`)},
	}
	return store.session, nil
}

func (store *memoryOpenModelUploadStore) Load(_ context.Context, projectID int, sessionID string) (OpenModelUploadSession, error) {
	if store.session.ID != sessionID || store.session.ProjectID != projectID {
		return OpenModelUploadSession{}, &APIError{SafeCode: "scope_forbidden"}
	}
	return store.session, nil
}

func (store *memoryOpenModelUploadStore) SaveCredential(_ context.Context, _ OpenModelUploadSession, envelope json.RawMessage, expiresAt time.Time) error {
	store.session.Status, store.session.CredentialEnvelope, store.session.CredentialExpiresAt = "credential_ready", envelope, expiresAt
	return nil
}

func (store *memoryOpenModelUploadStore) PrepareCallback(_ context.Context, _ OpenModelUploadSession, envelope json.RawMessage, digest, resourceDigest string) error {
	if subtle.ConstantTimeCompare([]byte(store.session.ResourceUUIDDigest), []byte(resourceDigest)) != 1 {
		return &APIError{SafeCode: "scope_forbidden"}
	}
	if store.session.CallbackDigest != "" {
		if store.session.CallbackDigest != digest {
			return &APIError{SafeCode: "idempotency_conflict"}
		}
		return nil
	}
	if !store.session.CredentialExpiresAt.After(store.now()) {
		store.session.Status, store.session.CredentialEnvelope = "expired", nil
		return &APIError{SafeCode: "credential_expired"}
	}
	store.session.Status, store.session.CallbackEnvelope, store.session.CallbackDigest = "callback_pending", envelope, digest
	return nil
}

func (store *memoryOpenModelUploadStore) BeginCallback(context.Context, OpenModelUploadSession) error {
	store.session.Status, store.session.CallbackAttempts = "reconciling", 1
	return nil
}

func (store *memoryOpenModelUploadStore) RecordReconciliation(_ context.Context, _ OpenModelUploadSession, code string) error {
	store.session.ReconciliationCount++
	store.session.LastErrorCode = code
	return nil
}

func (store *memoryOpenModelUploadStore) Complete(_ context.Context, _ OpenModelUploadSession, _ string) error {
	store.session.Status, store.session.LastErrorCode = "succeeded", ""
	store.session.CredentialEnvelope, store.session.CallbackEnvelope = nil, nil
	return nil
}

func (store *memoryOpenModelUploadStore) Finish(_ context.Context, _ OpenModelUploadSession, status, code string) error {
	store.session.Status, store.session.LastErrorCode = status, code
	store.session.CredentialEnvelope, store.session.CallbackEnvelope = nil, nil
	return nil
}

type openModelUploadClientFixture struct {
	credentialCalls, callbackCalls, resourceCalls int
	credential                                    OpenModelUploadCredential
	credentialErr, callbackErr, resourceErr       error
	callbackResult                                OpenModelUploadCallbackResult
	resource                                      OpenModelResource
}

func (client *openModelUploadClientFixture) ObtainOpenModelUploadCredential(context.Context, string, string) (OpenModelUploadCredential, error) {
	client.credentialCalls++
	return client.credential, client.credentialErr
}

func (client *openModelUploadClientFixture) NotifyOpenModelUploadComplete(context.Context, string, string, OpenModelUploadCallbackRequest) (OpenModelUploadCallbackResult, error) {
	client.callbackCalls++
	return client.callbackResult, client.callbackErr
}

func (client *openModelUploadClientFixture) GetOpenModelResource(context.Context, string, string, string) (OpenModelResource, error) {
	client.resourceCalls++
	return client.resource, client.resourceErr
}

func uploadSessionEvent(eventType string) outbox.Event {
	return outbox.Event{ProjectID: 3, TeamID: 2, EventType: eventType, Payload: json.RawMessage(`{"sessionId":"UPLOAD_SESSION_REDACTED"}`)}
}

func uploadSessionRequest() OpenModelUploadRequest {
	return OpenModelUploadRequest{ProjectID: 3, ConnectorInstanceID: 7, RequestedByUserID: 5, IdempotencyKey: "upload-session-key", ResourceUUID: "RESOURCE_UPLOAD_REDACTED", ResourceName: "脱敏资源"}
}

func uploadCompletionRequest() OpenModelUploadCompletionRequest {
	return OpenModelUploadCompletionRequest{ProjectID: 3, SessionID: "UPLOAD_SESSION_REDACTED", ResourceUUID: "RESOURCE_UPLOAD_REDACTED", ResourceName: "脱敏资源",
		Files: []OpenModelUploadedFile{{Name: "synthetic-1.jpg", ETag: "ETAG_REDACTED_1"}, {Name: "synthetic-2.jpg", ETag: "ETAG_REDACTED_2"}}}
}

func readyUploadHandler(t *testing.T, now *time.Time, callbackErr error) (*OpenModelUploadHandler, *memoryOpenModelUploadStore, *openModelUploadClientFixture, *modelJobProjectorFixture) {
	t.Helper()
	store := &memoryOpenModelUploadStore{now: func() time.Time { return *now }}
	client := &openModelUploadClientFixture{
		credential: OpenModelUploadCredential{CloudName: "ali", AccessKeyID: "ACCESS_SECRET", SecretAccessKey: "KEY_SECRET",
			SessionToken: "SESSION_SECRET", Region: "cn-synthetic-1", BucketName: "BUCKET_REDACTED", CallbackParam: "CALLBACK_SECRET",
			StorePath: "open-model/project/{fileName}", Endpoint: "https://objects.vendor.example", ExpiresAt: now.Add(10 * time.Minute)},
		callbackErr: callbackErr,
		callbackResult: OpenModelUploadCallbackResult{ResourceUUID: "RESOURCE_UPLOAD_REDACTED", UploadCount: 2,
			FileNames: []string{"synthetic-1.jpg", "synthetic-2.jpg"}},
		resource: OpenModelResource{ResourceUUID: "RESOURCE_UPLOAD_REDACTED", Status: 1, FileNames: []string{"synthetic-1.jpg", "synthetic-2.jpg"}},
	}
	projector := &modelJobProjectorFixture{}
	handler, err := NewOpenModelUploadHandler(store, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, projector, flightProjectorTestSecret, func() time.Time { return *now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handler.RequestCredential(context.Background(), uploadSessionRequest()); err != nil {
		t.Fatal(err)
	}
	if err := handler.Handler(context.Background(), nil, uploadSessionEvent(OpenModelUploadCredentialEventType)); err != nil {
		t.Fatal(err)
	}
	return handler, store, client, projector
}

func TestOpenModelUploadCredentialExpiresAndIsErased(t *testing.T) {
	now := time.Date(2026, 9, 2, 13, 0, 0, 0, time.UTC)
	handler, store, client, _ := readyUploadHandler(t, &now, nil)
	credential, err := handler.Credential(context.Background(), 3, store.session.ID)
	if err != nil || credential.SessionToken != "SESSION_SECRET" || client.credentialCalls != 1 {
		t.Fatalf("credential=%#v err=%v calls=%d", credential, err, client.credentialCalls)
	}
	if stringsContainsAny(string(store.session.CredentialEnvelope), "ACCESS_SECRET", "KEY_SECRET", "SESSION_SECRET", "CALLBACK_SECRET") {
		t.Fatal("credential plaintext persisted")
	}
	now = now.Add(11 * time.Minute)
	if _, err := handler.Credential(context.Background(), 3, store.session.ID); !IsSafeCode(err, "credential_expired") || store.session.Status != "expired" || len(store.session.CredentialEnvelope) != 0 {
		t.Fatalf("err=%v session=%#v", err, store.session)
	}
}

func TestOpenModelUploadRepeatedCallbackCallsUpstreamOnce(t *testing.T) {
	now := time.Date(2026, 9, 2, 14, 0, 0, 0, time.UTC)
	handler, store, client, projector := readyUploadHandler(t, &now, nil)
	completion := uploadCompletionRequest()
	if _, err := handler.SubmitCallback(context.Background(), completion); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.SubmitCallback(context.Background(), completion); err != nil {
		t.Fatal(err)
	}
	event := uploadSessionEvent(OpenModelUploadCallbackEventType)
	if err := handler.Handler(context.Background(), nil, event); err != nil {
		t.Fatal(err)
	}
	if err := handler.Handler(context.Background(), nil, event); err != nil || client.callbackCalls != 1 || store.session.Status != "succeeded" || len(projector.polls) != 1 || len(store.session.CredentialEnvelope) != 0 || len(store.session.CallbackEnvelope) != 0 {
		t.Fatalf("err=%v callbacks=%d session=%#v polls=%d", err, client.callbackCalls, store.session, len(projector.polls))
	}
}

func TestOpenModelUploadRejectsWrongResourceBeforeCallback(t *testing.T) {
	now := time.Date(2026, 9, 2, 15, 0, 0, 0, time.UTC)
	handler, store, client, _ := readyUploadHandler(t, &now, nil)
	completion := uploadCompletionRequest()
	completion.ResourceUUID = "RESOURCE_OTHER_REDACTED"
	if _, err := handler.SubmitCallback(context.Background(), completion); !IsSafeCode(err, "scope_forbidden") || client.callbackCalls != 0 || store.session.CallbackDigest != "" {
		t.Fatalf("err=%v callbacks=%d session=%#v", err, client.callbackCalls, store.session)
	}
}

func TestOpenModelUploadUnknownCallbackResponseReconcilesWithoutRepeat(t *testing.T) {
	now := time.Date(2026, 9, 2, 16, 0, 0, 0, time.UTC)
	handler, store, client, projector := readyUploadHandler(t, &now, &APIError{SafeCode: "upstream_unavailable", Retryable: true})
	if _, err := handler.SubmitCallback(context.Background(), uploadCompletionRequest()); err != nil {
		t.Fatal(err)
	}
	event := uploadSessionEvent(OpenModelUploadCallbackEventType)
	if err := handler.Handler(context.Background(), nil, event); !IsSafeCode(err, "upstream_unavailable") || client.callbackCalls != 1 || store.session.Status != "reconciling" {
		t.Fatalf("err=%v callbacks=%d session=%#v", err, client.callbackCalls, store.session)
	}
	now = now.Add(20 * time.Minute)
	client.callbackErr = nil
	if err := handler.Handler(context.Background(), nil, event); err != nil || client.callbackCalls != 1 || client.resourceCalls != 1 || store.session.Status != "succeeded" || len(projector.polls) != 1 {
		t.Fatalf("err=%v callbacks=%d resources=%d session=%#v", err, client.callbackCalls, client.resourceCalls, store.session)
	}
}

func TestOpenModelUploadWrongCallbackResultFailsClosed(t *testing.T) {
	now := time.Date(2026, 9, 2, 17, 0, 0, 0, time.UTC)
	handler, store, client, projector := readyUploadHandler(t, &now, nil)
	client.callbackResult.ResourceUUID = "RESOURCE_OTHER_REDACTED"
	if _, err := handler.SubmitCallback(context.Background(), uploadCompletionRequest()); err != nil {
		t.Fatal(err)
	}
	if err := handler.Handler(context.Background(), nil, uploadSessionEvent(OpenModelUploadCallbackEventType)); err != nil || store.session.Status != "blocked" || store.session.LastErrorCode != "resource_scope_mismatch" || len(projector.polls) != 0 {
		t.Fatalf("err=%v session=%#v polls=%d", err, store.session, len(projector.polls))
	}
}

func stringsContainsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func TestSQLOpenModelUploadIsScopedIdempotentSecretSafeAndAssetLinked(t *testing.T) {
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
	var userID, teamID, projectID int
	var adapterID, definitionID int64
	if err := database.QueryRowContext(ctx, `insert into users(name,email) values($1,$2) returning id`, "open model uploader", fmt.Sprintf("open-model-upload-%d@example.test", suffix)).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into teams(name) values($1) returning id`, fmt.Sprintf("open-model-upload-%d", suffix)).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(), `delete from teams where id=$1`, teamID)
		_, _ = database.ExecContext(context.Background(), `delete from users where id=$1`, userID)
	})
	if _, err := database.ExecContext(ctx, `insert into team_members(team_id,user_id,role) values($1,$2,'owner')`, teamID, userID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name,created_by_user_id) values($1,$2,$3) returning id`, teamID, fmt.Sprintf("open-model-upload-%d", suffix), userID).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `insert into project_feature_flags(project_id,flighthub_action_flags_json)
		values($1,'{"security.temporary-credential":true,"model.write":true}'::jsonb)`, projectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select id from connector_definitions where connector_key=$1 and version=$2`, ConnectorKey, ConnectorVersion).Scan(&definitionID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into device_adapters(project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status,discovery_scope_json,credential_envelope_json)
		values($1,$2,$3,'dji-flighthub2',$4,'2','connected',$5,'{}'::jsonb) returning id`, projectID, teamID,
		fmt.Sprintf("open-model-upload-%d", suffix), definitionID,
		fmt.Sprintf(`{"projectUuid":"%s","projectName":"脱敏项目"}`, runtimeProjectUUID)).Scan(&adapterID); err != nil {
		t.Fatal(err)
	}
	clock := time.Now().UTC()
	client := &openModelUploadClientFixture{
		credential: OpenModelUploadCredential{CloudName: "ali", AccessKeyID: "ACCESS_SECRET", SecretAccessKey: "KEY_SECRET",
			SessionToken: "SESSION_SECRET", Region: "cn-synthetic-1", BucketName: "BUCKET_REDACTED", CallbackParam: "CALLBACK_SECRET",
			StorePath: "open-model/project/{fileName}", Endpoint: "https://objects.vendor.example", ExpireTimestamp: clock.Add(10 * time.Minute).Unix(), ExpiresAt: clock.Add(10 * time.Minute)},
		callbackResult: OpenModelUploadCallbackResult{ResourceUUID: "RESOURCE_SQL_REDACTED", UploadCount: 1, FileNames: []string{"synthetic.jpg"}},
		resource:       OpenModelResource{ResourceUUID: "RESOURCE_SQL_REDACTED", Status: 1, FileNames: []string{"synthetic.jpg"}},
	}
	projector := NewSQLFlightCatalogProjector(database, &telemetryIngestorFixture{}, func() time.Time { return clock }, 30*time.Minute, flightProjectorTestSecret)
	sink, err := NewSQLResourceStreamSink(&telemetryIngestorFixture{}, connector.NewSQLResourceRepository(database), &freshnessProjectorFixture{}, &healthProjectorFixture{}, projector)
	if err != nil {
		t.Fatal(err)
	}
	store := NewSQLOpenModelUploadStore(database)
	handler, err := NewOpenModelUploadHandler(store, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, sink, flightProjectorTestSecret, func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	request := OpenModelUploadRequest{ProjectID: projectID, ConnectorInstanceID: adapterID, RequestedByUserID: userID,
		IdempotencyKey: "sql-upload-session-key", ResourceUUID: "RESOURCE_SQL_REDACTED", ResourceName: "数据库脱敏资源"}
	session, err := handler.RequestCredential(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := handler.RequestCredential(ctx, request)
	if err != nil || repeated.ID != session.ID {
		t.Fatalf("session=%#v repeated=%#v err=%v", session, repeated, err)
	}
	credentialEvent := outbox.Event{ProjectID: projectID, TeamID: teamID, EventType: OpenModelUploadCredentialEventType,
		Payload: json.RawMessage(`{"sessionId":"` + session.ID + `"}`)}
	if err := handler.Handler(ctx, nil, credentialEvent); err != nil {
		t.Fatal(err)
	}
	leased, err := handler.Credential(ctx, projectID, session.ID)
	if err != nil || leased.CallbackParam != "CALLBACK_SECRET" || !leased.ExpiresAt.Equal(client.credential.ExpiresAt) {
		t.Fatalf("credential=%#v err=%v", leased, err)
	}
	var persisted string
	if err := database.QueryRowContext(ctx, `select row_to_json(upload)::text from connector_open_model_uploads upload where id=$1::uuid`, session.ID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if stringsContainsAny(persisted, "RESOURCE_SQL_REDACTED", "数据库脱敏资源", "ACCESS_SECRET", "KEY_SECRET", "SESSION_SECRET", "CALLBACK_SECRET") {
		t.Fatal("open model upload plaintext persisted")
	}
	wrong := OpenModelUploadCompletionRequest{ProjectID: projectID, SessionID: session.ID, ResourceUUID: "RESOURCE_OTHER_REDACTED",
		Files: []OpenModelUploadedFile{{Name: "synthetic.jpg", ETag: "ETAG_SECRET"}}}
	if _, err := handler.SubmitCallback(ctx, wrong); !IsSafeCode(err, "scope_forbidden") || client.callbackCalls != 0 {
		t.Fatalf("wrong resource err=%v calls=%d", err, client.callbackCalls)
	}
	completion := OpenModelUploadCompletionRequest{ProjectID: projectID, SessionID: session.ID, ResourceUUID: "RESOURCE_SQL_REDACTED",
		ResourceName: "数据库脱敏资源", Files: []OpenModelUploadedFile{{Name: "synthetic.jpg", ETag: "ETAG_SECRET"}}}
	if _, err := handler.SubmitCallback(ctx, completion); err != nil {
		t.Fatal(err)
	}
	callbackEvent := outbox.Event{ProjectID: projectID, TeamID: teamID, EventType: OpenModelUploadCallbackEventType,
		Payload: json.RawMessage(`{"sessionId":"` + session.ID + `"}`)}
	if err := handler.Handler(ctx, nil, callbackEvent); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.SubmitCallback(ctx, completion); err != nil {
		t.Fatal(err)
	}
	if err := handler.Handler(ctx, nil, callbackEvent); err != nil || client.callbackCalls != 1 {
		t.Fatalf("repeated callback err=%v calls=%d", err, client.callbackCalls)
	}
	var linked, envelopesCleared bool
	if err := database.QueryRowContext(ctx, `select upload.status='succeeded' and upload.asset_id=resource.canonical_target_id::integer,
		upload.credential_envelope_json is null and upload.callback_envelope_json is null
		from connector_open_model_uploads upload join connector_remote_resources resource on resource.id=upload.remote_resource_id
		where upload.id=$1::uuid and resource.project_id=$2 and resource.remote_id='resource:RESOURCE_SQL_REDACTED'`, session.ID, projectID).Scan(&linked, &envelopesCleared); err != nil {
		t.Fatal(err)
	}
	if !linked || !envelopesCleared {
		t.Fatalf("linked=%t envelopesCleared=%t", linked, envelopesCleared)
	}
	if err := database.QueryRowContext(ctx, `select row_to_json(upload)::text from connector_open_model_uploads upload where id=$1::uuid`, session.ID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if stringsContainsAny(persisted, "RESOURCE_SQL_REDACTED", "数据库脱敏资源", "ETAG_SECRET", "ACCESS_SECRET", "KEY_SECRET", "SESSION_SECRET", "CALLBACK_SECRET") {
		t.Fatal("completed open model upload leaked plaintext")
	}
}
