package flighthub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

type contractFixture struct {
	ContractVersion      string         `json:"contractVersion"`
	DeviceDirectoryLimit int            `json:"deviceDirectoryLimit"`
	Cases                []contractCase `json:"cases"`
}

type contractCase struct {
	Name          string            `json:"name"`
	Endpoint      string            `json:"endpoint"`
	HTTPStatus    int               `json:"httpStatus"`
	Headers       map[string]string `json:"headers"`
	Body          json.RawMessage   `json:"body"`
	GeneratedBody *generatedBody    `json:"generatedBody"`
	Expected      expectedResult    `json:"expected"`
}

type generatedBody struct {
	Repeat       int    `json:"repeat"`
	TemplateCase string `json:"templateCase"`
}

type expectedResult struct {
	Kind              string `json:"kind"`
	ItemCount         *int   `json:"itemCount"`
	SafeCode          string `json:"safeCode"`
	Retryable         *bool  `json:"retryable"`
	RetryAfterSeconds *int   `json:"retryAfterSeconds"`
}

func loadContractFixture(t *testing.T) (contractFixture, []byte) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve FlightHub fixture test path")
	}
	path := filepath.Join(filepath.Dir(currentFile), "../../../../contracts/dji-flighthub/v2/fixtures/cases.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture contractFixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture, contents
}

func TestSharedContractFixtureCoverage(t *testing.T) {
	fixture, _ := loadContractFixture(t)
	if fixture.ContractVersion == "" || fixture.DeviceDirectoryLimit != 1000 {
		t.Fatalf("unexpected FlightHub contract metadata: %#v", fixture)
	}

	byName := make(map[string]contractCase, len(fixture.Cases))
	for _, item := range fixture.Cases {
		byName[item.Name] = item
	}
	required := []string{
		"project-list", "project-empty", "device-directory", "device-directory-limit",
		"http-401", "business-200401", "http-403", "http-404", "http-429",
		"http-503", "malformed-response", "system-health", "organization-list",
		"device-state-dock", "control-context-required", "wayline-list", "live-share-empty",
		"flight-area-list", "model-list", "temporary-credential-redacted-shape",
	}
	for _, name := range required {
		if _, ok := byName[name]; !ok {
			t.Fatalf("missing shared contract case %s", name)
		}
	}

	limit := byName["device-directory-limit"]
	if limit.GeneratedBody == nil || limit.GeneratedBody.Repeat != fixture.DeviceDirectoryLimit || limit.Expected.SafeCode != "directory_limit_reached" {
		t.Fatalf("invalid device directory limit case: %#v", limit)
	}

	expectedErrors := map[string]string{
		"http-401":           "credential_invalid",
		"business-200401":    "credential_invalid",
		"http-403":           "scope_forbidden",
		"http-404":           "scope_not_found",
		"http-429":           "rate_limited",
		"http-503":           "upstream_unavailable",
		"malformed-response": "schema_incompatible",
	}
	for name, safeCode := range expectedErrors {
		if got := byName[name].Expected.SafeCode; got != safeCode {
			t.Fatalf("case %s safe code = %q, want %q", name, got, safeCode)
		}
	}
	if byName["http-429"].Headers["Retry-After"] != "3" {
		t.Fatal("rate-limit fixture must carry a bounded Retry-After value")
	}
}

func TestSharedContractFixtureFieldsAndSanitization(t *testing.T) {
	fixture, contents := loadContractFixture(t)
	byName := make(map[string]contractCase, len(fixture.Cases))
	for _, item := range fixture.Cases {
		byName[item.Name] = item
	}

	var projects struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				Name    string `json:"name"`
				UUID    string `json:"uuid"`
				OrgUUID string `json:"org_uuid"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(byName["project-list"].Body, &projects); err != nil {
		t.Fatal(err)
	}
	if projects.Code != 0 || len(projects.Data.List) != 2 || projects.Data.List[0].UUID == "" || projects.Data.List[0].OrgUUID == "" {
		t.Fatalf("invalid project-list fixture: %#v", projects)
	}

	var directory struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				Gateway struct {
					SN    string `json:"sn"`
					Model struct {
						Key   string `json:"key"`
						Class string `json:"class"`
					} `json:"device_model"`
				} `json:"gateway"`
				Drone struct {
					SN    string `json:"sn"`
					Model struct {
						Key   string `json:"key"`
						Class string `json:"class"`
					} `json:"device_model"`
				} `json:"drone"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(byName["device-directory"].Body, &directory); err != nil {
		t.Fatal(err)
	}
	if directory.Code != 0 || len(directory.Data.List) != 1 || directory.Data.List[0].Gateway.Model.Class != "airport" || directory.Data.List[0].Drone.Model.Class != "drone" {
		t.Fatalf("invalid device-directory fixture: %#v", directory)
	}

	patterns := []*regexp.Regexp{
		regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.`),
		regexp.MustCompile(`7CT[A-Z0-9]{8,}`),
		regexp.MustCompile(`1581F[A-Z0-9]{8,}`),
		regexp.MustCompile(`(?i)https?://[^"[:space:]]+[?&](token|signature|x-amz-credential)=`),
	}
	for _, pattern := range patterns {
		if pattern.Match(contents) {
			t.Fatalf("shared fixture contains forbidden credential or serial pattern %s", pattern)
		}
	}
	credentialPattern := regexp.MustCompile(`(?i)"(access_key|secret_key|session_token)"\s*:\s*"([^"]+)"`)
	for _, match := range credentialPattern.FindAllSubmatch(contents, -1) {
		if len(match) != 3 || !strings.HasSuffix(string(match[2]), "REDACTED") {
			t.Fatalf("shared fixture contains an unredacted temporary credential")
		}
	}

	uuidPattern := regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`)
	for _, match := range uuidPattern.FindAll(contents, -1) {
		if !strings.HasPrefix(string(match), "00000000-0000-4000-8000-") {
			t.Fatalf("shared fixture contains a non-placeholder UUID: %s", match)
		}
	}
}
