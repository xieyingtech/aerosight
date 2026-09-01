package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/outbox"
)

const waylineUploadTestSecret = "0123456789abcdef0123456789abcdef"

type memoryWaylineUploadStore struct {
	job WaylineUploadJob
}

func (store *memoryWaylineUploadStore) Create(context.Context, WaylineUploadRequest) (WaylineUploadJob, error) {
	return store.job, nil
}
func (store *memoryWaylineUploadStore) Load(_ context.Context, projectID int, jobID string) (WaylineUploadJob, error) {
	if store.job.ProjectID != projectID || store.job.ID != jobID {
		return WaylineUploadJob{}, &APIError{SafeCode: "scope_forbidden"}
	}
	return store.job, nil
}
func (store *memoryWaylineUploadStore) MarkUploading(_ context.Context, job WaylineUploadJob) error {
	store.job.Status = "uploading"
	return nil
}
func (store *memoryWaylineUploadStore) MarkUploaded(_ context.Context, _ WaylineUploadJob, envelope json.RawMessage, _ string) error {
	store.job.Status = "notifying"
	store.job.ObjectKeyEnvelope = append(json.RawMessage(nil), envelope...)
	return nil
}
func (store *memoryWaylineUploadStore) BeginNotification(_ context.Context, _ WaylineUploadJob) (int, error) {
	if store.job.NotificationAttemptCount >= maxNotificationAttempts {
		return 0, errors.New("notification limit")
	}
	store.job.NotificationAttemptCount++
	store.job.Status = "reconciling"
	store.job.LastErrorCode = ""
	return store.job.NotificationAttemptCount, nil
}
func (store *memoryWaylineUploadStore) RecordError(_ context.Context, _ WaylineUploadJob, code string) error {
	store.job.LastErrorCode = code
	return nil
}
func (store *memoryWaylineUploadStore) RecordReconciliationMiss(_ context.Context, _ WaylineUploadJob) (int, error) {
	store.job.ReconciliationMissCount++
	return store.job.ReconciliationMissCount, nil
}
func (store *memoryWaylineUploadStore) Complete(_ context.Context, _ WaylineUploadJob, result WaylineUploadResult) error {
	store.job.Status = "succeeded"
	store.job.LastErrorCode = ""
	if result.UUID == "" {
		return errors.New("missing remote UUID")
	}
	return nil
}
func (store *memoryWaylineUploadStore) Fail(_ context.Context, _ WaylineUploadJob, code string) error {
	store.job.Status = "failed"
	store.job.LastErrorCode = code
	return nil
}

type waylineUploadClientFixture struct {
	calls          *[]string
	waylines       []WaylineSummary
	notifyFailures int
}

func (client *waylineUploadClientFixture) CreateStorageSTS(context.Context, string, string, StorageSTSRequest) (StorageSTS, error) {
	*client.calls = append(*client.calls, "sts")
	return StorageSTS{
		Endpoint: "https://objects.example", Provider: "minio", Region: "test", Bucket: "waylines",
		ObjectKeyPrefix: "organization/project/file",
		Credentials:     StorageSTSCredentials{AccessKeyID: "STS_ACCESS_SECRET", AccessKeySecret: "STS_KEY_SECRET", SecurityToken: "STS_TOKEN_SECRET", ExpireSeconds: 3600},
	}, nil
}
func (client *waylineUploadClientFixture) NotifyWaylineUploadComplete(_ context.Context, _, _ string, request WaylineUploadCompleteRequest) (WaylineUploadResult, error) {
	*client.calls = append(*client.calls, "notify")
	if client.notifyFailures > 0 {
		client.notifyFailures--
		client.waylines = append(client.waylines, WaylineSummary{ID: "REMOTE_REDACTED", Name: request.Name})
		return WaylineUploadResult{}, &APIError{SafeCode: "request_timeout", Retryable: true}
	}
	return WaylineUploadResult{Name: request.Name, UUID: "REMOTE_REDACTED"}, nil
}
func (client *waylineUploadClientFixture) ListWaylines(context.Context, string, string) ([]WaylineSummary, error) {
	*client.calls = append(*client.calls, "list")
	return append([]WaylineSummary(nil), client.waylines...), nil
}

type waylineTokenResolverFixture struct{}

func (waylineTokenResolverFixture) ResolveToken(context.Context, connector.Instance) (string, error) {
	return "TOKEN_SECRET", nil
}

type waylineSourceFixture struct{ calls *[]string }

func (source waylineSourceFixture) ReadWaylineSource(context.Context, string) (WaylineSourceObject, error) {
	*source.calls = append(*source.calls, "source")
	return WaylineSourceObject{Body: []byte("sanitized kmz"), ContentType: "application/vnd.google-earth.kmz"}, nil
}

type waylineUploaderFixture struct{ calls *[]string }

func (uploader waylineUploaderFixture) Upload(_ context.Context, sts StorageSTS, key string, body io.Reader, _ int64, _ string) error {
	*uploader.calls = append(*uploader.calls, "upload")
	if !strings.HasPrefix(key, sts.ObjectKeyPrefix+"/") {
		return errors.New("key escaped STS prefix")
	}
	_, err := io.ReadAll(body)
	return err
}

func uploadFixtureJob() WaylineUploadJob {
	scope, _ := json.Marshal(map[string]string{"projectUuid": "11111111-1111-4111-8111-111111111111", "projectName": "test"})
	return WaylineUploadJob{
		ID: "11111111-2222-4333-8444-555555555555", ProjectID: 41, TeamID: 42,
		ConnectorInstanceID: 43, OperationKind: "wayline", SourceAssetID: 44, RequestedByUserID: 45,
		IdempotencyKey: "upload-test-key", RequestedName: "巡检航线",
		ReconciliationName: "巡检航线 · AeroSight-a1b2c3d4e5f6", Status: "queued",
		SourceStorageKey: "projects/41/assets/wayline.kmz", SourceContentType: "application/vnd.google-earth.kmz",
		SourceStatus: "available", ConnectorStatus: "connected", ActionEnabled: true, Instance: connector.Instance{
			ID: 43, ProjectID: 41, ConnectorKey: ConnectorKey, Version: ConnectorVersion,
			DiscoveryScope: scope, CredentialEnvelope: json.RawMessage(`{"redacted":true}`),
		},
	}
}

func waylineUploadEvent(job WaylineUploadJob) outbox.Event {
	payload, _ := json.Marshal(map[string]string{"jobId": job.ID})
	return outbox.Event{ProjectID: job.ProjectID, TeamID: job.TeamID, Payload: payload}
}

func TestWaylineUploadLostResponseReconcilesBeforeAnyRetry(t *testing.T) {
	store := &memoryWaylineUploadStore{job: uploadFixtureJob()}
	calls := []string{}
	client := &waylineUploadClientFixture{calls: &calls, notifyFailures: 1}
	handler, err := NewWaylineUploadHandler(store, client, waylineTokenResolverFixture{}, waylineSourceFixture{&calls}, waylineUploaderFixture{&calls}, waylineUploadTestSecret)
	if err != nil {
		t.Fatal(err)
	}
	event := waylineUploadEvent(store.job)
	if err := handler.Handler(context.Background(), nil, event); !IsSafeCode(err, "request_timeout") {
		t.Fatalf("first attempt error=%v", err)
	}
	if err := handler.Handler(context.Background(), nil, event); err != nil {
		t.Fatal(err)
	}
	want := []string{"source", "sts", "upload", "notify", "list"}
	if !reflect.DeepEqual(calls, want) || store.job.Status != "succeeded" || store.job.NotificationAttemptCount != 1 {
		t.Fatalf("calls=%v job=%#v", calls, store.job)
	}
	persisted := string(store.job.ObjectKeyEnvelope)
	for _, secret := range []string{"STS_ACCESS_SECRET", "STS_KEY_SECRET", "STS_TOKEN_SECRET", "organization/project/file/wayline.kmz"} {
		if strings.Contains(persisted, secret) {
			t.Fatalf("persisted upload checkpoint leaked %q", secret)
		}
	}
}

func TestWaylineUploadRetriesNotificationOnlyAfterBoundedRemoteMisses(t *testing.T) {
	store := &memoryWaylineUploadStore{job: uploadFixtureJob()}
	calls := []string{}
	client := &waylineUploadClientFixture{calls: &calls, notifyFailures: 1}
	handler, _ := NewWaylineUploadHandler(store, client, waylineTokenResolverFixture{}, waylineSourceFixture{&calls}, waylineUploaderFixture{&calls}, waylineUploadTestSecret)
	event := waylineUploadEvent(store.job)
	if err := handler.Handler(context.Background(), nil, event); err == nil {
		t.Fatal("lost response did not request reconciliation")
	}
	client.waylines = nil
	if err := handler.Handler(context.Background(), nil, event); !IsSafeCode(err, "reconciliation_pending") {
		t.Fatalf("first miss=%v", err)
	}
	if err := handler.Handler(context.Background(), nil, event); err != nil {
		t.Fatal(err)
	}
	want := []string{"source", "sts", "upload", "notify", "list", "list", "notify"}
	if !reflect.DeepEqual(calls, want) || store.job.NotificationAttemptCount != 2 || store.job.Status != "succeeded" {
		t.Fatalf("calls=%v job=%#v", calls, store.job)
	}
}

func TestWaylineUploadAmbiguousReconciliationFailsClosed(t *testing.T) {
	store := &memoryWaylineUploadStore{job: uploadFixtureJob()}
	store.job.Status = "reconciling"
	store.job.NotificationAttemptCount = 1
	calls := []string{}
	client := &waylineUploadClientFixture{calls: &calls, waylines: []WaylineSummary{
		{ID: "REMOTE_1", Name: store.job.ReconciliationName}, {ID: "REMOTE_2", Name: store.job.ReconciliationName},
	}}
	handler, _ := NewWaylineUploadHandler(store, client, waylineTokenResolverFixture{}, waylineSourceFixture{&calls}, waylineUploaderFixture{&calls}, waylineUploadTestSecret)
	if err := handler.Handler(context.Background(), nil, waylineUploadEvent(store.job)); err != nil {
		t.Fatal(err)
	}
	if store.job.Status != "failed" || store.job.LastErrorCode != "reconciliation_ambiguous" || !reflect.DeepEqual(calls, []string{"list"}) {
		t.Fatalf("calls=%v job=%#v", calls, store.job)
	}
}

func TestWaylineUploadDisabledOrCrossProjectScopeMakesNoUpstreamCalls(t *testing.T) {
	for _, mutate := range []func(*WaylineUploadJob, *outbox.Event){
		func(job *WaylineUploadJob, _ *outbox.Event) { job.ConnectorStatus = "disabled" },
		func(job *WaylineUploadJob, _ *outbox.Event) { job.ActionEnabled = false },
		func(_ *WaylineUploadJob, event *outbox.Event) { event.TeamID++ },
	} {
		store := &memoryWaylineUploadStore{job: uploadFixtureJob()}
		calls := []string{}
		client := &waylineUploadClientFixture{calls: &calls}
		handler, _ := NewWaylineUploadHandler(store, client, waylineTokenResolverFixture{}, waylineSourceFixture{&calls}, waylineUploaderFixture{&calls}, waylineUploadTestSecret)
		event := waylineUploadEvent(store.job)
		mutate(&store.job, &event)
		if err := handler.Handler(context.Background(), nil, event); err != nil {
			t.Fatal(err)
		}
		if len(calls) != 0 || store.job.Status != "failed" {
			t.Fatalf("calls=%v job=%#v", calls, store.job)
		}
	}
}

func TestWaylineObjectKeyCannotEscapeSTSPrefix(t *testing.T) {
	for _, prefix := range []string{"", "/absolute", "tenant/../other", "tenant//other", "tenant\\other"} {
		if key, err := waylineObjectKey(prefix); err == nil {
			t.Fatalf("prefix %q produced key %q", prefix, key)
		}
	}
	key, err := waylineObjectKey("tenant/project/file/")
	if err != nil || key != "tenant/project/file/wayline.kmz" {
		t.Fatalf("key=%q err=%v", key, err)
	}
}

func TestFinishUploadClientDoesNotInternallyRetryUnknownResponse(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		calls++
		response.WriteHeader(http.StatusServiceUnavailable)
		_, _ = response.Write([]byte(`{"code":200500,"message":"redacted","data":{}}`))
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, 3)
	_, err := client.NotifyWaylineUploadComplete(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", WaylineUploadCompleteRequest{
		Name: "reconciliation-name", ObjectKey: "tenant/project/file/wayline.kmz",
	})
	if err == nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestSQLWaylineUploadJobIsIdempotentScopedSecretSafeAndRestartable(t *testing.T) {
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
	var userID, teamID, projectID, otherProjectID, assetID, otherAssetID int
	var adapterID, definitionID int64
	if err := database.QueryRowContext(ctx, `insert into users(name,email) values($1,$2) returning id`,
		"wayline uploader", fmt.Sprintf("wayline-%d@example.test", suffix)).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.ExecContext(context.Background(), `delete from users where id=$1`, userID) })
	if err := database.QueryRowContext(ctx, `insert into teams(name) values($1) returning id`, fmt.Sprintf("wayline-%d", suffix)).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.ExecContext(context.Background(), `delete from teams where id=$1`, teamID) })
	if _, err := database.ExecContext(ctx, `insert into team_members(team_id,user_id,role) values($1,$2,'owner')`, teamID, userID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, teamID, fmt.Sprintf("wayline-%d", suffix)).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `insert into project_feature_flags(project_id,flighthub_action_flags_json)
		values($1,'{"security.temporary-credential":true,"flight.execute":true}'::jsonb)
		on conflict(project_id) do update set flighthub_action_flags_json=excluded.flighthub_action_flags_json`, projectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, teamID, fmt.Sprintf("wayline-other-%d", suffix)).Scan(&otherProjectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select id from connector_definitions where connector_key=$1 and version=$2`, ConnectorKey, ConnectorVersion).Scan(&definitionID); err != nil {
		t.Fatal(err)
	}
	scope := `{"projectUuid":"11111111-1111-4111-8111-111111111111","projectName":"脱敏项目"}`
	if err := database.QueryRowContext(ctx, `insert into device_adapters(
		project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status,discovery_scope_json,credential_envelope_json
	) values($1,$2,$3,'dji-flighthub2',$4,'2','connected',$5,'{}'::jsonb) returning id`,
		projectID, teamID, fmt.Sprintf("wayline-%d", suffix), definitionID, scope).Scan(&adapterID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into assets(
		project_id,team_id,kind,mime_type,storage_key,logical_key,status,size_bytes
	) values($1,$2,'wayline','application/vnd.google-earth.kmz',$3,$4,'available',13) returning id`,
		projectID, teamID, fmt.Sprintf("projects/%d/assets/wayline.kmz", projectID), fmt.Sprintf("wayline/%d", suffix)).Scan(&assetID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into assets(
		project_id,team_id,kind,mime_type,storage_key,logical_key,status,size_bytes
	) values($1,$2,'wayline','application/vnd.google-earth.kmz',$3,$4,'available',13) returning id`,
		otherProjectID, teamID, fmt.Sprintf("projects/%d/assets/wayline.kmz", otherProjectID), fmt.Sprintf("wayline-other/%d", suffix)).Scan(&otherAssetID); err != nil {
		t.Fatal(err)
	}

	store := NewSQLWaylineUploadStore(database)
	request := WaylineUploadRequest{
		ProjectID: projectID, ConnectorInstanceID: adapterID, SourceAssetID: assetID,
		RequestedByUserID: userID, IdempotencyKey: fmt.Sprintf("upload-%d", suffix), Name: "持久巡检航线",
	}
	job, err := store.Create(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := store.Create(ctx, request)
	if err != nil || replayed.ID != job.ID {
		t.Fatalf("replayed=%#v err=%v", replayed, err)
	}
	var jobCount, eventCount int
	if err := database.QueryRowContext(ctx, `select count(*) from connector_object_upload_jobs where project_id=$1`, projectID).Scan(&jobCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from outbox_events where event_id=$1`, "flighthub-wayline-upload:"+job.ID).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if jobCount != 1 || eventCount != 1 {
		t.Fatalf("jobs=%d events=%d", jobCount, eventCount)
	}
	crossProject := request
	crossProject.IdempotencyKey += "-cross"
	crossProject.SourceAssetID = otherAssetID
	if _, err := store.Create(ctx, crossProject); !IsSafeCode(err, "scope_forbidden") {
		t.Fatalf("cross-project asset error=%v", err)
	}

	calls := []string{}
	client := &waylineUploadClientFixture{calls: &calls, notifyFailures: 1}
	firstWorker, err := NewWaylineUploadHandler(store, client, waylineTokenResolverFixture{}, waylineSourceFixture{&calls}, waylineUploaderFixture{&calls}, waylineUploadTestSecret)
	if err != nil {
		t.Fatal(err)
	}
	event := outbox.Event{ProjectID: projectID, TeamID: teamID, Payload: json.RawMessage(fmt.Sprintf(`{"jobId":%q}`, job.ID))}
	if err := firstWorker.Handler(ctx, nil, event); !IsSafeCode(err, "request_timeout") {
		t.Fatalf("first worker error=%v", err)
	}
	secondWorker, err := NewWaylineUploadHandler(NewSQLWaylineUploadStore(database), client, waylineTokenResolverFixture{}, waylineSourceFixture{&calls}, waylineUploaderFixture{&calls}, waylineUploadTestSecret)
	if err != nil {
		t.Fatal(err)
	}
	if err := secondWorker.Handler(ctx, nil, event); err != nil {
		t.Fatal(err)
	}
	var status string
	var notificationAttempts, remoteCount int
	if err := database.QueryRowContext(ctx, `select status,notification_attempt_count from connector_object_upload_jobs where id=$1::uuid`, job.ID).Scan(&status, &notificationAttempts); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from connector_remote_resources
		where project_id=$1 and connector_instance_id=$2 and resource_kind='wayline' and status='active'`, projectID, adapterID).Scan(&remoteCount); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" || notificationAttempts != 1 || remoteCount != 1 || !reflect.DeepEqual(calls, []string{"source", "sts", "upload", "notify", "list"}) {
		t.Fatalf("status=%s attempts=%d resources=%d calls=%v", status, notificationAttempts, remoteCount, calls)
	}
	var persisted string
	if err := database.QueryRowContext(ctx, `select row_to_json(job)::text || coalesce((select string_agg(payload_json::text,' ') from outbox_events where project_id=$1),'')
		from connector_object_upload_jobs job where job.id=$2::uuid`, projectID, job.ID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"STS_ACCESS_SECRET", "STS_KEY_SECRET", "STS_TOKEN_SECRET", "organization/project/file/wayline.kmz", "TOKEN_SECRET"} {
		if strings.Contains(persisted, secret) {
			t.Fatalf("database leaked %q", secret)
		}
	}
	if _, err := database.ExecContext(ctx, `update project_feature_flags set flighthub_action_flags_json='{}'::jsonb where project_id=$1`, projectID); err != nil {
		t.Fatal(err)
	}
	disabledRequest := request
	disabledRequest.IdempotencyKey += "-action-disabled"
	if _, err := store.Create(ctx, disabledRequest); !IsSafeCode(err, "scope_forbidden") {
		t.Fatalf("disabled action error=%v", err)
	}
	if _, err := database.ExecContext(ctx, `update project_feature_flags
		set flighthub_action_flags_json='{"security.temporary-credential":true,"flight.execute":true}'::jsonb where project_id=$1`, projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `update device_adapters set status='disabled' where id=$1 and project_id=$2`, adapterID, projectID); err != nil {
		t.Fatal(err)
	}
	disabledRequest.IdempotencyKey += "-connector"
	if _, err := store.Create(ctx, disabledRequest); !IsSafeCode(err, "scope_forbidden") {
		t.Fatalf("disabled connector error=%v", err)
	}
}

func newTestClient(t *testing.T, origin string, retries int) *Client {
	t.Helper()
	client, err := NewChinaClient(Config{MaxRetries: retries, HTTPClient: http.DefaultClient, RequestID: func() string { return "REQUEST_REDACTED" }})
	if err != nil {
		t.Fatal(err)
	}
	parsed := *client.baseURL
	testURL := strings.TrimSuffix(origin, "/")
	parsed.Scheme = strings.Split(strings.TrimPrefix(testURL, "http://"), "://")[0]
	if strings.HasPrefix(testURL, "https://") {
		parsed.Scheme = "https"
	} else {
		parsed.Scheme = "http"
	}
	parsed.Host = strings.TrimPrefix(strings.TrimPrefix(testURL, "http://"), "https://")
	client.baseURL = &parsed
	return client
}

var _ WaylineUploadStore = (*memoryWaylineUploadStore)(nil)
