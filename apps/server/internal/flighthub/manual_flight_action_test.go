package flighthub

import (
	"aerosight/server/internal/credentials"
	"context"
	"encoding/json"
	"testing"
)

type manualFlightClient struct {
	*flightActionClientFixture
	points  int
	model   string
	online  bool
	request FlightTaskCreateRequest
}

func (c *manualFlightClient) GetWayline(context.Context, string, string, string) (WaylineDetail, error) {
	return WaylineDetail{WaylineSummary: WaylineSummary{DeviceModelKey: c.model}, WaypointCount: c.points}, nil
}
func (c *manualFlightClient) ListDevices(context.Context, string, string) ([]Topology, error) {
	return []Topology{{Gateway: &Device{SN: "DOCK_REDACTED", Online: c.online, ModeCode: 0}, Drone: &Device{Model: DeviceModel{Key: "0-91-1"}}}}, nil
}
func (c *manualFlightClient) CreateFlightTask(ctx context.Context, token, project string, r FlightTaskCreateRequest) (FlightTaskCreateResult, error) {
	c.request = r
	return c.flightActionClientFixture.CreateFlightTask(ctx, token, project, r)
}
func TestManualFlightChecksAndSingleDispatch(t *testing.T) {
	for _, test := range []struct {
		name   string
		points int
		model  string
		online bool
		fail   string
	}{{"valid", 2, "0-91-1", true, ""}, {"one-waypoint", 1, "0-91-1", true, "wayline_requires_two_waypoints"}, {"wrong-model", 2, "other", true, "flight_device_not_ready_or_model_mismatch"}, {"offline", 2, "0-91-1", false, "flight_device_not_ready_or_model_mismatch"}} {
		t.Run(test.name, func(t *testing.T) {
			job := flightActionFixtureJob(t, "flight-task-create")
			request := FlightActionRequest{Name: "测试", TimeZone: "Asia/Shanghai", TaskType: "immediate", ManualDeviceFlight: true, WaylinePrecisionType: "gps", RTHAltitude: 50, RTHMode: "preset"}
			envelope, err := credentials.EncryptJSON(request, flightActionTestSecret, credentials.AAD("flighthub-flight-action", job.ID, job.ProjectID))
			if err != nil {
				t.Fatal(err)
			}
			job.RequestEnvelope, _ = json.Marshal(envelope)
			store := &memoryFlightActionStore{job: job}
			calls := []string{}
			client := &manualFlightClient{flightActionClientFixture: &flightActionClientFixture{calls: &calls, createRemoteID: "remote-task", remoteTasks: map[string]FlightTask{"remote-task": {UUID: "remote-task", Status: "waiting"}}}, points: test.points, model: test.model, online: test.online}
			handler, _ := NewFlightActionHandler(store, client, waylineTokenResolverFixture{}, flightActionTestSecret)
			_ = handler.Handler(context.Background(), nil, flightActionEvent(job))
			if test.fail != "" {
				if store.job.LastErrorCode != test.fail || store.job.AttemptCount != 0 {
					t.Fatal(store.job.LastErrorCode, store.job.AttemptCount)
				}
				return
			}
			_ = handler.Handler(context.Background(), nil, flightActionEvent(job))
			creates := 0
			for _, call := range calls {
				if call == "create" {
					creates++
				}
			}
			if creates != 1 || client.request.WaylinePrecisionType != "gps" || client.request.RTHAltitude != 50 || store.completedTask.Status != "waiting" {
				t.Fatal("incorrect dispatch", calls, client.request, store.completedTask)
			}
		})
	}
}
