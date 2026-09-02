package flighthub

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"

	"aerosight/worker/internal/connector"
)

type commandStatusClientFixture struct {
	snapshot CommandStatusOutput
	calls    int
}

func (fixture *commandStatusClientFixture) ListOrganizationCommandStatus(context.Context, string, string, string, []string, []string) (CommandStatusOutput, error) {
	fixture.calls++
	return fixture.snapshot, nil
}

type commandStatusTokenResolverFixture struct{}

func (commandStatusTokenResolverFixture) ResolveToken(context.Context, connector.Instance) (string, error) {
	return "TOKEN_REDACTED", nil
}

type commandStatusMemoryStore struct {
	commands []PendingStatusCommand
	status   string
}

func (store *commandStatusMemoryStore) load(context.Context) ([]PendingStatusCommand, error) {
	return append([]PendingStatusCommand(nil), store.commands...), nil
}

func (store *commandStatusMemoryStore) apply(_ context.Context, _ []PendingStatusCommand, decisions []CommandStatusDecision, _ time.Time) (int, error) {
	for _, decision := range decisions {
		for index := range store.commands {
			if store.commands[index].ID != decision.CommandID {
				continue
			}
			store.commands[index].RemoteBusinessID = decision.RemoteBusinessID
			store.commands[index].RemoteUpdatedAt = decision.RemoteUpdatedAt
			store.status = decision.Outcome
			if decision.Outcome != "pending" {
				store.commands = nil
			}
			return 1, nil
		}
	}
	return 0, nil
}

func memoryStatusReconciler(store *commandStatusMemoryStore, client CommandStatusClient, now time.Time) *ControlCommandStatusReconciler {
	return &ControlCommandStatusReconciler{
		client: client, resolver: commandStatusTokenResolverFixture{}, now: func() time.Time { return now },
		load: store.load, apply: store.apply,
	}
}

func statusService(businessID string, createdAt, updatedAt int64, percent, step, deviceCode int) ControlServiceProgress {
	service := ControlServiceProgress{BusinessID: businessID, CreateTime: createdAt, UpdateTime: updatedAt, DeviceCode: deviceCode}
	service.Progress.Percent = percent
	service.Progress.CurrentStep = step
	service.Extension = []byte(`{}`)
	return service
}

func snapshotWithMethods(deviceSN string, services map[string]ControlServiceProgress) CommandStatusOutput {
	return CommandStatusOutput{List: []DeviceCommandStatus{{SN: deviceSN, Services: services}}}
}

func TestListOrganizationCommandStatusUsesOrganizationPathAndProjectScope(t *testing.T) {
	t.Parallel()
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.Path != "/openapi/v2.0/organizations/ORG_REDACTED/manage-devices/cmds" {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		if request.URL.Query().Get("device_sn") != "AIRCRAFT_REDACTED" || request.URL.Query().Get("identifiers") != "BUSINESS_REDACTED" {
			t.Fatalf("unexpected query %s", request.URL.RawQuery)
		}
		if request.Header.Get("X-Project-Uuid") != "PROJECT_REDACTED" {
			t.Fatalf("organization status query lost project scope")
		}
		if request.Body != nil {
			t.Fatal("status query unexpectedly sent a body")
		}
		return response(http.StatusOK, []byte(`{"code":0,"message":"","data":{"list":[{"sn":"AIRCRAFT_REDACTED","services":{"return_home":{"bid":"BUSINESS_REDACTED","create_time":1788339600000,"update_time":1788339601000,"progress":{"percent":100,"current_step":2},"device_code":0,"ext":{}}}}]}}`), nil), nil
	}), nil)
	output, err := client.ListOrganizationCommandStatus(
		context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", "ORG_REDACTED",
		[]string{"AIRCRAFT_REDACTED"}, []string{"BUSINESS_REDACTED"},
	)
	if err != nil || len(output.List) != 1 || output.List[0].Services["return_home"].Progress.Percent != 100 {
		t.Fatalf("unexpected command status output=%#v err=%v", output, err)
	}
}

func TestReconcileCommandStatusDoesNotCompleteBeforeTerminalProgress(t *testing.T) {
	t.Parallel()
	sentAt := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	createdAt := sentAt.Add(time.Second).UnixMilli()
	command := PendingStatusCommand{ID: "COMMAND_LOCAL", CommandKey: "return_home", DeviceSN: "AIRCRAFT_REDACTED", SentAt: sentAt}
	pending := snapshotWithMethods("AIRCRAFT_REDACTED", map[string]ControlServiceProgress{
		"return_home": statusService("BUSINESS_REDACTED", createdAt, createdAt+1000, 99, 1, 0),
	})
	decisions := ReconcileCommandStatus([]PendingStatusCommand{command}, pending)
	if len(decisions) != 1 || decisions[0].Outcome != "pending" || decisions[0].RemoteBusinessID != "BUSINESS_REDACTED" {
		t.Fatalf("non-terminal progress was not safely correlated: %#v", decisions)
	}

	restarted := command
	restarted.RemoteBusinessID = decisions[0].RemoteBusinessID
	restarted.RemoteUpdatedAt = decisions[0].RemoteUpdatedAt
	terminal := snapshotWithMethods("AIRCRAFT_REDACTED", map[string]ControlServiceProgress{
		"return_home": statusService("BUSINESS_REDACTED", createdAt, createdAt+2000, 100, 2, 0),
	})
	decisions = ReconcileCommandStatus([]PendingStatusCommand{restarted}, terminal)
	if len(decisions) != 1 || decisions[0].Outcome != "acknowledged" {
		t.Fatalf("restarted reconciler did not resume persisted correlation: %#v", decisions)
	}
}

func TestCommandStatusPollingResumesAfterWorkerRestartWithoutEarlySuccess(t *testing.T) {
	t.Parallel()
	sentAt := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	createdAt := sentAt.Add(time.Second).UnixMilli()
	store := &commandStatusMemoryStore{commands: []PendingStatusCommand{{
		ID: "COMMAND_LOCAL", CommandKey: "return_home", DeviceSN: "AIRCRAFT_REDACTED", SentAt: sentAt,
		AdapterID: 7, ProjectUUID: "PROJECT_REDACTED", OrganizationUUID: "ORG_REDACTED",
		Instance: connector.Instance{ID: 7, ProjectID: 3},
	}}}
	pendingClient := &commandStatusClientFixture{snapshot: snapshotWithMethods("AIRCRAFT_REDACTED", map[string]ControlServiceProgress{
		"return_home": statusService("BUSINESS_REDACTED", createdAt, createdAt+1000, 75, 1, 0),
	})}
	firstWorker := memoryStatusReconciler(store, pendingClient, sentAt.Add(2*time.Second))
	if applied, err := firstWorker.PollOnce(context.Background()); err != nil || applied != 1 || store.status != "pending" || store.commands[0].RemoteBusinessID == "" {
		t.Fatalf("first worker lost pending correlation applied=%d status=%s commands=%#v err=%v", applied, store.status, store.commands, err)
	}

	terminalClient := &commandStatusClientFixture{snapshot: snapshotWithMethods("AIRCRAFT_REDACTED", map[string]ControlServiceProgress{
		"return_home": statusService("BUSINESS_REDACTED", createdAt, createdAt+2000, 100, 2, 0),
	})}
	restartedWorker := memoryStatusReconciler(store, terminalClient, sentAt.Add(3*time.Second))
	if applied, err := restartedWorker.PollOnce(context.Background()); err != nil || applied != 1 || store.status != "acknowledged" || len(store.commands) != 0 {
		t.Fatalf("restarted worker did not finish persisted command applied=%d status=%s commands=%#v err=%v", applied, store.status, store.commands, err)
	}
	if pendingClient.calls != 1 || terminalClient.calls != 1 {
		t.Fatalf("restart polling calls pending=%d terminal=%d", pendingClient.calls, terminalClient.calls)
	}
}

func TestReconcileCommandStatusHandlesNackDuplicatesAmbiguityAndOutOfOrder(t *testing.T) {
	t.Parallel()
	sentAt := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	createdAt := sentAt.Add(time.Second).UnixMilli()
	command := PendingStatusCommand{ID: "COMMAND_LOCAL", CommandKey: "return_home", DeviceSN: "AIRCRAFT_REDACTED", SentAt: sentAt}
	nack := snapshotWithMethods("AIRCRAFT_REDACTED", map[string]ControlServiceProgress{
		"return_home": statusService("BUSINESS_REDACTED", createdAt, createdAt+1000, 100, 2, 42),
	})
	if decisions := ReconcileCommandStatus([]PendingStatusCommand{command}, nack); len(decisions) != 1 || decisions[0].Outcome != "nacked" {
		t.Fatalf("terminal device failure was not a NACK: %#v", decisions)
	}

	duplicate := CommandStatusOutput{List: []DeviceCommandStatus{
		{SN: "AIRCRAFT_REDACTED", Services: map[string]ControlServiceProgress{"return_home": statusService("BUSINESS_REDACTED", createdAt, createdAt+1000, 100, 2, 0)}},
		{SN: "OTHER_REDACTED", Services: map[string]ControlServiceProgress{"return_home": statusService("BUSINESS_REDACTED", createdAt, createdAt+1000, 100, 2, 0)}},
	}}
	if decisions := ReconcileCommandStatus([]PendingStatusCommand{command}, duplicate); len(decisions) != 0 {
		t.Fatalf("duplicate remote business ID was correlated: %#v", decisions)
	}

	second := command
	second.ID = "COMMAND_LOCAL_2"
	if decisions := ReconcileCommandStatus([]PendingStatusCommand{command, second}, nack); len(decisions) != 0 {
		t.Fatalf("ambiguous repeated commands were correlated: %#v", decisions)
	}

	bound := command
	bound.RemoteBusinessID = "BUSINESS_REDACTED"
	bound.RemoteUpdatedAt = createdAt + 2000
	if decisions := ReconcileCommandStatus([]PendingStatusCommand{bound}, nack); len(decisions) != 0 {
		t.Fatalf("out-of-order remote result was applied: %#v", decisions)
	}

	unmatched := snapshotWithMethods("OTHER_REDACTED", map[string]ControlServiceProgress{
		"return_home": statusService("OTHER_BUSINESS", createdAt, createdAt+3000, 100, 2, 0),
	})
	if decisions := ReconcileCommandStatus([]PendingStatusCommand{bound}, unmatched); len(decisions) != 0 {
		t.Fatalf("unmatched device result was applied: %#v", decisions)
	}

	expired := command
	expired.Deadline = sentAt.Add(1500 * time.Millisecond)
	if decisions := ReconcileCommandStatus([]PendingStatusCommand{expired}, nack); len(decisions) != 0 {
		t.Fatalf("result updated after the local deadline was applied: %#v", decisions)
	}
}

func TestCommandStatusBatchingIsDeterministicAndBounded(t *testing.T) {
	t.Parallel()
	commands := make([]PendingStatusCommand, 0, 51)
	for index := 0; index < 51; index++ {
		commands = append(commands, PendingStatusCommand{
			ID: "COMMAND_" + time.Unix(int64(index), 0).Format("150405"), AdapterID: 7,
			DeviceSN: "DEVICE_" + time.Unix(int64(index), 0).Format("150405"),
		})
	}
	groups := groupPendingStatusCommands(commands)
	if len(groups) != 2 || len(groups[0]) != 50 || len(groups[1]) != 1 {
		t.Fatalf("status batching exceeded endpoint limits: %#v", groups)
	}
	serials, identifiers := commandStatusQuery([]PendingStatusCommand{
		{DeviceSN: "DEVICE_B", RemoteBusinessID: "BUSINESS_B"},
		{DeviceSN: "DEVICE_A", RemoteBusinessID: "BUSINESS_A"},
		{DeviceSN: "DEVICE_A", RemoteBusinessID: "BUSINESS_A"},
	})
	if !reflect.DeepEqual(serials, []string{"DEVICE_A", "DEVICE_B"}) || !reflect.DeepEqual(identifiers, []string{"BUSINESS_A", "BUSINESS_B"}) {
		t.Fatalf("query identifiers are unstable serials=%#v identifiers=%#v", serials, identifiers)
	}
}
