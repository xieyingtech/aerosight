package flighthub

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestManagementWriteGateRejectsProjectAdminCancellationAndCrossScopeBeforeUpstream(t *testing.T) {
	t.Parallel()
	allowed := managementWriteJob{CapabilityCode: projectMemberWriteCapability, FeatureFlag: FlightHubProjectMemberFeatureFlag,
		Authorized: true, Connected: true, FeatureEnabled: true, CapabilityReady: true, ApprovalValid: true}
	for _, mutate := range []func(*managementWriteJob){
		func(job *managementWriteJob) { job.Authorized = false },
		func(job *managementWriteJob) { job.ApprovalValid = false },
		func(job *managementWriteJob) { job.FeatureEnabled = false },
		func(job *managementWriteJob) { job.CapabilityReady = false },
		func(job *managementWriteJob) { job.CapabilityCode = "organization.write" },
	} {
		job := allowed
		mutate(&job)
		calls := 0
		if err := authorizeManagementWrite(job); err == nil {
			calls++
		}
		if calls != 0 {
			t.Fatalf("unauthorized path called upstream")
		}
	}
}

func TestProjectMemberReconciliationRequiresExactRoleAndCallsign(t *testing.T) {
	t.Parallel()
	target := []AddProjectMember{{UserID: "USER_REDACTED", Role: "project-admin", Nickname: "现场管理员"}}
	users := []ProjectUser{{UserID: "USER_REDACTED", Role: "project-admin"}}
	members := []ProjectMember{{UserID: "USER_REDACTED", ProjectRole: "project-admin", ProjectCallsign: "现场管理员"}}
	if !projectMembersMatch(target, users, members) {
		t.Fatal("expected exact reconciliation")
	}
	for _, changed := range [][]ProjectMember{
		{{UserID: "OTHER", ProjectRole: "project-admin", ProjectCallsign: "现场管理员"}},
		{{UserID: "USER_REDACTED", ProjectRole: "project-member", ProjectCallsign: "现场管理员"}},
		{{UserID: "USER_REDACTED", ProjectRole: "project-admin", ProjectCallsign: "其他"}},
	} {
		if projectMembersMatch(target, users, changed) {
			t.Fatal("mismatched read-back must not succeed")
		}
	}
}

func TestAddProjectMembersUsesExactReleasedContractAndNeverRetries(t *testing.T) {
	t.Parallel()
	calls := 0
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.Method != http.MethodPut || request.URL.Path != "/openapi/v2.0/project/member" ||
			request.Header.Get("X-Project-Uuid") != "PROJECT_REDACTED" || request.Header.Get("X-User-Token") != "TOKEN_REDACTED" {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		body, _ := io.ReadAll(request.Body)
		var decoded map[string]any
		if json.Unmarshal(body, &decoded) != nil || !strings.Contains(string(body), `"add_users"`) ||
			!strings.Contains(string(body), `"role":"project-admin"`) || !strings.Contains(string(body), `"nickname":"现场管理员"`) {
			t.Fatalf("unexpected body %s", body)
		}
		return response(http.StatusServiceUnavailable, []byte(`{"code":500000,"message":"redacted","data":null}`), nil), nil
	}), func(config *Config) { config.MaxRetries = 3 })
	err := client.AddProjectMembers(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", AddProjectMembersRequest{AddUsers: []AddProjectMember{{
		UserID: "USER_REDACTED", Role: "project-admin", Nickname: "现场管理员",
	}}})
	if err == nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestAddProjectMembersRejectsInvalidTargetsBeforeCallingUpstream(t *testing.T) {
	t.Parallel()
	calls := 0
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return response(http.StatusOK, []byte(`{"code":0,"message":"","data":null}`), nil), nil
	}), nil)
	for _, input := range []AddProjectMembersRequest{
		{},
		{AddUsers: []AddProjectMember{{UserID: "USER_REDACTED", Role: "owner"}}},
		{AddUsers: []AddProjectMember{{UserID: "USER_REDACTED", Role: "project-member"}, {UserID: "USER_REDACTED", Role: "project-admin"}}},
	} {
		if err := client.AddProjectMembers(context.Background(), "TOKEN_REDACTED", "PROJECT_REDACTED", input); err == nil {
			t.Fatal("expected validation error")
		}
	}
	if calls != 0 {
		t.Fatalf("upstream calls=%d", calls)
	}
}
