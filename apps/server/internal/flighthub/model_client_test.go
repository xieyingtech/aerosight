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

type modelContractFixture struct {
	ContractVersion string              `json:"contractVersion"`
	Cases           []modelContractCase `json:"cases"`
}

type modelContractCase struct {
	Name          string          `json:"name"`
	EndpointID    string          `json:"endpointId"`
	Method        string          `json:"method"`
	Path          string          `json:"path"`
	ProjectHeader bool            `json:"projectHeader"`
	RequestBody   json.RawMessage `json:"requestBody"`
	ResponseBody  json.RawMessage `json:"responseBody"`
}

func loadModelFixture(t *testing.T) (map[string]modelContractCase, []byte) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve model fixture path")
	}
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), "../../../../contracts/dji-flighthub/v2/fixtures/model_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture modelContractFixture
	if json.Unmarshal(contents, &fixture) != nil || fixture.ContractVersion != ContractVersion || len(fixture.Cases) != 14 {
		t.Fatalf("invalid model fixture metadata: version=%q cases=%d", fixture.ContractVersion, len(fixture.Cases))
	}
	byName := make(map[string]modelContractCase, len(fixture.Cases))
	for _, item := range fixture.Cases {
		if item.Name == "" || item.EndpointID == "" || item.Method == "" || item.Path == "" || len(item.ResponseBody) == 0 {
			t.Fatalf("incomplete model fixture: %#v", item)
		}
		if _, duplicate := byName[item.Name]; duplicate {
			t.Fatalf("duplicate model fixture name %s", item.Name)
		}
		byName[item.Name] = item
	}
	return byName, contents
}

func modelFixtureClient(t *testing.T, item modelContractCase) *Client {
	t.Helper()
	return testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != item.Method || request.URL.RequestURI() != item.Path {
			t.Fatalf("request = %s %s, want %s %s", request.Method, request.URL.RequestURI(), item.Method, item.Path)
		}
		if request.Header.Get("X-User-Token") != "TOKEN_REDACTED" || request.Header.Get("X-Request-Id") != "request-redacted" || request.Header.Get("X-Language") != "zh" {
			t.Fatalf("missing required model headers")
		}
		if item.ProjectHeader != (request.Header.Get("X-Project-Uuid") == "PROJECT_REDACTED") {
			t.Fatalf("project header presence=%t expected=%t", request.Header.Get("X-Project-Uuid") != "", item.ProjectHeader)
		}
		var body []byte
		if request.Body != nil {
			body, _ = io.ReadAll(request.Body)
		}
		if len(item.RequestBody) == 0 {
			if len(body) != 0 {
				t.Fatalf("unexpected model request body")
			}
		} else if !equalJSON(body, item.RequestBody) {
			t.Fatalf("model request body does not match fixture")
		}
		return response(http.StatusOK, item.ResponseBody, nil), nil
	}), func(config *Config) {
		config.Now = func() time.Time { return time.Unix(1779440000, 0).UTC() }
		config.AllowedLinkHosts = []string{"objects.vendor.example"}
	})
}

func TestModelTypedClientsMatchEveryReleasedEndpointFixture(t *testing.T) {
	cases, _ := loadModelFixture(t)
	ctx := context.Background()

	models, err := modelFixtureClient(t, cases["model-list-completed"]).ListModels(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED")
	if err != nil || len(models) != 2 || models[0].FileType != ModelFile2D || models[1].Size != 4096 {
		t.Fatalf("models=%#v err=%v", models, err)
	}
	detail, err := modelFixtureClient(t, cases["model-detail-completed"]).GetModel(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "9")
	if err != nil || detail.ID != 9 || detail.PreviewURL == "" || detail.FileType != ModelFile2D {
		t.Fatalf("detail=%#v err=%v", detail, err)
	}
	var reconstructionRequest ModelReconstructionRequest
	if json.Unmarshal(cases["model-reconstruction-create"].RequestBody, &reconstructionRequest) != nil {
		t.Fatal("invalid traditional reconstruction request fixture")
	}
	reconstruction, err := modelFixtureClient(t, cases["model-reconstruction-create"]).CreateModelReconstruction(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", reconstructionRequest)
	if err != nil || reconstruction.ID != 11 {
		t.Fatalf("reconstruction=%#v err=%v", reconstruction, err)
	}
	download, err := modelFixtureClient(t, cases["model-download-ready"]).GetModelDownloadURL(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "FILE_REDACTED_01")
	if err != nil || !download.Ready || download.ID != 901 || !download.ExpiresAt.Equal(time.Unix(1779443600, 0).UTC()) {
		t.Fatalf("download=%#v err=%v", download, err)
	}

	running, err := modelFixtureClient(t, cases["open-model-running"]).ListRunningOpenModels(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED")
	if err != nil || len(running) != 1 || running[0].ModelStatus != OpenModelReconstructionExecuting || running[0].ReconstructionProgress != 42 {
		t.Fatalf("running=%#v err=%v", running, err)
	}
	empty, err := modelFixtureClient(t, cases["open-model-running-empty"]).ListRunningOpenModels(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty=%#v err=%v", empty, err)
	}
	failed, err := modelFixtureClient(t, cases["open-model-detail-failed"]).GetOpenModel(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "OPEN_MODEL_REDACTED_02")
	if err != nil || failed.ModelStatus != OpenModelReconstructionFailed || failed.ErrorCode == 0 {
		t.Fatalf("failed=%#v err=%v", failed, err)
	}
	resource, err := modelFixtureClient(t, cases["open-model-resource-imported"]).GetOpenModelResource(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "RESOURCE_REDACTED_01")
	if err != nil || resource.Status != 1 || len(resource.FileNames) != 2 {
		t.Fatalf("resource=%#v err=%v", resource, err)
	}

	var startRequest OpenModelStartRequest
	if json.Unmarshal(cases["open-model-start"].RequestBody, &startRequest) != nil {
		t.Fatal("invalid start request fixture")
	}
	started, err := modelFixtureClient(t, cases["open-model-start"]).StartOpenModelReconstruction(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", startRequest)
	if err != nil || started.Model3D == nil || started.Model3D.Status != OpenModelRequestingResource {
		t.Fatalf("started=%#v err=%v", started, err)
	}
	if err := modelFixtureClient(t, cases["open-model-stop"]).StopOpenModelReconstruction(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "OPEN_MODEL_REDACTED_03"); err != nil {
		t.Fatal(err)
	}
	if err := modelFixtureClient(t, cases["open-model-delete"]).DeleteOpenModel(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "OPEN_MODEL_REDACTED_03"); err != nil {
		t.Fatal(err)
	}
	if err := modelFixtureClient(t, cases["open-model-resource-delete"]).DeleteOpenModelResource(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "RESOURCE_REDACTED_03"); err != nil {
		t.Fatal(err)
	}
	credential, err := modelFixtureClient(t, cases["open-model-upload-token"]).ObtainOpenModelUploadCredential(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED")
	if err != nil || credential.CloudName != "ali" || credential.SessionToken == "" || !credential.ExpiresAt.Equal(time.Unix(1779443600, 0).UTC()) {
		t.Fatalf("credential=%#v err=%v", credential, err)
	}
	var callbackRequest OpenModelUploadCallbackRequest
	if json.Unmarshal(cases["open-model-upload-callback"].RequestBody, &callbackRequest) != nil {
		t.Fatal("invalid callback request fixture")
	}
	callback, err := modelFixtureClient(t, cases["open-model-upload-callback"]).NotifyOpenModelUploadComplete(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", callbackRequest)
	if err != nil || callback.UploadCount != 2 || len(callback.FileNames) != 2 {
		t.Fatalf("callback=%#v err=%v", callback, err)
	}
}

func TestModelFixtureCoversReleasedManifestAndContainsNoUsableSecrets(t *testing.T) {
	cases, contents := loadModelFixture(t)
	wantEndpointIDs := map[string]struct{}{
		"458069507e0": {}, "458069508e0": {}, "458069510e0": {}, "458069511e0": {}, "458069512e0": {},
		"458069513e0": {}, "458069514e0": {}, "458069515e0": {}, "458069516e0": {},
		"458069517e0": {}, "458069518e0": {}, "458069519e0": {}, "463460267e0": {},
	}
	for _, item := range cases {
		delete(wantEndpointIDs, item.EndpointID)
	}
	if len(wantEndpointIDs) != 0 {
		t.Fatalf("model fixture is missing released endpoint IDs: %#v", wantEndpointIDs)
	}
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.`),
		regexp.MustCompile(`\b(?:7CT|1581F)[A-Z0-9]{8,}\b`),
		regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`),
	} {
		if pattern.Match(contents) {
			t.Fatalf("model fixture contains forbidden identity or secret pattern %s", pattern)
		}
	}
	var document any
	if json.Unmarshal(contents, &document) != nil {
		t.Fatal("model fixture is not valid JSON")
	}
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
		case string:
			if strings.Contains(strings.ToLower(key), "url") || key == "end_point" {
				parsed, err := url.Parse(typed)
				if err != nil || parsed.Scheme != "https" || parsed.Hostname() != "objects.vendor.example" {
					t.Fatalf("model fixture contains unsafe URL host")
				}
			}
			if strings.Contains(strings.ToLower(key), "token") || strings.Contains(strings.ToLower(key), "key") || key == "callback_param" || key == "etag" {
				if !strings.Contains(typed, "REDACTED") && key != "zip_file_key" {
					t.Fatalf("model fixture contains an unredacted secret-like field %s", key)
				}
			}
		}
	}
	scan(document, "")
}

func TestModelClientsRejectInvalidInputBeforeNetwork(t *testing.T) {
	called := false
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return response(http.StatusOK, []byte(`{"code":0,"message":"","data":{}}`), nil), nil
	}), nil)
	ctx := context.Background()
	tests := []struct {
		name string
		call func() error
	}{
		{name: "missing project", call: func() error { _, err := client.ListModels(ctx, "TOKEN_REDACTED", ""); return err }},
		{name: "unsafe model id", call: func() error {
			_, err := client.GetModel(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "../model")
			return err
		}},
		{name: "unsafe resource id", call: func() error {
			_, err := client.GetOpenModelResource(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "resource/other")
			return err
		}},
		{name: "start without model type", call: func() error {
			_, err := client.StartOpenModelReconstruction(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", OpenModelStartRequest{ResourceUUID: "RESOURCE_REDACTED"})
			return err
		}},
		{name: "traditional create with unknown format", call: func() error {
			_, err := client.CreateModelReconstruction(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", ModelReconstructionRequest{
				Name: "invalid", ReconstructionTypes: []ModelFileType{ModelFile3D}, SimplifiedFactor: 0.2,
				TaskFolderID: 1, WKT: "EPSG:4326", QualityLevel: "medium", ReconstructionMode: "normal",
				GenerateModelFormats: []string{"future"},
			})
			return err
		}},
		{name: "start with malformed parameter", call: func() error {
			_, err := client.StartOpenModelReconstruction(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", OpenModelStartRequest{ResourceUUID: "RESOURCE_REDACTED", Parameter3D: "not-json"})
			return err
		}},
		{name: "callback duplicate file", call: func() error {
			_, err := client.NotifyOpenModelUploadComplete(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", OpenModelUploadCallbackRequest{
				ResourceUUID: "RESOURCE_REDACTED", Callback: "CALLBACK_REDACTED",
				Files: []OpenModelUploadedFile{{Name: "one.jpg", ETag: "ETAG_REDACTED"}, {Name: "one.jpg", ETag: "ETAG_REDACTED"}},
			})
			return err
		}},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			called = false
			if err := item.call(); !IsSafeCode(err, "request_invalid") && !IsSafeCode(err, "scope_forbidden") {
				t.Fatalf("error=%v", err)
			}
			if called {
				t.Fatal("invalid model request reached upstream")
			}
		})
	}
}

func TestModelClientsFailClosedOnInvalidStatesAndTemporaryCredentials(t *testing.T) {
	now := time.Unix(1779440000, 0).UTC()
	tests := []struct {
		name string
		body string
		call func(*Client) error
		code string
	}{
		{name: "unknown model type", body: `{"code":0,"message":"","data":{"list":[{"id":1,"name":"bad","file_type":"future","show_on_map":false,"size":1,"update_at":2,"create_at":1}]}}`, code: "schema_incompatible", call: func(c *Client) error {
			_, err := c.ListModels(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED")
			return err
		}},
		{name: "unknown open status", body: `{"code":0,"message":"","data":{"resource_uuid":"RESOURCE_REDACTED","model_uuid":"MODEL_REDACTED","model_type":2,"model_status":99,"model_size":0,"reconstruction_progress":0,"error_code":0,"zip_status":0,"zip_progress":0,"zip_file_key":""}}`, code: "schema_incompatible", call: func(c *Client) error {
			_, err := c.GetOpenModel(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", "MODEL_REDACTED")
			return err
		}},
		{name: "forbidden model host", body: `{"code":0,"message":"","data":{"id":1,"url":"https://attacker.example/model.zip?Expires=1779443600"}}`, code: "temporary_link_host_forbidden", call: func(c *Client) error {
			_, err := c.GetModelDownloadURL(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", "FILE_REDACTED")
			return err
		}},
		{name: "expired model URL", body: `{"code":0,"message":"","data":{"id":1,"url":"https://objects.vendor.example/model.zip?Expires=1779430000&Signature=REDACTED"}}`, code: "temporary_link_expired", call: func(c *Client) error {
			_, err := c.GetModelDownloadURL(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", "FILE_REDACTED")
			return err
		}},
		{name: "expired upload credential", body: `{"code":0,"message":"","data":{"cloud_name":"ali","access_key_id":"ACCESS_REDACTED","secret_access_key":"SECRET_REDACTED","session_token":"TOKEN_REDACTED","region":"cn-test","cloud_bucket_name":"BUCKET_REDACTED","callback_param":"CALLBACK_REDACTED","store_path":"models/{fileName}","expire_time":1779430000,"end_point":"https://objects.vendor.example"}}`, code: "temporary_link_expired", call: func(c *Client) error {
			_, err := c.ObtainOpenModelUploadCredential(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED")
			return err
		}},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
				return response(http.StatusOK, []byte(item.body), nil), nil
			}), func(config *Config) {
				config.Now = func() time.Time { return now }
				config.AllowedLinkHosts = []string{"objects.vendor.example"}
			})
			if err := item.call(client); !IsSafeCode(err, item.code) {
				t.Fatalf("error=%v want=%s", err, item.code)
			}
		})
	}
}

func TestModelWriteAndCredentialRequestsNeverRetryAnUnknownOutcome(t *testing.T) {
	ctx := context.Background()
	start := OpenModelStartRequest{ResourceUUID: "RESOURCE_REDACTED", Parameter3D: `{"quality":"standard"}`}
	callback := OpenModelUploadCallbackRequest{
		ResourceUUID: "RESOURCE_REDACTED", Callback: "CALLBACK_REDACTED",
		Files: []OpenModelUploadedFile{{Name: "one.jpg", ETag: "ETAG_REDACTED"}},
	}
	tests := []struct {
		name string
		call func(*Client) error
	}{
		{name: "traditional create", call: func(c *Client) error {
			_, err := c.CreateModelReconstruction(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", ModelReconstructionRequest{
				Name: "synthetic", ReconstructionTypes: []ModelFileType{ModelFile3D}, SimplifiedFactor: 0.2,
				TaskFolderID: 1, WKT: "EPSG:4326", QualityLevel: "medium", ReconstructionMode: "normal",
				GenerateModelFormats: []string{"b3dm"},
			})
			return err
		}},
		{name: "start", call: func(c *Client) error {
			_, err := c.StartOpenModelReconstruction(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", start)
			return err
		}},
		{name: "stop", call: func(c *Client) error {
			return c.StopOpenModelReconstruction(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "MODEL_REDACTED")
		}},
		{name: "delete model", call: func(c *Client) error {
			return c.DeleteOpenModel(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "MODEL_REDACTED")
		}},
		{name: "delete resource", call: func(c *Client) error {
			return c.DeleteOpenModelResource(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", "RESOURCE_REDACTED")
		}},
		{name: "obtain token", call: func(c *Client) error {
			_, err := c.ObtainOpenModelUploadCredential(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED")
			return err
		}},
		{name: "upload callback", call: func(c *Client) error {
			_, err := c.NotifyOpenModelUploadComplete(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", callback)
			return err
		}},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			attempts := 0
			client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
				attempts++
				return response(http.StatusServiceUnavailable, []byte(`{"code":200500,"message":"unavailable","data":{}}`), nil), nil
			}), func(config *Config) { config.MaxRetries = 3 })
			if err := item.call(client); err == nil {
				t.Fatal("unknown write outcome was reported as success")
			}
			if attempts != 1 {
				t.Fatalf("unknown write outcome retried %d times", attempts)
			}
		})
	}
}
