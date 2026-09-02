package flighthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/credentials"
	"aerosight/worker/internal/outbox"
	"github.com/google/uuid"
)

const flightActionTestSecret = "0123456789abcdef0123456789abcdef"

type memoryFlightActionStore struct {
	job           FlightActionJob
	completedTask FlightTask
}

func (store *memoryFlightActionStore) Load(_ context.Context, projectID int, jobID string) (FlightActionJob, error) {
	if store.job.ProjectID != projectID || store.job.ID != jobID {
		return FlightActionJob{}, &APIError{SafeCode: "scope_forbidden"}
	}
	return store.job, nil
}
func (store *memoryFlightActionStore) ExternalDeviceID(_ context.Context, projectID int, connectorID int64, deviceID int) (string, error) {
	if projectID != store.job.ProjectID || connectorID != store.job.ConnectorInstanceID || deviceID <= 0 {
		return "", &APIError{SafeCode: "scope_forbidden"}
	}
	return "LANDING_REDACTED", nil
}
func (store *memoryFlightActionStore) MarkPrepared(_ context.Context, _ FlightActionJob, _ map[string]any) error {
	store.job.Status = "prepared"
	return nil
}
func (store *memoryFlightActionStore) BeginAttempt(_ context.Context, _ FlightActionJob) error {
	if store.job.AttemptCount != 0 {
		return errors.New("duplicate unsafe attempt")
	}
	store.job.AttemptCount = 1
	store.job.Status = "reconciling"
	return nil
}
func (store *memoryFlightActionStore) RecordAccepted(_ context.Context, _ FlightActionJob, remoteID string) error {
	if remoteID != "" {
		store.job.RemoteResultResourceID = sql.NullInt64{Int64: 1, Valid: true}
		store.job.RemoteResultID = remoteID
	}
	return nil
}
func (store *memoryFlightActionStore) RecordError(_ context.Context, _ FlightActionJob, code string) error {
	store.job.LastErrorCode = code
	return nil
}
func (store *memoryFlightActionStore) RecordReconciliationRead(_ context.Context, _ FlightActionJob) (int, error) {
	store.job.ReconciliationCount++
	return store.job.ReconciliationCount, nil
}
func (store *memoryFlightActionStore) Complete(_ context.Context, _ FlightActionJob, task FlightTask) error {
	store.job.Status = "succeeded"
	store.completedTask = task
	return nil
}
func (store *memoryFlightActionStore) Fail(_ context.Context, _ FlightActionJob, code string) error {
	store.job.Status = "failed"
	store.job.LastErrorCode = code
	return nil
}
func (store *memoryFlightActionStore) Block(_ context.Context, _ FlightActionJob, code string) error {
	store.job.Status = "blocked"
	store.job.LastErrorCode = code
	return nil
}

type flightActionClientFixture struct {
	calls            *[]string
	createError      error
	statusError      error
	resumptionErr    error
	listed           []FlightTaskSummary
	remoteTasks      map[string]FlightTask
	createRemoteID   string
	dispatchWarnings []FlightTaskDispatchWarning
}

func (client *flightActionClientFixture) CheckFlightTaskDispatch(context.Context, string, string, string, string) (FlightTaskDispatchCheck, error) {
	*client.calls = append(*client.calls, "dispatch-check")
	return FlightTaskDispatchCheck{Warnings: append([]FlightTaskDispatchWarning(nil), client.dispatchWarnings...), DevicePosition: &FlightTaskDispatchPosition{}}, nil
}
func (client *flightActionClientFixture) CreateFlightTask(_ context.Context, _ string, _ string, request FlightTaskCreateRequest) (FlightTaskCreateResult, error) {
	*client.calls = append(*client.calls, "create")
	if !strings.Contains(request.Name, "AeroSight-") {
		return FlightTaskCreateResult{}, errors.New("missing reconciliation marker")
	}
	if client.createError != nil {
		return FlightTaskCreateResult{}, client.createError
	}
	return FlightTaskCreateResult{TaskUUID: client.createRemoteID}, nil
}
func (client *flightActionClientFixture) UpdateFlightTaskStatus(context.Context, string, string, string, string) error {
	*client.calls = append(*client.calls, "status")
	return client.statusError
}
func (client *flightActionClientFixture) CreateFlightTaskResumption(context.Context, string, string, string, string) (FlightTaskResumption, error) {
	*client.calls = append(*client.calls, "resumption")
	if client.resumptionErr != nil {
		return FlightTaskResumption{}, client.resumptionErr
	}
	return FlightTaskResumption{Task: ResumedFlightTask{UUID: client.createRemoteID, BeginAt: 1, EndAt: 2}}, nil
}
func (client *flightActionClientFixture) GetFlightTask(_ context.Context, _ string, _ string, taskID string) (FlightTask, error) {
	*client.calls = append(*client.calls, "get")
	if task, ok := client.remoteTasks[taskID]; ok {
		return task, nil
	}
	return FlightTask{}, &APIError{SafeCode: "scope_not_found"}
}
func (client *flightActionClientFixture) ListFlightTasks(context.Context, string, string, FlightTaskListOptions) ([]FlightTaskSummary, error) {
	*client.calls = append(*client.calls, "list")
	return append([]FlightTaskSummary(nil), client.listed...), nil
}
func (client *flightActionClientFixture) ListRecentFlightTasks(context.Context, string, string, []string) ([]FlightTaskSummary, error) {
	*client.calls = append(*client.calls, "recent")
	return append([]FlightTaskSummary(nil), client.listed...), nil
}

func flightActionFixtureJob(t *testing.T, kind string) FlightActionJob {
	t.Helper()
	id := "11111111-2222-4333-8444-555555555555"
	request := FlightActionRequest{Name: "巡检任务", TimeZone: "Asia/Shanghai", TaskType: "immediate", DesiredStatus: "suspended"}
	envelope, err := credentials.EncryptJSON(request, flightActionTestSecret, credentials.AAD("flighthub-flight-action", id, 41))
	if err != nil {
		t.Fatal(err)
	}
	envelopeJSON, _ := json.Marshal(envelope)
	scope, _ := json.Marshal(map[string]string{"projectUuid": "11111111-1111-4111-8111-111111111111", "projectName": "test"})
	job := FlightActionJob{
		ID: id, ProjectID: 41, TeamID: 42, ConnectorInstanceID: 43, TaskRunID: 44, DeviceID: 45,
		ApprovalRequestID: "11111111-1111-4111-8111-111111111112", RequestedByUserID: 46,
		ActionKind: kind, RequestDigest: strings.Repeat("a", 64), RequestEnvelope: envelopeJSON,
		Status: "queued", DeviceExternalID: "DOCK_REDACTED", ConnectorStatus: "connected", ActionEnabled: true,
		CapabilityVerified: true, TaskRunStatus: "ready", PreflightAllowed: true, ApprovalValid: true,
		Instance: connector.Instance{ID: 43, ProjectID: 41, ConnectorKey: ConnectorKey, Version: ConnectorVersion,
			DiscoveryScope: scope, CredentialEnvelope: json.RawMessage(`{"redacted":true}`)},
	}
	if kind == "flight-task-create" {
		job.WaylineResourceID = sql.NullInt64{Int64: 47, Valid: true}
		job.WaylineRemoteID = "WAYLINE_REDACTED"
		job.ApprovalAction = "flighthub.flight-task.create"
	} else {
		job.TargetResourceID = sql.NullInt64{Int64: 48, Valid: true}
		job.TargetRemoteID = "TASK_PARENT_REDACTED"
		job.ApprovalAction = "flighthub.flight-task.status"
		if kind == "flight-task-resumption" {
			job.ApprovalAction = "flighthub.flight-task.resume"
		}
	}
	return job
}

func flightActionEvent(job FlightActionJob) outbox.Event {
	payload, _ := json.Marshal(map[string]string{"jobId": job.ID})
	return outbox.Event{ProjectID: job.ProjectID, TeamID: job.TeamID, Payload: payload}
}

func TestFlightTaskCreateLostResponseReconcilesWithoutBlindSecondFlight(t *testing.T) {
	job := flightActionFixtureJob(t, "flight-task-create")
	store := &memoryFlightActionStore{job: job}
	calls := []string{}
	remoteName := reconciledTaskName("巡检任务", job.RequestDigest)
	client := &flightActionClientFixture{calls: &calls, createError: &APIError{SafeCode: "request_timeout", Retryable: true}}
	handler, err := NewFlightActionHandler(store, client, waylineTokenResolverFixture{}, flightActionTestSecret)
	if err != nil {
		t.Fatal(err)
	}
	event := flightActionEvent(job)
	if err := handler.Handler(context.Background(), nil, event); !IsSafeCode(err, "request_timeout") {
		t.Fatalf("lost response=%v", err)
	}
	client.listed = []FlightTaskSummary{{UUID: "TASK_CHILD_REDACTED", Name: remoteName, TaskType: "immediate", Status: "waiting",
		SN: job.DeviceExternalID, WaylineUUID: job.WaylineRemoteID}}
	if err := handler.Handler(context.Background(), nil, event); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"dispatch-check", "create", "list"}) || store.job.AttemptCount != 1 || store.job.Status != "succeeded" {
		t.Fatalf("calls=%v job=%#v", calls, store.job)
	}
	if store.completedTask.Status == "success" {
		t.Fatal("HTTP acceptance was misreported as physical flight success")
	}
}

func TestFlightTaskCreateAcceptedButFinallyUnknownBlocksWithoutRepeat(t *testing.T) {
	job := flightActionFixtureJob(t, "flight-task-create")
	store := &memoryFlightActionStore{job: job}
	calls := []string{}
	client := &flightActionClientFixture{calls: &calls, createError: &APIError{SafeCode: "request_timeout", Retryable: true}}
	handler, _ := NewFlightActionHandler(store, client, waylineTokenResolverFixture{}, flightActionTestSecret)
	event := flightActionEvent(job)
	_ = handler.Handler(context.Background(), nil, event)
	for index := 0; index < maxActionReconciliationReads; index++ {
		_ = handler.Handler(context.Background(), nil, event)
	}
	if store.job.Status != "blocked" || store.job.LastErrorCode != "remote_result_unknown" || store.job.AttemptCount != 1 {
		t.Fatalf("job=%#v", store.job)
	}
	if !reflect.DeepEqual(calls, []string{"dispatch-check", "create", "list", "list", "list"}) {
		t.Fatalf("blind create retry calls=%v", calls)
	}
}

func TestFlightTaskStatusUnknownNeverBlindlyRepeatsPut(t *testing.T) {
	job := flightActionFixtureJob(t, "flight-task-status")
	job.TaskRunStatus = "running"
	store := &memoryFlightActionStore{job: job}
	calls := []string{}
	client := &flightActionClientFixture{calls: &calls, statusError: &APIError{SafeCode: "request_timeout", Retryable: true}, remoteTasks: map[string]FlightTask{
		job.TargetRemoteID: {UUID: job.TargetRemoteID, Name: "redacted", TaskType: "immediate", Status: "executing", SN: job.DeviceExternalID,
			WaylineUUID: "WAYLINE_REDACTED", BeginAt: "", EndAt: ""},
	}}
	handler, _ := NewFlightActionHandler(store, client, waylineTokenResolverFixture{}, flightActionTestSecret)
	event := flightActionEvent(job)
	_ = handler.Handler(context.Background(), nil, event)
	for index := 0; index < maxActionReconciliationReads; index++ {
		_ = handler.Handler(context.Background(), nil, event)
	}
	if store.job.Status != "blocked" || store.job.AttemptCount != 1 {
		t.Fatalf("job=%#v", store.job)
	}
	if !reflect.DeepEqual(calls, []string{"status", "get", "get", "get"}) {
		t.Fatalf("blind status retry calls=%v", calls)
	}
}

func TestFlightTaskResumptionUnknownNeverBlindlyRepeatsPost(t *testing.T) {
	job := flightActionFixtureJob(t, "flight-task-resumption")
	job.TaskRunStatus = "paused"
	store := &memoryFlightActionStore{job: job}
	calls := []string{}
	client := &flightActionClientFixture{calls: &calls, resumptionErr: &APIError{SafeCode: "request_timeout", Retryable: true},
		remoteTasks: map[string]FlightTask{job.TargetRemoteID: {
			UUID: job.TargetRemoteID, Name: "parent", TaskType: "immediate", Status: "paused", SN: job.DeviceExternalID,
			WaylineUUID: "WAYLINE_REDACTED",
		}}}
	handler, _ := NewFlightActionHandler(store, client, waylineTokenResolverFixture{}, flightActionTestSecret)
	event := flightActionEvent(job)
	_ = handler.Handler(context.Background(), nil, event)
	for index := 0; index < maxActionReconciliationReads; index++ {
		_ = handler.Handler(context.Background(), nil, event)
	}
	if store.job.Status != "blocked" || store.job.AttemptCount != 1 {
		t.Fatalf("job=%#v", store.job)
	}
	if !reflect.DeepEqual(calls, []string{"resumption", "get", "recent", "get", "recent", "get", "recent"}) {
		t.Fatalf("blind resumption retry calls=%v", calls)
	}
}

func TestFlightActionGovernanceRevocationMakesZeroUpstreamCalls(t *testing.T) {
	mutations := []func(*FlightActionJob){
		func(job *FlightActionJob) { job.ActionEnabled = false },
		func(job *FlightActionJob) { job.CapabilityVerified = false },
		func(job *FlightActionJob) { job.PreflightAllowed = false },
		func(job *FlightActionJob) { job.ApprovalValid = false },
		func(job *FlightActionJob) { job.ConnectorStatus = "disabled" },
	}
	for _, mutate := range mutations {
		job := flightActionFixtureJob(t, "flight-task-create")
		mutate(&job)
		store := &memoryFlightActionStore{job: job}
		calls := []string{}
		client := &flightActionClientFixture{calls: &calls}
		handler, _ := NewFlightActionHandler(store, client, waylineTokenResolverFixture{}, flightActionTestSecret)
		if err := handler.Handler(context.Background(), nil, flightActionEvent(job)); err != nil {
			t.Fatal(err)
		}
		if len(calls) != 0 || store.job.Status != "failed" {
			t.Fatalf("calls=%v job=%#v", calls, store.job)
		}
	}
}

func TestFlightTaskDispatchWarningStopsBeforeCreate(t *testing.T) {
	job := flightActionFixtureJob(t, "flight-task-create")
	store := &memoryFlightActionStore{job: job}
	calls := []string{}
	client := &flightActionClientFixture{calls: &calls, dispatchWarnings: []FlightTaskDispatchWarning{{Code: "model_mismatch", Type: "warning"}}}
	handler, _ := NewFlightActionHandler(store, client, waylineTokenResolverFixture{}, flightActionTestSecret)
	if err := handler.Handler(context.Background(), nil, flightActionEvent(job)); err != nil {
		t.Fatal(err)
	}
	if store.job.Status != "failed" || !reflect.DeepEqual(calls, []string{"dispatch-check"}) {
		t.Fatalf("job=%#v calls=%v", store.job, calls)
	}
}

func TestUnsafeFlightTaskWritesDisableTransportRetries(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		calls++
		response.WriteHeader(http.StatusServiceUnavailable)
		_, _ = response.Write([]byte(`{"code":200500,"message":"redacted","data":{}}`))
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, 3)
	_, _ = client.CreateFlightTask(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", FlightTaskCreateRequest{
		Name: "inspection", SN: "DOCK_REDACTED", WaylineUUID: "WAYLINE_REDACTED", TimeZone: "Asia/Shanghai", TaskType: "immediate",
	})
	if calls != 1 {
		t.Fatalf("create calls=%d", calls)
	}
	_ = client.UpdateFlightTaskStatus(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", "TASK_REDACTED", "suspended")
	if calls != 2 {
		t.Fatalf("status calls=%d", calls)
	}
	_, _ = client.CreateFlightTaskResumption(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", "WORKSPACE_REDACTED", "TASK_REDACTED")
	if calls != 3 {
		t.Fatalf("resumption calls=%d", calls)
	}
}

func TestSQLFlightActionJobRestartsReconcilesAndKeepsIntentEncrypted(t *testing.T) {
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
	var requesterID, approverID, teamID, projectID, deviceID, taskID, taskRunID int
	var adapterID, definitionID, policyID, waylineID int64
	if err := database.QueryRowContext(ctx, `insert into users(name,email) values($1,$2) returning id`, "flight requester", fmt.Sprintf("flight-requester-%d@example.test", suffix)).Scan(&requesterID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into users(name,email) values($1,$2) returning id`, "flight approver", fmt.Sprintf("flight-approver-%d@example.test", suffix)).Scan(&approverID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(), `delete from users where id in($1,$2)`, requesterID, approverID)
	})
	if err := database.QueryRowContext(ctx, `insert into teams(name) values($1) returning id`, fmt.Sprintf("flight-action-%d", suffix)).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.ExecContext(context.Background(), `delete from teams where id=$1`, teamID) })
	if _, err := database.ExecContext(ctx, `insert into team_members(team_id,user_id,role) values($1,$2,'owner'),($1,$3,'admin')`, teamID, requesterID, approverID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, teamID, fmt.Sprintf("flight-action-%d", suffix)).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `insert into project_feature_flags(project_id,flighthub_action_flags_json)
		values($1,'{"flight.execute":true}'::jsonb) on conflict(project_id) do update set flighthub_action_flags_json=excluded.flighthub_action_flags_json`, projectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select id from connector_definitions where connector_key=$1 and version=$2`, ConnectorKey, ConnectorVersion).Scan(&definitionID); err != nil {
		t.Fatal(err)
	}
	accountFingerprint := strings.Repeat("a", 64)
	scope := fmt.Sprintf(`{"projectUuid":"11111111-1111-4111-8111-111111111111","projectName":"redacted","accountFingerprint":%q}`, accountFingerprint)
	if err := database.QueryRowContext(ctx, `insert into device_adapters(
		project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status,discovery_scope_json,credential_envelope_json
	) values($1,$2,$3,'dji-flighthub2',$4,'2','connected',$5,'{}'::jsonb) returning id`,
		projectID, teamID, fmt.Sprintf("flight-action-%d", suffix), definitionID, scope).Scan(&adapterID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into connector_capability_snapshots(
		project_id,team_id,connector_instance_id,capability_code,status,evidence_level,region,deployment,
		account_fingerprint,device_model,firmware_version,details_json,verified_at,expires_at
	) values($1,$2,$3,'flight.execute','supported','field-write','cn','cn-public-cloud',$4,'dock-model','01.00','{}'::jsonb,now(),now()+interval '1 hour') returning id`,
		projectID, teamID, adapterID, accountFingerprint).Scan(new(int64)); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into devices(project_id,adapter_id,device_type_id,name,type,status,device_model,firmware_version)
		select $1,$2,id,'flight action dock','dock','online','dock-model','01.00' from device_types
		 where type_key='dji.dock2' and status='active' order by version desc limit 1 returning id`, projectID, adapterID).Scan(&deviceID); err != nil {
		t.Fatal(err)
	}
	serial := fmt.Sprintf("DOCK_TEST_%d", suffix)
	if _, err := database.ExecContext(ctx, `insert into device_external_identities(
		project_id,team_id,adapter_id,device_id,external_device_id,external_device_type,identity_json,discovery_status,bound_at
	) values($1,$2,$3,$4,$5,'dji.dock2',jsonb_build_object('attributes',jsonb_build_object('serialNumber',$6::text)),'managed',now())`,
		projectID, teamID, adapterID, deviceID, secureRemoteKey(serial), serial); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into safety_policy_versions(
		project_id,team_id,version,status,max_altitude_meters,max_speed_meters_per_second,minimum_battery_percent,published_at
	) values($1,$2,1,'published',120,15,50,now()) returning id`, projectID, teamID).Scan(&policyID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into tasks(project_id,team_id,name,trigger_type,script,created_by_user_id)
		values($1,$2,'flight action task','manual','{}',$3) returning id`, projectID, teamID, requesterID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into task_runs(
		project_id,team_id,task_id,selected_device_id,safety_policy_version_id,trigger_source,status,preflight_snapshot_json,created_by_user_id
	) values($1,$2,$3,$4,$5,'manual','ready','{"allowed":true}'::jsonb,$6) returning id`,
		projectID, teamID, taskID, deviceID, policyID, requesterID).Scan(&taskRunID); err != nil {
		t.Fatal(err)
	}
	approvalID := uuid.NewString()
	if _, err := database.ExecContext(ctx, `insert into approval_requests(
		id,project_id,team_id,resource_type,resource_id,action,requested_by_user_id,status,expires_at,context_json,decided_at
	) values($1,$2,$3,'task_run',$4,'flighthub.flight-task.create',$5,'approved',now()+interval '1 hour','{"preflight":{"allowed":true}}'::jsonb,now())`,
		approvalID, projectID, teamID, fmt.Sprint(taskRunID), requesterID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `update task_runs set approval_request_id=$2 where id=$1 and project_id=$3`, taskRunID, approvalID, projectID); err != nil {
		t.Fatal(err)
	}
	waylineRemoteID := fmt.Sprintf("WAYLINE_TEST_%d", suffix)
	if err := database.QueryRowContext(ctx, `insert into connector_remote_resources(
		project_id,team_id,connector_instance_id,resource_kind,remote_id,status,summary_json
	) values($1,$2,$3,'wayline',$4,'active','{}'::jsonb) returning id`, projectID, teamID, adapterID, waylineRemoteID).Scan(&waylineID); err != nil {
		t.Fatal(err)
	}
	jobID := uuid.NewString()
	request := FlightActionRequest{Name: "encrypted flight intent", TimeZone: "Asia/Shanghai", TaskType: "immediate"}
	envelope, err := credentials.EncryptJSON(request, flightActionTestSecret, credentials.AAD("flighthub-flight-action", jobID, projectID))
	if err != nil {
		t.Fatal(err)
	}
	requestDigest := strings.Repeat("b", 64)
	if _, err := database.ExecContext(ctx, `insert into connector_action_jobs(
		id,project_id,team_id,connector_instance_id,task_run_id,device_id,wayline_resource_id,approval_request_id,
		requested_by_user_id,action_kind,idempotency_key,request_digest,request_envelope_json
	) values($1,$2,$3,$4,$5,$6,$7,$8,$9,'flight-task-create',$10,$11,$12)`,
		jobID, projectID, teamID, adapterID, taskRunID, deviceID, waylineID, approvalID, requesterID,
		fmt.Sprintf("flight-action-%d", suffix), requestDigest, envelope); err != nil {
		t.Fatal(err)
	}
	duplicate, err := database.ExecContext(ctx, `insert into connector_action_jobs(
		id,project_id,team_id,connector_instance_id,task_run_id,device_id,wayline_resource_id,approval_request_id,
		requested_by_user_id,action_kind,idempotency_key,request_digest,request_envelope_json
	) values($1,$2,$3,$4,$5,$6,$7,$8,$9,'flight-task-create',$10,$11,$12)
	 on conflict(project_id,connector_instance_id,action_kind,idempotency_key) do nothing`,
		uuid.NewString(), projectID, teamID, adapterID, taskRunID, deviceID, waylineID, approvalID, requesterID,
		fmt.Sprintf("flight-action-%d", suffix), requestDigest, envelope)
	if err != nil {
		t.Fatal(err)
	}
	if count, _ := duplicate.RowsAffected(); count != 0 {
		t.Fatal("stable idempotency key created a duplicate connector action job")
	}

	store := NewSQLFlightActionStore(database)
	calls := []string{}
	client := &flightActionClientFixture{calls: &calls, createError: &APIError{SafeCode: "request_timeout", Retryable: true}}
	firstWorker, err := NewFlightActionHandler(store, client, waylineTokenResolverFixture{}, flightActionTestSecret)
	if err != nil {
		t.Fatal(err)
	}
	event := outbox.Event{ProjectID: projectID, TeamID: teamID, Payload: json.RawMessage(fmt.Sprintf(`{"jobId":%q}`, jobID))}
	if err := firstWorker.Handler(ctx, nil, event); !IsSafeCode(err, "request_timeout") {
		t.Fatalf("first worker=%v", err)
	}
	client.listed = []FlightTaskSummary{{UUID: fmt.Sprintf("TASK_TEST_%d", suffix),
		Name: reconciledTaskName(request.Name, requestDigest), TaskType: "immediate", Status: "waiting", SN: serial, WaylineUUID: waylineRemoteID}}
	secondWorker, err := NewFlightActionHandler(NewSQLFlightActionStore(database), client, waylineTokenResolverFixture{}, flightActionTestSecret)
	if err != nil {
		t.Fatal(err)
	}
	if err := secondWorker.Handler(ctx, nil, event); err != nil {
		t.Fatal(err)
	}
	var jobStatus, runStatus, persisted string
	var attempts int
	if err := database.QueryRowContext(ctx, `select status,attempt_count,row_to_json(job)::text from connector_action_jobs job where id=$1`, jobID).Scan(&jobStatus, &attempts, &persisted); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select status from task_runs where id=$1 and project_id=$2`, taskRunID, projectID).Scan(&runStatus); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "succeeded" || attempts != 1 || runStatus != "dispatching" || !reflect.DeepEqual(calls, []string{"dispatch-check", "create", "list"}) {
		t.Fatalf("job=%s attempts=%d run=%s calls=%v", jobStatus, attempts, runStatus, calls)
	}
	for _, secret := range []string{request.Name, serial, "TOKEN_SECRET"} {
		if strings.Contains(persisted, secret) {
			t.Fatalf("action job leaked %q", secret)
		}
	}
}

var _ FlightActionStore = (*memoryFlightActionStore)(nil)
