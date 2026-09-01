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
	"time"
)

type liveContractFixture struct {
	ContractVersion string             `json:"contractVersion"`
	Cases           []liveContractCase `json:"cases"`
}

type liveContractCase struct {
	Name          string          `json:"name"`
	EndpointID    string          `json:"endpointId"`
	Method        string          `json:"method"`
	Path          string          `json:"path"`
	ProjectHeader bool            `json:"projectHeader"`
	RequestBody   json.RawMessage `json:"requestBody"`
	ResponseBody  json.RawMessage `json:"responseBody"`
}

func loadLiveFixture(t *testing.T) (map[string]liveContractCase, []byte) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve live fixture path")
	}
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), "../../../../contracts/dji-flighthub/v2/fixtures/live_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture liveContractFixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.ContractVersion == "" || len(fixture.Cases) != 14 {
		t.Fatalf("invalid live fixture metadata: version=%q cases=%d", fixture.ContractVersion, len(fixture.Cases))
	}
	byName := make(map[string]liveContractCase, len(fixture.Cases))
	for _, item := range fixture.Cases {
		if _, duplicate := byName[item.Name]; duplicate {
			t.Fatalf("duplicate live fixture %q", item.Name)
		}
		byName[item.Name] = item
	}
	return byName, contents
}

func liveFixtureClient(t *testing.T, item liveContractCase) *Client {
	t.Helper()
	return testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != item.Method || request.URL.RequestURI() != item.Path {
			t.Fatalf("request = %s %s, want %s %s", request.Method, request.URL.RequestURI(), item.Method, item.Path)
		}
		if request.Header.Get("X-User-Token") != "TOKEN_REDACTED" || request.Header.Get("X-Request-Id") != "request-redacted" || request.Header.Get("X-Language") != "zh" {
			t.Fatalf("missing required live headers: %#v", request.Header)
		}
		if got := request.Header.Get("X-Project-Uuid"); item.ProjectHeader != (got == "PROJECT_REDACTED") || (!item.ProjectHeader && got != "") {
			t.Fatalf("project header=%q expected=%t", got, item.ProjectHeader)
		}
		var actualBody []byte
		if request.Body != nil {
			var err error
			actualBody, err = io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
		}
		if item.RequestBody == nil {
			if len(actualBody) != 0 || request.Header.Get("Content-Type") != "" {
				t.Fatalf("unexpected live request body/header: %q %#v", actualBody, request.Header)
			}
		} else {
			var actual, expected any
			if json.Unmarshal(actualBody, &actual) != nil || json.Unmarshal(item.RequestBody, &expected) != nil || !reflect.DeepEqual(actual, expected) {
				t.Fatalf("request body=%s want=%s", actualBody, item.RequestBody)
			}
			if request.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("missing JSON content type: %#v", request.Header)
			}
		}
		return response(http.StatusOK, item.ResponseBody, nil), nil
	}), func(config *Config) {
		config.Now = func() time.Time { return time.Unix(1779440000, 0).UTC() }
	})
}

func TestLiveClientsCoverReleasedEndpointFixtures(t *testing.T) {
	cases, _ := loadLiveFixture(t)
	ctx := context.Background()

	for _, item := range []struct {
		name     string
		provider string
	}{
		{name: "live-start-volc", provider: "volc"},
		{name: "live-start-agora", provider: "agora"},
		{name: "live-start-srs", provider: "srs"},
	} {
		fixture := cases[item.name]
		var input LiveStreamStartRequest
		if err := json.Unmarshal(fixture.RequestBody, &input); err != nil {
			t.Fatal(err)
		}
		result, err := liveFixtureClient(t, fixture).StartLiveStream(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", input)
		if err != nil || result.URLType != item.provider || result.URL == "" || !result.ExpiresAt.Equal(time.Unix(1779443600, 0).UTC()) {
			t.Fatalf("%s result=%#v err=%v", item.name, result, err)
		}
	}

	quality := cases["stream-quality"]
	var qualityInput StreamQualityRequest
	if err := json.Unmarshal(quality.RequestBody, &qualityInput); err != nil {
		t.Fatal(err)
	}
	if err := liveFixtureClient(t, quality).SetStreamQuality(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", qualityInput); err != nil {
		t.Fatal(err)
	}

	organizationRecordings, err := liveFixtureClient(t, cases["organization-recordings-empty"]).ListOrganizationRecordingTasks(ctx, "TOKEN_REDACTED", "ORG_REDACTED_01", "AIRCRAFT_REDACTED_01")
	if err != nil || organizationRecordings == nil || len(organizationRecordings) != 0 {
		t.Fatalf("organization recordings=%#v err=%v", organizationRecordings, err)
	}
	projectRecordings, err := liveFixtureClient(t, cases["project-recordings-empty"]).ListProjectRecordingTasks(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "AIRCRAFT_REDACTED_01")
	if err != nil || projectRecordings == nil || len(projectRecordings) != 0 {
		t.Fatalf("project recordings=%#v err=%v", projectRecordings, err)
	}
	autoRecord, err := liveFixtureClient(t, cases["auto-record-config"]).GetAutoRecordingConfig(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED")
	if err != nil || autoRecord.ID != 9 || len(autoRecord.Items) != 1 {
		t.Fatalf("auto record=%#v err=%v", autoRecord, err)
	}

	shares, err := liveFixtureClient(t, cases["live-shares-empty-code"]).ListLiveShares(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", LiveShareListOptions{})
	if err != nil || shares == nil || len(shares) != 0 {
		t.Fatalf("business-empty shares=%#v err=%v", shares, err)
	}
	shares, err = liveFixtureClient(t, cases["live-shares-empty-list"]).ListLiveShares(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", LiveShareListOptions{PageOptions: PageOptions{Page: 2, PageSize: 10}, Status: 2})
	if err != nil || shares == nil || len(shares) != 0 {
		t.Fatalf("empty-list shares=%#v err=%v", shares, err)
	}
	share, err := liveFixtureClient(t, cases["live-share-detail-empty"]).GetLiveShare(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "AIRCRAFT_REDACTED_01")
	if err != nil || share != nil {
		t.Fatalf("share=%#v err=%v", share, err)
	}

	enabled := true
	converters, err := liveFixtureClient(t, cases["stream-converters"]).ListStreamConverters(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", StreamConverterListOptions{
		PageOptions: PageOptions{Page: 1, PageSize: 20}, DeviceSN: "AIRCRAFT_REDACTED_01", CameraIndex: "165-0-7", Schema: "rtsp", Enabled: &enabled,
	})
	if err != nil || converters.Pagination.Total != 1 || len(converters.List) != 1 || converters.List[0].BypassOption == nil || converters.List[0].BypassOption.Password != "PASSWORD_REDACTED" {
		t.Fatalf("converters=%#v err=%v", converters, err)
	}

	create := cases["stream-converter-create-rtsp"]
	var createInput StreamConverterCreateRequest
	if err := json.Unmarshal(create.RequestBody, &createInput); err != nil {
		t.Fatal(err)
	}
	created, err := liveFixtureClient(t, create).CreateStreamConverter(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", createInput)
	if err != nil || created.ID != "CONVERTER_REDACTED_02" {
		t.Fatalf("created=%#v err=%v", created, err)
	}
	if err := liveFixtureClient(t, cases["stream-converter-enable"]).SetStreamConverterEnabled(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "CONVERTER_REDACTED_01", true); err != nil {
		t.Fatal(err)
	}
	if err := liveFixtureClient(t, cases["stream-converter-delete"]).DeleteStreamConverter(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "CONVERTER_REDACTED_01"); err != nil {
		t.Fatal(err)
	}
}

func TestLiveClientFixtureCoversAllReleasedLiveEndpointsAndIsRedacted(t *testing.T) {
	cases, contents := loadLiveFixture(t)
	wantEndpointIDs := map[string]struct{}{
		"454273423e0": {}, "456444816e0": {}, "456444818e0": {}, "456809558e0": {},
		"457494960e0": {}, "457494963e0": {}, "457494964e0": {}, "457494965e0": {},
		"458069500e0": {}, "458069503e0": {}, "480437503e0": {},
	}
	for _, item := range cases {
		delete(wantEndpointIDs, item.EndpointID)
	}
	if len(wantEndpointIDs) != 0 {
		t.Fatalf("live fixture is missing released endpoint IDs: %#v", wantEndpointIDs)
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.`),
		regexp.MustCompile(`\b(?:7CT|1581F)[A-Z0-9]{8,}\b`),
		regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`),
		regexp.MustCompile(`(?i)https?://[^"[:space:]]+[?&](token|signature|auth_key|x-amz-credential)=`),
		regexp.MustCompile(`(?i)"(?:latitude|longitude)"\s*:`),
	}
	for _, pattern := range patterns {
		if pattern.Match(contents) {
			t.Fatalf("live fixture contains forbidden secret, identity, or coordinate pattern %s", pattern)
		}
	}
	var document any
	if err := json.Unmarshal(contents, &document); err != nil {
		t.Fatal(err)
	}
	var scan func(any)
	scan = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, item := range typed {
				if stringValue, ok := item.(string); ok && regexp.MustCompile(`(?i)(password|username|credential|rtsp_url|^url$|^sn$)`).MatchString(key) && stringValue != "" && !strings.Contains(stringValue, "REDACTED") {
					t.Fatalf("live fixture field %q is not visibly redacted", key)
				}
				scan(item)
			}
		case []any:
			for _, item := range typed {
				scan(item)
			}
		}
	}
	scan(document)
}

func TestLiveClientsFailClosedForInvalidInputsExpiredCredentialsAndMalformedSchemas(t *testing.T) {
	called := false
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, nil
	}), nil)
	if _, err := client.StartLiveStream(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", LiveStreamStartRequest{
		SN: "AIRCRAFT_REDACTED", CameraIndex: "165-0-7", VideoExpire: 3600, QualityType: "unknown",
	}); !IsSafeCode(err, "request_invalid") {
		t.Fatalf("invalid quality error=%v", err)
	}
	if _, err := client.ListStreamConverters(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", StreamConverterListOptions{Schema: "unknown"}); !IsSafeCode(err, "request_invalid") {
		t.Fatalf("invalid converter schema error=%v", err)
	}
	if _, err := client.CreateStreamConverter(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", StreamConverterCreateRequest{
		Name: "converter", SN: "AIRCRAFT_REDACTED", CameraIndex: "165-0-7", Schema: "rtsp",
	}); !IsSafeCode(err, "request_invalid") {
		t.Fatalf("invalid converter option error=%v", err)
	}
	if called {
		t.Fatal("invalid live request reached upstream")
	}

	now := time.Unix(1779440000, 0).UTC()
	expiredClient := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, []byte(`{"code":0,"message":"","data":{"expire_ts":1779440000,"url":"PLAYBACK_REDACTED","url_type":"volc"}}`), nil), nil
	}), func(config *Config) { config.Now = func() time.Time { return now } })
	_, err := expiredClient.StartLiveStream(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", LiveStreamStartRequest{
		SN: "AIRCRAFT_REDACTED", CameraIndex: "165-0-7", VideoExpire: 3600, QualityType: LiveQualityAdaptive,
	})
	if !IsSafeCode(err, "temporary_link_expired") {
		t.Fatalf("expired credential error=%v", err)
	}

	malformed := []string{
		`{"code":0,"message":"","data":{"pagination":{"page":0,"page_size":20,"total":1},"list":[]}}`,
		`{"code":0,"message":"","data":{"pagination":{"page":1,"page_size":20,"total":1},"list":[{"converter_id":"ID_REDACTED","converter_name":"name","sn":"SN_REDACTED","camera":"camera","video":"normal-0","schema":"unknown","state":"running","bypass_option":{}}]}}`,
	}
	for _, body := range malformed {
		malformedClient := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
			return response(http.StatusOK, []byte(body), nil), nil
		}), nil)
		if _, err := malformedClient.ListStreamConverters(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", StreamConverterListOptions{}); !IsSafeCode(err, "schema_incompatible") {
			t.Fatalf("malformed converter error=%v body=%s", err, body)
		}
	}

	nullShares := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, []byte(`{"code":0,"message":"","data":null}`), nil), nil
	}), nil)
	if _, err := nullShares.ListLiveShares(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", LiveShareListOptions{}); !IsSafeCode(err, "schema_incompatible") {
		t.Fatalf("null live shares error=%v", err)
	}

	emptyDetail := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, []byte(`{"code":231011,"message":"NO_RESOURCE_REDACTED","data":{}}`), nil), nil
	}), nil)
	share, err := emptyDetail.GetLiveShare(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", "AIRCRAFT_REDACTED")
	if err != nil || share != nil {
		t.Fatalf("business-empty detail=%#v err=%v", share, err)
	}
}

func TestLiveWriteClientsNeverAutomaticallyRetry(t *testing.T) {
	enableTS := false
	writes := []struct {
		name string
		call func(*Client) error
	}{
		{name: "start", call: func(client *Client) error {
			_, err := client.StartLiveStream(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", LiveStreamStartRequest{SN: "AIRCRAFT_REDACTED", CameraIndex: "165-0-7", VideoExpire: 3600, QualityType: LiveQualityAdaptive})
			return err
		}},
		{name: "quality", call: func(client *Client) error {
			return client.SetStreamQuality(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", StreamQualityRequest{SN: "AIRCRAFT_REDACTED", CameraIndex: "165-0-7", QualityType: LiveQualityAdaptive})
		}},
		{name: "create converter", call: func(client *Client) error {
			_, err := client.CreateStreamConverter(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", StreamConverterCreateRequest{
				Name: "converter", SN: "AIRCRAFT_REDACTED", CameraIndex: "165-0-7", Schema: "rtsp",
				SchemaOption: StreamConverterSchemaOption{Username: "USERNAME_REDACTED", Password: "PASSWORD_REDACTED", EnableTS: &enableTS},
			})
			return err
		}},
		{name: "enable converter", call: func(client *Client) error {
			return client.SetStreamConverterEnabled(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", "CONVERTER_REDACTED", true)
		}},
		{name: "delete converter", call: func(client *Client) error {
			return client.DeleteStreamConverter(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", "CONVERTER_REDACTED")
		}},
	}
	for _, item := range writes {
		t.Run(item.name, func(t *testing.T) {
			calls := 0
			client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return response(http.StatusServiceUnavailable, []byte(`{"code":200500,"message":"REDACTED","data":{}}`), nil), nil
			}), func(config *Config) { config.MaxRetries = 3 })
			if err := item.call(client); !IsSafeCode(err, "upstream_unavailable") {
				t.Fatalf("write error=%v", err)
			}
			if calls != 1 {
				t.Fatalf("write calls=%d want=1", calls)
			}
		})
	}
}

func TestLiveFixtureJSONIsCanonical(t *testing.T) {
	_, contents := loadLiveFixture(t)
	var compact bytes.Buffer
	if err := json.Compact(&compact, contents); err != nil || compact.Len() == 0 {
		t.Fatalf("invalid live fixture JSON: %v", err)
	}
}
