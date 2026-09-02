package flighthub

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/credentials"
	"aerosight/worker/internal/outbox"
)

const (
	FlightHubManagementWriteEventType = "flighthub.management_write.requested"
	projectMemberWriteCapability      = "organization.project-member.write"
	projectMemberWriteAction          = "flighthub.organization.project-member-upsert"
)

type ManagementWriteClient interface {
	AddProjectMembers(context.Context, string, string, AddProjectMembersRequest) error
	ListProjectUsers(context.Context, string, string, ProjectUserListOptions) (PageResult[ProjectUser], error)
	ListProjectMembers(context.Context, string, string) ([]ProjectMember, error)
}

type managementWriteJob struct {
	ID, Status, CapabilityCode, FeatureFlag, PreviewDigest string
	ProjectID, TeamID, RequestedByUserID                   int
	ConnectorInstanceID                                    int64
	AttemptCount                                           int
	Authorized, Connected, FeatureEnabled, CapabilityReady bool
	ApprovalValid                                          bool
	RequestEnvelope, PreviewJSON                           json.RawMessage
	Instance                                               connector.Instance
}

func authorizeManagementWrite(job managementWriteJob) error {
	if job.CapabilityCode != projectMemberWriteCapability || job.FeatureFlag != FlightHubProjectMemberFeatureFlag ||
		!job.Authorized || !job.Connected || !job.FeatureEnabled || !job.CapabilityReady || !job.ApprovalValid {
		return &APIError{SafeCode: "action_disabled"}
	}
	return nil
}

type ManagementWriteHandler struct {
	db         *sql.DB
	client     ManagementWriteClient
	resolver   TokenResolver
	authSecret string
}

func NewManagementWriteHandler(db *sql.DB, client ManagementWriteClient, resolver TokenResolver, authSecret string) (*ManagementWriteHandler, error) {
	if db == nil || client == nil || resolver == nil || strings.TrimSpace(authSecret) == "" {
		return nil, errors.New("FlightHub management write dependencies are required")
	}
	return &ManagementWriteHandler{db: db, client: client, resolver: resolver, authSecret: authSecret}, nil
}

func (handler *ManagementWriteHandler) load(ctx context.Context, projectID int, jobID string) (job managementWriteJob, err error) {
	var credential, scope []byte
	err = handler.db.QueryRowContext(ctx, `select job.id::text,job.project_id,job.team_id,job.connector_instance_id,
		job.requested_by_user_id,job.capability_code,job.feature_flag,job.status,job.attempt_count,
		job.request_envelope_json,job.preview_json,job.preview_digest,adapter.credential_envelope_json,adapter.discovery_scope_json,
		(member.role='owner' or exists(select 1 from project_permissions permission where permission.project_id=job.project_id
		  and permission.team_id=job.team_id and permission.user_id=job.requested_by_user_id and permission.permission='organization:manage')),
		adapter.status='connected',coalesce(flags.flighthub_action_flags_json @> jsonb_build_object(job.feature_flag,true),false),
		exists(select 1 from connector_capability_snapshots capability where capability.project_id=job.project_id
		  and capability.connector_instance_id=job.connector_instance_id and capability.capability_code=job.capability_code
		  and capability.status='supported' and capability.evidence_level='field-write'
		  and capability.device_model is null and capability.firmware_version is null
		  and (capability.expires_at is null or capability.expires_at>now())),
		exists(select 1 from approval_requests approval where approval.id=job.approval_request_id and approval.project_id=job.project_id
		  and approval.team_id=job.team_id and approval.resource_type='connector' and approval.resource_id=job.connector_instance_id::text
		  and approval.action=$3 and approval.status='approved' and approval.expires_at>now()
		  and approval.context_json->>'previewDigest'=job.preview_digest)
	 from connector_management_write_jobs job
	 join device_adapters adapter on adapter.id=job.connector_instance_id and adapter.project_id=job.project_id
	 join connector_definitions definition on definition.id=adapter.connector_definition_id
	   and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'
	 join team_members member on member.team_id=job.team_id and member.user_id=job.requested_by_user_id
	 left join project_feature_flags flags on flags.project_id=job.project_id
	 where job.project_id=$1 and job.id=$2::uuid`, projectID, jobID, projectMemberWriteAction).Scan(
		&job.ID, &job.ProjectID, &job.TeamID, &job.ConnectorInstanceID, &job.RequestedByUserID,
		&job.CapabilityCode, &job.FeatureFlag, &job.Status, &job.AttemptCount, &job.RequestEnvelope,
		&job.PreviewJSON, &job.PreviewDigest, &credential, &scope, &job.Authorized, &job.Connected,
		&job.FeatureEnabled, &job.CapabilityReady, &job.ApprovalValid)
	job.Instance = connector.Instance{ID: job.ConnectorInstanceID, ProjectID: job.ProjectID, ConnectorKey: ConnectorKey,
		Version: ConnectorVersion, CredentialEnvelope: credential, DiscoveryScope: scope}
	return job, err
}

func (handler *ManagementWriteHandler) finish(ctx context.Context, job managementWriteJob, status, code string, result map[string]any) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = handler.db.ExecContext(ctx, `update connector_management_write_jobs set status=$3,last_error_code=nullif($4,''),
		result_json=$5,reconciliation_count=reconciliation_count+case when $3 in('succeeded','blocked') then 1 else 0 end,
		unknown_at=case when $3='blocked' then now() else null end,completed_at=case when $3 in('succeeded','failed','blocked') then now() else null end,
		updated_at=now() where project_id=$1 and id=$2::uuid and status in('queued','executing','accepted')`, job.ProjectID, job.ID, status, code, encoded)
	return err
}

func managementPreviewDigest(raw json.RawMessage) (string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func previewMatchesRequest(raw json.RawMessage, request AddProjectMembersRequest, projectID int, connectorID int64) bool {
	var preview struct {
		ProjectID   int   `json:"projectId"`
		ConnectorID int64 `json:"connectorInstanceId"`
		Members     []struct {
			Reference, Role, Nickname string
		} `json:"members"`
	}
	if json.Unmarshal(raw, &preview) != nil || preview.ProjectID != projectID || preview.ConnectorID != connectorID || len(preview.Members) != len(request.AddUsers) {
		return false
	}
	for index, member := range request.AddUsers {
		if preview.Members[index].Reference != secureRemoteKey(member.UserID)[:12] || preview.Members[index].Role != member.Role || preview.Members[index].Nickname != member.Nickname {
			return false
		}
	}
	return true
}

func (handler *ManagementWriteHandler) targetsBelongToOrganization(ctx context.Context, job managementWriteJob, request AddProjectMembersRequest, organizationUUID string) bool {
	var exists bool
	if handler.db.QueryRowContext(ctx, `select exists(select 1 from connector_remote_resources where project_id=$1 and connector_instance_id=$2
		and resource_kind='organization' and remote_id=$3 and status='active')`, job.ProjectID, job.ConnectorInstanceID, secureRemoteKey(organizationUUID)).Scan(&exists) != nil || !exists {
		return false
	}
	for _, member := range request.AddUsers {
		if handler.db.QueryRowContext(ctx, `select exists(select 1 from connector_remote_resources where project_id=$1 and connector_instance_id=$2
			and resource_kind='organization-user' and remote_id=$3 and status='active')`, job.ProjectID, job.ConnectorInstanceID, secureRemoteKey(member.UserID)).Scan(&exists) != nil || !exists {
			return false
		}
	}
	return true
}

func (handler *ManagementWriteHandler) Handler(ctx context.Context, _ *sql.Tx, event outbox.Event) error {
	var payload struct {
		JobID string `json:"jobId"`
	}
	if json.Unmarshal(event.Payload, &payload) != nil || strings.TrimSpace(payload.JobID) == "" {
		return &APIError{SafeCode: "request_invalid"}
	}
	job, err := handler.load(ctx, event.ProjectID, payload.JobID)
	if err != nil {
		return err
	}
	if err := authorizeManagementWrite(job); err != nil {
		if job.Status == "queued" {
			_, err = handler.db.ExecContext(ctx, `update connector_management_write_jobs set status='failed',attempt_count=0,
				last_error_code='action_disabled',completed_at=now(),updated_at=now() where project_id=$1 and id=$2::uuid and status='queued'`, job.ProjectID, job.ID)
		}
		return err
	}
	if job.Status == "succeeded" || job.Status == "failed" || job.Status == "blocked" {
		return nil
	}
	if job.Status == "executing" || job.AttemptCount > 0 {
		return handler.finish(ctx, job, "blocked", "result_unknown_after_restart", map[string]any{"final": false})
	}
	var input AddProjectMembersRequest
	var envelope credentials.Envelope
	if json.Unmarshal(job.RequestEnvelope, &envelope) != nil || credentials.DecryptJSON(envelope, handler.authSecret,
		credentials.AAD("flighthub-management-write", job.ID, job.ProjectID), &input) != nil {
		return handler.finish(ctx, job, "failed", "request_unavailable", map[string]any{"final": true})
	}
	digest, digestErr := managementPreviewDigest(job.PreviewJSON)
	scope, scopeErr := parseScope(job.Instance.DiscoveryScope)
	if digestErr != nil || digest != job.PreviewDigest || scopeErr != nil || scope.OrganizationUUID == "" ||
		!previewMatchesRequest(job.PreviewJSON, input, job.ProjectID, job.ConnectorInstanceID) ||
		!handler.targetsBelongToOrganization(ctx, job, input, scope.OrganizationUUID) {
		return handler.finish(ctx, job, "failed", "scope_forbidden", map[string]any{"final": true})
	}
	changed, err := handler.db.ExecContext(ctx, `update connector_management_write_jobs set status='executing',attempt_count=1,
		attempted_at=now(),updated_at=now() where project_id=$1 and id=$2::uuid and status='queued' and attempt_count=0`, job.ProjectID, job.ID)
	if err != nil {
		return err
	}
	if count, _ := changed.RowsAffected(); count != 1 {
		return nil
	}
	token, err := handler.resolver.ResolveToken(ctx, job.Instance)
	if err != nil {
		return handler.finish(ctx, job, "failed", safeWorkflowCode(err), map[string]any{"final": true})
	}
	defer func() { token = "" }()
	if err := handler.client.AddProjectMembers(ctx, token, scope.ProjectUUID, input); err != nil {
		return handler.finish(ctx, job, "blocked", "write_result_unknown", map[string]any{"final": false})
	}
	_, err = handler.db.ExecContext(ctx, `update connector_management_write_jobs set status='accepted',updated_at=now()
		where project_id=$1 and id=$2::uuid and status='executing'`, job.ProjectID, job.ID)
	if err != nil {
		return err
	}
	users, err := handler.listAllProjectUsers(ctx, token, scope.ProjectUUID)
	if err != nil {
		return handler.finish(ctx, job, "blocked", "reconciliation_unavailable", map[string]any{"accepted": true, "final": false})
	}
	members, err := handler.client.ListProjectMembers(ctx, token, scope.ProjectUUID)
	if err != nil || !projectMembersMatch(input.AddUsers, users, members) {
		return handler.finish(ctx, job, "blocked", "reconciliation_mismatch", map[string]any{"accepted": true, "final": false})
	}
	return handler.finish(ctx, job, "succeeded", "", map[string]any{"accepted": true, "final": true, "verifiedCount": len(input.AddUsers)})
}

func (handler *ManagementWriteHandler) listAllProjectUsers(ctx context.Context, token, projectUUID string) ([]ProjectUser, error) {
	all := make([]ProjectUser, 0)
	for page := 1; page <= 100; page++ {
		result, err := handler.client.ListProjectUsers(ctx, token, projectUUID, ProjectUserListOptions{PageOptions: PageOptions{Page: page, PageSize: 100}})
		if err != nil {
			return nil, err
		}
		all = append(all, result.List...)
		if len(result.List) < 100 || len(all) >= result.Pagination.Total {
			return all, nil
		}
	}
	return nil, schemaError()
}

func projectMembersMatch(expected []AddProjectMember, users []ProjectUser, members []ProjectMember) bool {
	userByID := make(map[string]ProjectUser, len(users))
	for _, user := range users {
		userByID[user.UserID] = user
	}
	memberByID := make(map[string]ProjectMember, len(members))
	for _, member := range members {
		memberByID[member.UserID] = member
	}
	for _, target := range expected {
		user, userOK := userByID[target.UserID]
		member, memberOK := memberByID[target.UserID]
		if !userOK || !memberOK || user.Role != target.Role || member.ProjectRole != target.Role || member.ProjectCallsign != target.Nickname {
			return false
		}
	}
	return true
}
