package flighthub

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

type adminContractFixture struct {
	ContractVersion string              `json:"contractVersion"`
	Cases           []adminContractCase `json:"cases"`
}

type adminContractCase struct {
	Name    string          `json:"name"`
	Method  string          `json:"method"`
	Path    string          `json:"path"`
	Scope   string          `json:"scope"`
	Request json.RawMessage `json:"request"`
	Body    json.RawMessage `json:"body"`
}

func loadAdminFixture(t *testing.T) (map[string]adminContractCase, []byte) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve admin fixture path")
	}
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), "../../../../contracts/dji-flighthub/v2/fixtures/admin_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture adminContractFixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.ContractVersion == "" || len(fixture.Cases) != 14 {
		t.Fatalf("invalid admin fixture metadata: version=%q cases=%d", fixture.ContractVersion, len(fixture.Cases))
	}
	byName := make(map[string]adminContractCase, len(fixture.Cases))
	for _, item := range fixture.Cases {
		if _, duplicate := byName[item.Name]; duplicate {
			t.Fatalf("duplicate admin fixture %q", item.Name)
		}
		byName[item.Name] = item
	}
	return byName, contents
}

func adminFixtureClient(t *testing.T, item adminContractCase) *Client {
	t.Helper()
	return testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != item.Method || request.URL.RequestURI() != item.Path {
			t.Fatalf("request = %s %s, want %s %s", request.Method, request.URL.RequestURI(), item.Method, item.Path)
		}
		if request.Header.Get("X-User-Token") != "TOKEN_REDACTED" || request.Header.Get("X-Request-Id") != "request-redacted" || request.Header.Get("X-Language") != "zh" || request.Header.Get("Accept") != "application/json" {
			t.Fatalf("missing required common headers: %#v", request.Header)
		}
		wantProject := item.Scope == "project"
		if got := request.Header.Get("X-Project-Uuid"); (wantProject && got != "PROJECT_REDACTED") || (!wantProject && got != "") {
			t.Fatalf("project scope header = %q for scope %q", got, item.Scope)
		}
		var body []byte
		if request.Body != nil {
			var err error
			body, err = io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
		}
		if item.Request == nil {
			if len(body) != 0 || request.Header.Get("Content-Type") != "" {
				t.Fatalf("unexpected request body or content type: %q %q", body, request.Header.Get("Content-Type"))
			}
		} else {
			if request.Header.Get("Content-Type") != "application/json" || !jsonEqual(body, item.Request) {
				t.Fatalf("request body = %s, want %s", body, item.Request)
			}
		}
		return response(http.StatusOK, item.Body, nil), nil
	}), func(config *Config) {
		config.AllowedLinkHosts = []string{"objects.vendor.example"}
	})
}

func jsonEqual(left, right []byte) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && bytes.Equal(mustCanonicalJSON(leftValue), mustCanonicalJSON(rightValue))
}

func mustCanonicalJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}

func TestAdminTypedClientsUseOfficialHeadersScopesAndSchemas(t *testing.T) {
	cases, _ := loadAdminFixture(t)
	ctx := context.Background()

	t.Run("health", func(t *testing.T) {
		result, err := adminFixtureClient(t, cases["health"]).CheckHealth(ctx, "TOKEN_REDACTED")
		if err != nil || !result.Healthy {
			t.Fatalf("health=%#v err=%v", result, err)
		}
	})
	t.Run("system status", func(t *testing.T) {
		result, err := adminFixtureClient(t, cases["system-status"]).CheckSystemStatus(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED")
		if err != nil || !result.Healthy {
			t.Fatalf("health=%#v err=%v", result, err)
		}
	})
	t.Run("storage sts", func(t *testing.T) {
		result, err := adminFixtureClient(t, cases["storage-sts"]).CreateStorageSTS(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", StorageSTSRequest{SpecifyPath: "tests/object.bin", FileUUID: "FILE_REDACTED_01"})
		if err != nil || result.Provider != "ali" || result.Credentials.ExpireSeconds != 900 || result.ExpiresAt.IsZero() {
			t.Fatalf("sts=%#v err=%v", result, err)
		}
	})
	t.Run("sn decrypt", func(t *testing.T) {
		result, err := adminFixtureClient(t, cases["sn-decrypt"]).DecryptSNs(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", SNDecryptRequest{EncryptedSNs: []string{"ENCRYPTED_SN_REDACTED_01"}})
		if err != nil || result.Mapping["ENCRYPTED_SN_REDACTED_01"] != "PLAIN_SN_REDACTED_01" {
			t.Fatalf("mapping=%#v err=%v", result.Mapping, err)
		}
	})
	t.Run("organizations", func(t *testing.T) {
		result, err := adminFixtureClient(t, cases["organizations"]).ListOrganizations(ctx, "TOKEN_REDACTED", OrganizationListOptions{Query: "脱敏", SortColumn: "create_time", SortType: "desc"})
		if err != nil || result.Pagination.Total != 1 || len(result.List) != 1 || result.List[0].UUID == "" {
			t.Fatalf("organizations=%#v err=%v", result, err)
		}
	})
	t.Run("organization detail", func(t *testing.T) {
		result, err := adminFixtureClient(t, cases["organization-detail"]).GetOrganization(ctx, "TOKEN_REDACTED", "ORG_REDACTED_01")
		if err != nil || result.UUID == "" || result.Name == "" {
			t.Fatalf("organization=%#v err=%v", result, err)
		}
	})
	t.Run("organization users", func(t *testing.T) {
		result, err := adminFixtureClient(t, cases["organization-users"]).ListOrganizationUsers(ctx, "TOKEN_REDACTED", "ORG_REDACTED_01", OrganizationUserListOptions{Query: "测试", SortColumn: "create_time", SortType: "desc"})
		if err != nil || len(result.List) != 1 || result.List[0].UserID == "" {
			t.Fatalf("organization users=%#v err=%v", result, err)
		}
	})
	t.Run("current organization role", func(t *testing.T) {
		result, err := adminFixtureClient(t, cases["current-organization-role"]).GetCurrentOrganizationRole(ctx, "TOKEN_REDACTED", "ORG_REDACTED_01")
		if err != nil || result.Role != "organization-admin" {
			t.Fatalf("current role=%#v err=%v", result, err)
		}
	})
	t.Run("organization roles", func(t *testing.T) {
		result, err := adminFixtureClient(t, cases["organization-roles"]).ListOrganizationRoles(ctx, "TOKEN_REDACTED", "ORG_REDACTED_01", "organization", PageOptions{})
		if err != nil || len(result.List) != 1 || result.List[0].RoleID == "" {
			t.Fatalf("roles=%#v err=%v", result, err)
		}
	})
	t.Run("organization permissions", func(t *testing.T) {
		result, err := adminFixtureClient(t, cases["organization-permissions"]).ListOrganizationPermissions(ctx, "TOKEN_REDACTED", "ORG_REDACTED_01", "organization")
		if err != nil || len(result) != 1 || result[0].PermissionID == "" {
			t.Fatalf("permissions=%#v err=%v", result, err)
		}
	})
	t.Run("role permissions", func(t *testing.T) {
		result, err := adminFixtureClient(t, cases["role-permissions"]).ListRolePermissions(ctx, "TOKEN_REDACTED", "ORG_REDACTED_01", "organization", []string{"ROLE_REDACTED_01"})
		if err != nil || len(result) != 1 || result[0].PermissionID == "" {
			t.Fatalf("role permissions=%#v err=%v", result, err)
		}
	})
	t.Run("project users", func(t *testing.T) {
		result, err := adminFixtureClient(t, cases["project-users"]).ListProjectUsers(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED", ProjectUserListOptions{Query: "测试", SortColumn: "create_time", SortType: "desc"})
		if err != nil || len(result.List) != 1 || result.List[0].UserID == "" {
			t.Fatalf("project users=%#v err=%v", result, err)
		}
	})
	t.Run("project members", func(t *testing.T) {
		result, err := adminFixtureClient(t, cases["project-members"]).ListProjectMembers(ctx, "TOKEN_REDACTED", "PROJECT_REDACTED")
		if err != nil || len(result) != 1 || result[0].UserID == "" {
			t.Fatalf("project members=%#v err=%v", result, err)
		}
	})
	t.Run("join code", func(t *testing.T) {
		result, err := adminFixtureClient(t, cases["join-code-info"]).GetJoinCodeInfo(ctx, "TOKEN_REDACTED", JoinCodeQuery{ProjectID: "PROJECT_CODE_REDACTED", FastJoinCode: "JOIN_REDACTED", AssociationDroneSN: "AIRCRAFT_REDACTED_01"})
		if err != nil || result.ProjectUUID == "" || result.OrganizationUUID == "" {
			t.Fatalf("join info=%#v err=%v", result, err)
		}
	})
}

func TestAdminFixturesAndTypedClientsFailClosed(t *testing.T) {
	_, contents := loadAdminFixture(t)
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.`),
		regexp.MustCompile(`7CT[A-Z0-9]{8,}`),
		regexp.MustCompile(`1581F[A-Z0-9]{8,}`),
		regexp.MustCompile(`(?i)https?://[^"[:space:]]+[?&](token|signature|x-amz-credential)=`),
	} {
		if pattern.Match(contents) {
			t.Fatalf("admin fixture contains forbidden secret pattern %s", pattern)
		}
	}
	credentialPattern := regexp.MustCompile(`(?i)"(access_key_id|access_key_secret|security_token)"\s*:\s*"([^"]+)"`)
	for _, match := range credentialPattern.FindAllSubmatch(contents, -1) {
		if len(match) != 3 || !strings.HasSuffix(string(match[2]), "REDACTED") {
			t.Fatalf("admin fixture contains unredacted temporary credential")
		}
	}

	t.Run("missing core response field", func(t *testing.T) {
		client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
			return response(http.StatusOK, []byte(`{"code":0,"message":"OK","data":{"pagination":{"page":1,"page_size":20,"total":1},"list":[{"name":"missing id"}]}}`), nil), nil
		}), nil)
		_, err := client.ListOrganizations(context.Background(), "TOKEN_REDACTED", OrganizationListOptions{})
		if !IsSafeCode(err, "schema_incompatible") {
			t.Fatalf("schema error=%v", err)
		}
	})
	t.Run("project scope required before network", func(t *testing.T) {
		called := false
		client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
			called = true
			return nil, nil
		}), nil)
		_, err := client.ListProjectMembers(context.Background(), "TOKEN_REDACTED", "")
		if !IsSafeCode(err, "scope_forbidden") || called {
			t.Fatalf("scope error=%v called=%v", err, called)
		}
	})
}
