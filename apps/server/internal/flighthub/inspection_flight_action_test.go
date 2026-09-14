package flighthub

import (
	"aerosight/server/internal/credentials"
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

type inspectionActionClientFixture struct {
	*flightActionClientFixture
	wayline WaylineDetail
	readErr error
}

func (c *inspectionActionClientFixture) GetWayline(_ context.Context, _, project, route string) (WaylineDetail, error) {
	*c.calls = append(*c.calls, "wayline")
	if project != "11111111-1111-4111-8111-111111111111" || route != "WAYLINE_REDACTED" {
		return WaylineDetail{}, &APIError{SafeCode: "scope_forbidden"}
	}
	return c.wayline, c.readErr
}
func inspectionActionRequest(t *testing.T, job *FlightActionJob, mutate func(*FlightActionRequest)) {
	t.Helper()
	version, _ := FreezeInspectionWayline(WaylineSummary{ID: job.WaylineRemoteID, UpdatedAt: 1000, SizeBytes: 200})
	request := FlightActionRequest{Name: "巡检任务", TimeZone: "Asia/Shanghai", TaskType: "immediate", Inspection: &InspectionFlightActionContract{SchedulerOwner: "aerosight", WaylineVersion: version}}
	if mutate != nil {
		mutate(&request)
	}
	envelope, err := credentials.EncryptJSON(request, flightActionTestSecret, credentials.AAD("flighthub-flight-action", job.ID, job.ProjectID))
	if err != nil {
		t.Fatal(err)
	}
	job.RequestEnvelope, _ = json.Marshal(envelope)
}
func TestInspectionFlightActionChecksVersionBeforeAttempt(t *testing.T) {
	for _, tc := range []struct {
		name, status, want string
		mutate             func(*FlightActionRequest)
		changeRemote       bool
	}{
		{name: "queued changed", status: "queued", want: "inspection_wayline_version_changed", changeRemote: true},
		{name: "restart prepared changed", status: "prepared", want: "inspection_wayline_version_changed", changeRemote: true},
		{name: "forged", status: "queued", want: "inspection_wayline_version_invalid", mutate: func(r *FlightActionRequest) { r.Inspection.WaylineVersion.RemoteVersion = "forged" }},
		{name: "recurring", status: "queued", want: "inspection_schedule_invalid", mutate: func(r *FlightActionRequest) { r.TaskType = "recurring" }},
		{name: "hidden second scheduler", status: "prepared", want: "inspection_schedule_invalid", mutate: func(r *FlightActionRequest) { r.RecurringTaskStartTimes = []int64{1000} }},
		{name: "other owner", status: "queued", want: "inspection_schedule_invalid", mutate: func(r *FlightActionRequest) { r.Inspection.SchedulerOwner = "flighthub" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			job := flightActionFixtureJob(t, "flight-task-create")
			job.Status = tc.status
			inspectionActionRequest(t, &job, tc.mutate)
			store := &memoryFlightActionStore{job: job}
			calls := []string{}
			client := &inspectionActionClientFixture{flightActionClientFixture: &flightActionClientFixture{calls: &calls}, wayline: WaylineDetail{WaylineSummary: WaylineSummary{ID: job.WaylineRemoteID, UpdatedAt: 1000, SizeBytes: 200}}}
			if tc.changeRemote {
				client.wayline.UpdatedAt++
			}
			handler, _ := NewFlightActionHandler(store, client, waylineTokenResolverFixture{}, flightActionTestSecret)
			if err := handler.Handler(context.Background(), nil, flightActionEvent(job)); err != nil {
				t.Fatal(err)
			}
			if store.job.Status != "failed" || store.job.LastErrorCode != tc.want || store.job.AttemptCount != 0 {
				t.Fatalf("job=%+v calls=%v", store.job, calls)
			}
			for _, call := range calls {
				if call == "create" {
					t.Fatal("unsafe create")
				}
			}
		})
	}
}
func TestInspectionFlightActionReadRetryAndUnknownWrite(t *testing.T) {
	job := flightActionFixtureJob(t, "flight-task-create")
	inspectionActionRequest(t, &job, nil)
	store := &memoryFlightActionStore{job: job}
	calls := []string{}
	client := &inspectionActionClientFixture{flightActionClientFixture: &flightActionClientFixture{calls: &calls, createError: &APIError{SafeCode: "request_timeout", Retryable: true}}, readErr: &APIError{SafeCode: "request_timeout", Retryable: true}, wayline: WaylineDetail{WaylineSummary: WaylineSummary{ID: job.WaylineRemoteID, UpdatedAt: 1000, SizeBytes: 200}}}
	handler, _ := NewFlightActionHandler(store, client, waylineTokenResolverFixture{}, flightActionTestSecret)
	event := flightActionEvent(job)
	if err := handler.Handler(context.Background(), nil, event); !IsSafeCode(err, "request_timeout") {
		t.Fatal(err)
	}
	if store.job.AttemptCount != 0 || store.job.Status != "prepared" {
		t.Fatal("read failure consumed attempt")
	}
	client.readErr = nil
	if err := handler.Handler(context.Background(), nil, event); !IsSafeCode(err, "request_timeout") {
		t.Fatal(err)
	}
	client.wayline.UpdatedAt++
	client.listed = []FlightTaskSummary{{UUID: "CHILD", Name: reconciledTaskName("巡检任务", job.RequestDigest), TaskType: "immediate", Status: "waiting", SN: job.DeviceExternalID, WaylineUUID: job.WaylineRemoteID}}
	if err := handler.Handler(context.Background(), nil, event); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"dispatch-check", "wayline", "dispatch-check", "wayline", "create", "list"}) || store.job.AttemptCount != 1 || store.job.Status != "succeeded" {
		t.Fatalf("calls=%v job=%+v", calls, store.job)
	}
}

func TestInspectionPreparedFlightRechecksDispatchAndGovernance(t *testing.T) {
	for _, revoked := range []bool{false, true} {
		t.Run(map[bool]string{false: "new dispatch warning", true: "revoked approval"}[revoked], func(t *testing.T) {
			job := flightActionFixtureJob(t, "flight-task-create")
			job.Status = "prepared"
			inspectionActionRequest(t, &job, nil)
			if revoked {
				job.ApprovalValid = false
			}
			store := &memoryFlightActionStore{job: job}
			calls := []string{}
			client := &inspectionActionClientFixture{flightActionClientFixture: &flightActionClientFixture{calls: &calls, dispatchWarnings: []FlightTaskDispatchWarning{{Code: "device_busy", Type: "warning"}}}}
			handler, _ := NewFlightActionHandler(store, client, waylineTokenResolverFixture{}, flightActionTestSecret)
			if err := handler.Handler(context.Background(), nil, flightActionEvent(job)); err != nil {
				t.Fatal(err)
			}
			expected := []string{"dispatch-check"}
			code := "dispatch_check_warning"
			if revoked {
				expected = []string{}
				code = "governance_revoked"
			}
			if !reflect.DeepEqual(calls, expected) || store.job.LastErrorCode != code || store.job.AttemptCount != 0 || store.job.Status != "failed" {
				t.Fatalf("calls=%v job=%+v", calls, store.job)
			}
		})
	}
}
