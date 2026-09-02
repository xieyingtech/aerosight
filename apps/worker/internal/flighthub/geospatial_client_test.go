package flighthub

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

type geospatialContractFixture struct {
	ContractVersion string                   `json:"contractVersion"`
	Cases           []geospatialContractCase `json:"cases"`
}

type geospatialContractCase struct {
	Name          string          `json:"name"`
	EndpointID    string          `json:"endpointId"`
	Method        string          `json:"method"`
	Path          string          `json:"path"`
	ProjectHeader bool            `json:"projectHeader"`
	RequestBody   json.RawMessage `json:"requestBody"`
	ResponseBody  json.RawMessage `json:"responseBody"`
}

func loadGeospatialFixture(t *testing.T) (map[string]geospatialContractCase, []byte) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve geospatial fixture path")
	}
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), "../../../../contracts/dji-flighthub/v2/fixtures/geospatial_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture geospatialContractFixture
	if json.Unmarshal(contents, &fixture) != nil || fixture.ContractVersion != ContractVersion || len(fixture.Cases) != 10 {
		t.Fatalf("invalid geospatial fixture metadata: version=%q cases=%d", fixture.ContractVersion, len(fixture.Cases))
	}
	byName := make(map[string]geospatialContractCase, len(fixture.Cases))
	for _, item := range fixture.Cases {
		if item.Name == "" || item.EndpointID == "" || item.Method == "" || item.Path == "" || len(item.ResponseBody) == 0 {
			t.Fatalf("incomplete geospatial fixture: %#v", item)
		}
		if _, duplicate := byName[item.Name]; duplicate {
			t.Fatalf("duplicate geospatial fixture name %s", item.Name)
		}
		byName[item.Name] = item
	}
	return byName, contents
}

func geospatialFixtureClient(t *testing.T, item geospatialContractCase) *Client {
	t.Helper()
	return testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != item.Method || request.URL.RequestURI() != item.Path {
			t.Fatalf("request = %s %s, want %s %s", request.Method, request.URL.RequestURI(), item.Method, item.Path)
		}
		if request.Header.Get("X-User-Token") != "TOKEN_REDACTED" || request.Header.Get("X-Request-Id") != "request-redacted" || request.Header.Get("X-Language") != "zh" {
			t.Fatalf("missing required geospatial headers: %#v", request.Header)
		}
		projectHeader := request.Header.Get("X-Project-Uuid")
		if item.ProjectHeader != (projectHeader == "PROJECT_REDACTED") {
			t.Fatalf("project header=%q expected=%t", projectHeader, item.ProjectHeader)
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
			if len(body) != 0 || request.Header.Get("Content-Type") != "" {
				t.Fatalf("unexpected geospatial request body/header: %s %#v", body, request.Header)
			}
		} else if !equalJSON(body, item.RequestBody) || request.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("request body=%s want=%s header=%#v", body, item.RequestBody, request.Header)
		}
		return response(http.StatusOK, item.ResponseBody, nil), nil
	}), func(config *Config) {
		config.Now = func() time.Time { return time.Unix(1779440000, 0).UTC() }
		config.AllowedLinkHosts = []string{"objects.vendor.example"}
	})
}

func TestGeospatialTypedClientsMatchReleasedEndpointFixtures(t *testing.T) {
	cases, _ := loadGeospatialFixture(t)
	ctx := context.Background()

	var create MapElementCreateRequest
	if json.Unmarshal(cases["map-element-create-point"].RequestBody, &create) != nil {
		t.Fatal("invalid create fixture")
	}
	created, err := geospatialFixtureClient(t, cases["map-element-create-point"]).CreateMapElement(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", create)
	if err != nil || created.ID != "ELEMENT_REDACTED_01" {
		t.Fatalf("created=%#v err=%v", created, err)
	}

	active, err := geospatialFixtureClient(t, cases["workspace-air-sense-active"]).ListWorkspaceAirSenseWarnings(ctx, "TOKEN_REDACTED", "WORKSPACE_REDACTED")
	if err != nil || len(active) != 1 || active[0].Expired || !active[0].Enabled || len(active[0].Events) != 1 || active[0].Events[0].WarningLevel != 1 {
		t.Fatalf("active AirSense=%#v err=%v", active, err)
	}
	if active[0].CapturedAt.UnixMilli() != active[0].Timestamp || !active[0].ExpiresAt.Equal(active[0].CapturedAt.Add(airSenseWarningTTL)) {
		t.Fatalf("active AirSense freshness=%#v", active[0])
	}
	expired, err := geospatialFixtureClient(t, cases["workspace-air-sense-expired"]).ListWorkspaceAirSenseWarnings(ctx, "TOKEN_REDACTED", "WORKSPACE_REDACTED")
	if err != nil || len(expired) != 1 || !expired[0].Expired {
		t.Fatalf("expired AirSense=%#v err=%v", expired, err)
	}

	workspaceAreas, err := geospatialFixtureClient(t, cases["workspace-flight-areas-empty"]).ListWorkspaceFlightAreas(ctx, "TOKEN_REDACTED", "WORKSPACE_REDACTED", FlightAreaListOptions{Type: "nfz", Status: "enable"})
	if err != nil || workspaceAreas.List == nil || len(workspaceAreas.List) != 0 || workspaceAreas.Pagination.Total != 0 {
		t.Fatalf("workspace areas=%#v err=%v", workspaceAreas, err)
	}

	offlineDownload, err := geospatialFixtureClient(t, cases["workspace-offline-map-url"]).GetWorkspaceOfflineMapDownload(ctx, "TOKEN_REDACTED", "WORKSPACE_REDACTED")
	if err != nil || !offlineDownload.Enabled || offlineDownload.File == nil || offlineDownload.File.Size != 12720 ||
		!offlineDownload.File.ExpiresAt.Equal(time.Unix(1779443600, 0).UTC()) {
		t.Fatalf("offline download=%#v err=%v", offlineDownload, err)
	}
	offlineMap, err := geospatialFixtureClient(t, cases["workspace-offline-map-detail"]).GetWorkspaceOfflineMap(ctx, "TOKEN_REDACTED", "WORKSPACE_REDACTED")
	if err != nil || offlineMap.Disabled || offlineMap.OfflineMap == nil || offlineMap.OfflineMap.Status != "finish" || len(offlineMap.OfflineMap.Models) != 1 {
		t.Fatalf("offline map=%#v err=%v", offlineMap, err)
	}

	areaFile, err := geospatialFixtureClient(t, cases["project-flight-area-url"]).GetProjectFlightAreaFile(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED")
	if err != nil || areaFile.Name != "flight-areas-redacted.json" || !areaFile.ExpiresAt.Equal(time.Unix(1779443600, 0).UTC()) {
		t.Fatalf("flight area file=%#v err=%v", areaFile, err)
	}
	projectAreas, err := geospatialFixtureClient(t, cases["project-flight-areas-page"]).ListProjectFlightAreas(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", FlightAreaListOptions{
		PageOptions: PageOptions{Page: 2, PageSize: 10}, Name: "synthetic", Type: "dfence", Status: "disable",
	})
	if err != nil || projectAreas.Pagination.Total != 11 || len(projectAreas.List) != 1 || projectAreas.List[0].Content.Geometry.Type != "Polygon" {
		t.Fatalf("project areas=%#v err=%v", projectAreas, err)
	}

	var update MapElementUpdateRequest
	if json.Unmarshal(cases["workspace-map-element-update"].RequestBody, &update) != nil {
		t.Fatal("invalid update fixture")
	}
	updated, err := geospatialFixtureClient(t, cases["workspace-map-element-update"]).UpdateWorkspaceMapElement(ctx, "TOKEN_REDACTED", "WORKSPACE_REDACTED", "ELEMENT_REDACTED_01", update)
	if err != nil || updated.ID != "ELEMENT_REDACTED_01" {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	deleted, err := geospatialFixtureClient(t, cases["workspace-map-element-delete"]).DeleteWorkspaceMapElement(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "WORKSPACE_REDACTED", "ELEMENT_REDACTED_01")
	if err != nil || len(deleted.AffectedTriStates) != 1 || deleted.AffectedTriStates[0].TriState != "half" {
		t.Fatalf("deleted=%#v err=%v", deleted, err)
	}
}

func TestGeospatialFixtureCoversReleasedManifestAndContainsOnlySyntheticSpatialData(t *testing.T) {
	cases, contents := loadGeospatialFixture(t)
	wantEndpointIDs := map[string]struct{}{
		"454273488e0": {}, "456403938e0": {}, "456403940e0": {}, "456403943e0": {}, "456403944e0": {},
		"457494968e0": {}, "457494969e0": {}, "458354139e0": {}, "458354140e0": {},
	}
	for _, item := range cases {
		delete(wantEndpointIDs, item.EndpointID)
	}
	if len(wantEndpointIDs) != 0 {
		t.Fatalf("geospatial fixture is missing released endpoint IDs: %#v", wantEndpointIDs)
	}
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.`),
		regexp.MustCompile(`\b(?:7CT|1581F)[A-Z0-9]{8,}\b`),
		regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`),
	} {
		if pattern.Match(contents) {
			t.Fatalf("geospatial fixture contains forbidden identity or secret pattern %s", pattern)
		}
	}
	var document any
	if json.Unmarshal(contents, &document) != nil {
		t.Fatal("geospatial fixture is not valid JSON")
	}
	allowedCoordinates := map[float64]bool{0.125: true, 0.25: true, 15: true}
	var scan func(any, string)
	scan = func(value any, key string) {
		switch typed := value.(type) {
		case map[string]any:
			for childKey, child := range typed {
				scan(child, childKey)
			}
		case []any:
			for _, child := range typed {
				scan(child, key)
			}
		case float64:
			if (key == "coordinates" || key == "latitude" || key == "longitude") && !allowedCoordinates[typed] {
				t.Fatalf("geospatial fixture contains non-synthetic coordinate %v", typed)
			}
		case string:
			if key == "url" {
				parsed, err := url.Parse(typed)
				if err != nil || parsed.Scheme != "https" || parsed.Hostname() != "objects.vendor.example" ||
					!strings.Contains(parsed.Query().Get("Signature"), "REDACTED") {
					t.Fatalf("geospatial fixture URL is not safely synthetic: %s", typed)
				}
			}
		}
	}
	scan(document, "")
	if !strings.Contains(string(contents), `"enable_waring"`) || !strings.Contains(string(contents), `"waring_events"`) {
		t.Fatal("fixture must preserve the released AirSense field spellings")
	}
}

func TestGeospatialClientsFailClosedOnMalformedGeometryLinksPaginationAndWarnings(t *testing.T) {
	called := false
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return response(http.StatusOK, []byte(`{"code":0,"message":"","data":{}}`), nil), nil
	}), nil)
	invalidGeometry := MapElementCreateRequest{Name: "invalid", Resource: MapElementResource{Type: 0, Content: GeoJSONFeature{
		Type: "Feature", Properties: json.RawMessage(`{}`), Geometry: GeoJSONGeometry{Type: "Point", Coordinates: json.RawMessage(`[]`)},
	}}}
	if _, err := client.CreateMapElement(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", invalidGeometry); !IsSafeCode(err, "request_invalid") || called {
		t.Fatalf("invalid geometry err=%v called=%v", err, called)
	}

	tests := []struct {
		name string
		body string
		call func(*Client) error
		code string
	}{
		{name: "forbidden file host", body: `{"code":0,"message":"","data":{"name":"map.zip","url":"https://attacker.example/map.zip?Expires=1779443600","checksum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size":1}}`, code: "temporary_link_host_forbidden",
			call: func(c *Client) error {
				_, err := c.GetProjectFlightAreaFile(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED")
				return err
			}},
		{name: "expired file", body: `{"code":0,"message":"","data":{"name":"map.zip","url":"https://objects.vendor.example/map.zip?Expires=1779430000","checksum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size":1}}`, code: "temporary_link_expired",
			call: func(c *Client) error {
				_, err := c.GetProjectFlightAreaFile(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED")
				return err
			}},
		{name: "pagination mismatch", body: `{"code":0,"message":"","data":{"list":null,"pagination":{"page":2,"page_size":20,"total":0}}}`, code: "schema_incompatible",
			call: func(c *Client) error {
				_, err := c.ListProjectFlightAreas(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", FlightAreaListOptions{})
				return err
			}},
		{name: "invalid warning coordinate", body: `{"code":0,"message":"","data":[{"sn":"DEVICE_REDACTED","timestamp":1779439990000,"enable_waring":true,"waring_events":[{"icao":"ICAO_REDACTED","warning_level":1,"latitude":91,"longitude":0,"altitude":1,"altitude_type":0,"heading":0,"relative_altitude":0,"vert_trend":0,"distance":1}]}]}`, code: "schema_incompatible",
			call: func(c *Client) error {
				_, err := c.ListWorkspaceAirSenseWarnings(context.Background(), "TOKEN_REDACTED", "WORKSPACE_REDACTED")
				return err
			}},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			fixtureClient := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
				return response(http.StatusOK, []byte(item.body), nil), nil
			}), func(config *Config) {
				config.Now = func() time.Time { return time.Unix(1779440000, 0).UTC() }
				config.AllowedLinkHosts = []string{"objects.vendor.example"}
			})
			if err := item.call(fixtureClient); !IsSafeCode(err, item.code) {
				t.Fatalf("error=%v want=%s", err, item.code)
			}
		})
	}
}
