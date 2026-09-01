package flighthub

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

type flightContractFixture struct {
	ContractVersion string               `json:"contractVersion"`
	Cases           []flightContractCase `json:"cases"`
}

type flightContractCase struct {
	Name          string          `json:"name"`
	EndpointID    string          `json:"endpointId"`
	Method        string          `json:"method"`
	Path          string          `json:"path"`
	ProjectHeader bool            `json:"projectHeader"`
	RequestBody   json.RawMessage `json:"requestBody"`
	ResponseBody  json.RawMessage `json:"responseBody"`
}

func loadFlightFixture(t *testing.T) (map[string]flightContractCase, []byte) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve flight fixture path")
	}
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), "../../../../contracts/dji-flighthub/v2/fixtures/flight_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture flightContractFixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.ContractVersion != ContractVersion || len(fixture.Cases) != 17 {
		t.Fatalf("invalid flight fixture metadata: version=%q cases=%d", fixture.ContractVersion, len(fixture.Cases))
	}
	byName := make(map[string]flightContractCase, len(fixture.Cases))
	endpointIDs := make(map[string]struct{}, len(fixture.Cases))
	for _, item := range fixture.Cases {
		if item.Name == "" || item.EndpointID == "" || item.Method == "" || item.Path == "" || len(item.ResponseBody) == 0 {
			t.Fatalf("incomplete flight fixture: %#v", item)
		}
		if _, duplicate := byName[item.Name]; duplicate {
			t.Fatalf("duplicate flight fixture name %s", item.Name)
		}
		if _, duplicate := endpointIDs[item.EndpointID]; duplicate {
			t.Fatalf("duplicate flight fixture endpoint %s", item.EndpointID)
		}
		byName[item.Name] = item
		endpointIDs[item.EndpointID] = struct{}{}
	}
	return byName, contents
}

func equalJSON(left, right []byte) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
}

func flightFixtureClient(t *testing.T, item flightContractCase) *Client {
	t.Helper()
	return testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != item.Method || request.URL.RequestURI() != item.Path {
			t.Fatalf("request = %s %s, want %s %s", request.Method, request.URL.RequestURI(), item.Method, item.Path)
		}
		if request.Header.Get("X-User-Token") != "TOKEN_REDACTED" || request.Header.Get("X-Request-Id") != "request-redacted" || request.Header.Get("X-Language") != "zh" {
			t.Fatalf("missing required FlightHub headers: %#v", request.Header)
		}
		projectHeader := request.Header.Get("X-Project-Uuid")
		if (item.ProjectHeader && projectHeader != "PROJECT_REDACTED") || (!item.ProjectHeader && projectHeader != "") {
			t.Fatalf("project header=%q expected=%v", projectHeader, item.ProjectHeader)
		}
		var body []byte
		if request.Body != nil {
			var err error
			body, err = io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
		}
		if len(item.RequestBody) == 0 {
			if len(body) != 0 {
				t.Fatalf("unexpected request body %s", body)
			}
		} else if !equalJSON(body, item.RequestBody) {
			t.Fatalf("request body=%s want=%s", body, item.RequestBody)
		}
		return response(http.StatusOK, item.ResponseBody, nil), nil
	}), func(config *Config) {
		config.AllowedLinkHosts = []string{"es-flight-api-cn.djigate.com", "objects.vendor.example"}
	})
}

func TestFlightOperationTypedClientsMatchEveryReleasedEndpointFixture(t *testing.T) {
	cases, _ := loadFlightFixture(t)
	ctx := context.Background()

	waylines, err := flightFixtureClient(t, cases["wayline-list"]).ListWaylines(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED")
	if err != nil || len(waylines) != 1 || waylines[0].ID != "WAYLINE_REDACTED_01" || waylines[0].TemplateTypes[0] != "waypoint" {
		t.Fatalf("waylines=%#v err=%v", waylines, err)
	}
	wayline, err := flightFixtureClient(t, cases["wayline-detail"]).GetWayline(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "WAYLINE_REDACTED_01")
	if err != nil || wayline.WaypointCount != 5 || wayline.DownloadURL == "" {
		t.Fatalf("wayline=%#v err=%v", wayline, err)
	}
	uploaded, err := flightFixtureClient(t, cases["wayline-finish-upload"]).NotifyWaylineUploadComplete(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", WaylineUploadCompleteRequest{Name: "inspection", ObjectKey: "projects/PROJECT_REDACTED/waylines/upload.kmz"})
	if err != nil || uploaded.UUID != "WAYLINE_REDACTED_02" {
		t.Fatalf("uploaded=%#v err=%v", uploaded, err)
	}

	listed, err := flightFixtureClient(t, cases["flight-task-list"]).ListFlightTasks(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", FlightTaskListOptions{
		SNs: []string{"DOCK_REDACTED_01", "DOCK_REDACTED_02"}, Name: "inspection", BeginAt: 1770000000, EndAt: 1770003600,
		TaskType: "immediate", Statuses: []string{"executing", "success"}, FlightTaskType: "1",
	})
	if err != nil || len(listed) != 1 || listed[0].Status != "executing" || listed[0].CurrentWaypoint != 2 {
		t.Fatalf("listed=%#v err=%v", listed, err)
	}
	recent, err := flightFixtureClient(t, cases["flight-task-recent"]).ListRecentFlightTasks(ctx, "TOKEN_REDACTED", "WORKSPACE_REDACTED", []string{"DOCK_REDACTED_01", "DOCK_REDACTED_02"})
	if err != nil || len(recent) != 1 || recent[0].BelongToSN != "DOCK_REDACTED_01" {
		t.Fatalf("recent=%#v err=%v", recent, err)
	}
	batch, err := flightFixtureClient(t, cases["flight-task-batch"]).BatchGetFlightTasks(ctx, "TOKEN_REDACTED", "WORKSPACE_REDACTED", []string{"TASK_REDACTED_01", "TASK_REDACTED_02"})
	if err != nil || len(batch) != 1 || batch[0].TaskType != "timed" {
		t.Fatalf("batch=%#v err=%v", batch, err)
	}
	defaultName, err := flightFixtureClient(t, cases["flight-task-default-name"]).GetDefaultFlightTaskName(ctx, "TOKEN_REDACTED", "WORKSPACE_REDACTED", "inspection")
	if err != nil || defaultName.IndexName != "inspection-2" {
		t.Fatalf("defaultName=%#v err=%v", defaultName, err)
	}
	detail, err := flightFixtureClient(t, cases["flight-task-detail"]).GetFlightTaskDetail(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "TASK_REDACTED_01")
	if err != nil || len(detail.FlightPieces) != 1 || detail.FlightPieces[0].FlightPieceUUID != "PIECE_REDACTED_01" {
		t.Fatalf("detail=%#v err=%v", detail, err)
	}
	task, err := flightFixtureClient(t, cases["flight-task-single"]).GetFlightTask(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "TASK_REDACTED_01")
	if err != nil || task.Status != "success" || task.FolderInfo.UploadedFileCount != 6 {
		t.Fatalf("task=%#v err=%v", task, err)
	}
	dispatch, err := flightFixtureClient(t, cases["flight-task-dispatch-check"]).CheckFlightTaskDispatch(ctx, "TOKEN_REDACTED", "WORKSPACE_REDACTED", "DOCK_REDACTED_01", "WAYLINE_REDACTED_01")
	if err != nil || len(dispatch.Warnings) != 1 || dispatch.DevicePosition == nil {
		t.Fatalf("dispatch=%#v err=%v", dispatch, err)
	}
	if err := flightFixtureClient(t, cases["flight-task-status"]).UpdateFlightTaskStatus(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "TASK_REDACTED_01", "suspended"); err != nil {
		t.Fatalf("status update error=%v", err)
	}
	resumption, err := flightFixtureClient(t, cases["flight-task-resumption"]).CreateFlightTaskResumption(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "WORKSPACE_REDACTED", "TASK_REDACTED_01")
	if err != nil || resumption.Task.UUID != "TASK_REDACTED_02" || resumption.Task.ParentTask == nil || resumption.Task.ParentTask.UUID != "TASK_REDACTED_01" {
		t.Fatalf("resumption=%#v err=%v", resumption, err)
	}
	track, err := flightFixtureClient(t, cases["flight-task-track"]).GetFlightTaskTrack(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "TASK_REDACTED_01")
	if err != nil || track.Track.ID != "TRACK_REDACTED_01" || len(track.Track.Points) != 2 || track.Track.Points[1].Height != 33 {
		t.Fatalf("track=%#v err=%v", track, err)
	}
	timeline, err := flightFixtureClient(t, cases["flight-task-operation-timeline"]).GetFlightTaskOperationTimeline(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "TASK_REDACTED_01")
	if err != nil || len(timeline.ControlChanges) != 1 || len(timeline.OperationLogs) != 1 || timeline.OperationLogs[0].Method != "pause_task" {
		t.Fatalf("timeline=%#v err=%v", timeline, err)
	}
	media, err := flightFixtureClient(t, cases["flight-task-media"]).ListFlightTaskMedia(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "TASK_REDACTED_01")
	if err != nil || len(media) != 1 || media[0].UUID != "MEDIA_REDACTED_01" || media[0].FileType != "image" || media[0].SizeBytes != 4096 {
		t.Fatalf("media=%#v err=%v", media, err)
	}
	exports, err := flightFixtureClient(t, cases["flight-task-export-history"]).ListFlightTaskExports(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", FlightExportOptions{})
	if err != nil || exports.Pagination.Total != 1 || len(exports.List) != 1 || exports.List[0].Status != "export_complete" {
		t.Fatalf("exports=%#v err=%v", exports, err)
	}
	download, err := flightFixtureClient(t, cases["flight-record-download-url"]).GetFlightRecordDownloadURL(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "exports/PROJECT_REDACTED/RECORD_REDACTED_01.csv")
	if err != nil || download.URL == "" || download.ExpiresAt.IsZero() {
		t.Fatalf("download=%#v err=%v", download, err)
	}
}

func TestFlightOperationClientsRejectInvalidParametersBeforeNetwork(t *testing.T) {
	called := false
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, nil
	}), nil)
	ctx := context.Background()
	tests := []struct {
		name string
		call func() error
	}{
		{name: "missing project", call: func() error { _, err := client.ListWaylines(ctx, "TOKEN_REDACTED", ""); return err }},
		{name: "duplicate serial", call: func() error {
			_, err := client.ListFlightTasks(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", FlightTaskListOptions{SNs: []string{"DOCK_REDACTED", "DOCK_REDACTED"}})
			return err
		}},
		{name: "unpaired time", call: func() error {
			_, err := client.ListFlightTasks(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", FlightTaskListOptions{SNs: []string{"DOCK_REDACTED"}, BeginAt: 1})
			return err
		}},
		{name: "empty batch", call: func() error {
			_, err := client.BatchGetFlightTasks(ctx, "TOKEN_REDACTED", "WORKSPACE_REDACTED", nil)
			return err
		}},
		{name: "bad status", call: func() error {
			return client.UpdateFlightTaskStatus(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "TASK_REDACTED", "completed")
		}},
		{name: "unsafe object key", call: func() error {
			_, err := client.NotifyWaylineUploadComplete(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", WaylineUploadCompleteRequest{Name: "test", ObjectKey: "../secret"})
			return err
		}},
		{name: "missing track task", call: func() error {
			_, err := client.GetFlightTaskTrack(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "")
			return err
		}},
		{name: "unsafe operation task", call: func() error {
			_, err := client.GetFlightTaskOperationTimeline(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "../task")
			return err
		}},
		{name: "unsafe media task", call: func() error {
			_, err := client.ListFlightTaskMedia(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "../task")
			return err
		}},
		{name: "invalid export page", call: func() error {
			_, err := client.ListFlightTaskExports(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", FlightExportOptions{Page: -1})
			return err
		}},
		{name: "unsafe record key", call: func() error {
			_, err := client.GetFlightRecordDownloadURL(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "../secret")
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !IsSafeCode(err, "request_invalid") && !IsSafeCode(err, "scope_forbidden") {
				t.Fatalf("error=%v", err)
			}
		})
	}
	if called {
		t.Fatal("invalid flight operation request reached upstream")
	}
}

func TestFlightOperationFixturesAreSanitizedAndSchemasFailClosed(t *testing.T) {
	_, contents := loadFlightFixture(t)
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.`),
		regexp.MustCompile(`7CT[A-Z0-9]{8,}`),
		regexp.MustCompile(`1581F[A-Z0-9]{8,}`),
		regexp.MustCompile(`(?i)https?://[^"[:space:]]+[?&](token|signature|x-amz-credential)=`),
	} {
		if pattern.Match(contents) {
			t.Fatalf("flight fixture contains forbidden secret pattern %s", pattern)
		}
	}
	uuidPattern := regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`)
	if match := uuidPattern.Find(contents); match != nil {
		t.Fatalf("flight fixture contains a complete UUID: %s", match)
	}

	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, bytes.NewBufferString(`{"code":0,"message":"","data":{"list":[{"name":"missing id"}]}}`).Bytes(), nil), nil
	}), nil)
	_, err := client.ListWaylines(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED")
	if !IsSafeCode(err, "schema_incompatible") || strings.Contains(err.Error(), "missing id") {
		t.Fatalf("schema error=%v", err)
	}
}
