import "server-only";

import { randomUUID } from "node:crypto";
import type { QueryResult, QueryResultRow } from "pg";
import { withAuditedProjectWrite } from "@/lib/audit";
import { auditHash } from "@/lib/audit-boundary";
import { credentialAAD, encryptCredentialObject } from "@/lib/credential-encryption";
import { requireCurrentProjectPermission } from "@/lib/data";
import { query } from "@/lib/db";
import { authorizeProjectMemberWrite, flightHubManagementTargetKey, flightHubProjectMemberPreviewInputSchema,
  flightHubProjectMemberWriteInputSchema, PROJECT_MEMBER_WRITE_POLICY, projectMemberPreview,
  type FlightHubProjectMemberPreviewInput, type FlightHubProjectMemberWriteInput } from "@/lib/dji-flighthub-management-write-core";
import { correlationId } from "@/lib/observability";
import { getWebRuntimeConfig } from "@/lib/runtime-config";

type TargetRow = {
  teamId: number; managementGranted: boolean; connectorProjectId: number; connectorTeamId: number;
  connectorStatus: string; projectName: string; organizationName: string; targetCount: number;
  featureEnabled?: boolean; capabilityVerified?: boolean; approvalProjectId?: number | null; approvalTeamId?: number | null;
  approvalResourceType?: string | null; approvalResourceId?: string | null; approvalAction?: string | null;
  approvalStatus?: string | null; approvalUnexpired?: boolean; approvalPreviewDigest?: string | null;
};

function targetKeys(input: FlightHubProjectMemberPreviewInput) {
  return input.members.map((item) => flightHubManagementTargetKey(item.userId));
}

type QueryExecutor = { query<T extends QueryResultRow>(text: string, values?: unknown[]): Promise<QueryResult<T>> };

async function loadTargets(client: QueryExecutor, projectId: number, teamId: number, userId: number,
  connectorInstanceId: number, input: FlightHubProjectMemberPreviewInput, approvalRequestId?: string): Promise<TargetRow> {
  const policy = PROJECT_MEMBER_WRITE_POLICY;
  const result = await client.query<TargetRow>(`select project.team_id::int as "teamId",
      (member.role='owner' or exists(select 1 from project_permissions permission where permission.project_id=project.id
        and permission.team_id=project.team_id and permission.user_id=$4 and permission.permission='organization:manage')) as "managementGranted",
      adapter.project_id::int as "connectorProjectId",adapter.team_id::int as "connectorTeamId",adapter.status as "connectorStatus",
      project.name as "projectName",coalesce(organization.summary_json->>'name','') as "organizationName",
      (select count(*)::int from connector_remote_resources target where target.project_id=adapter.project_id
        and target.connector_instance_id=adapter.id and target.resource_kind='organization-user' and target.status='active'
        and target.remote_id=any($5::text[])) as "targetCount",
      coalesce(flags.flighthub_action_flags_json @> jsonb_build_object($6::text,true),false) as "featureEnabled",
      exists(select 1 from connector_capability_snapshots capability where capability.project_id=adapter.project_id
        and capability.connector_instance_id=adapter.id and capability.capability_code=$7 and capability.status='supported'
        and capability.evidence_level='field-write' and capability.device_model is null and capability.firmware_version is null
        and (capability.expires_at is null or capability.expires_at>now())) as "capabilityVerified",
      approval.project_id as "approvalProjectId",approval.team_id as "approvalTeamId",approval.resource_type as "approvalResourceType",
      approval.resource_id as "approvalResourceId",approval.action as "approvalAction",approval.status as "approvalStatus",
      coalesce(approval.expires_at>now(),false) as "approvalUnexpired",approval.context_json->>'previewDigest' as "approvalPreviewDigest"
    from projects project join team_members member on member.team_id=project.team_id and member.user_id=$4
    join device_adapters adapter on adapter.project_id=project.id and adapter.team_id=project.team_id
    join connector_definitions definition on definition.id=adapter.connector_definition_id
      and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'
    left join project_feature_flags flags on flags.project_id=project.id
    left join connector_remote_resources organization on organization.project_id=adapter.project_id
      and organization.connector_instance_id=adapter.id and organization.resource_kind='organization' and organization.status='active'
    left join approval_requests approval on approval.id=$8::uuid and approval.project_id=project.id
    where project.id=$1 and adapter.id=$2 and adapter.team_id=$3`,
  [projectId,connectorInstanceId,teamId,userId,targetKeys(input),policy.featureFlag,policy.capability,approvalRequestId ?? null]);
  if (!result.rows[0]) throw new Error("FLIGHTHUB_MANAGEMENT_WRITE_SCOPE_MISMATCH");
  return result.rows[0];
}

function previewFor(projectId: number, connectorInstanceId: number, input: FlightHubProjectMemberPreviewInput, row: TargetRow) {
  if (!row.managementGranted) throw new Error("FLIGHTHUB_MANAGEMENT_WRITE_PERMISSION_DENIED");
  if (row.connectorProjectId !== projectId || row.connectorTeamId !== row.teamId || row.connectorStatus !== "connected"
      || row.targetCount !== input.members.length || !row.organizationName) {
    throw new Error("FLIGHTHUB_MANAGEMENT_WRITE_TARGET_MISMATCH");
  }
  return projectMemberPreview({ projectId, connectorInstanceId, projectName: row.projectName,
    organizationName: row.organizationName, members: input.members });
}

export async function previewFlightHubProjectMemberWrite(projectId: number, connectorInstanceId: number, rawInput: unknown) {
  const input = flightHubProjectMemberPreviewInputSchema.parse(rawInput);
  const { user, access } = await requireCurrentProjectPermission(projectId, "project:view");
  const row = await loadTargets({ query }, projectId, access.teamId, user.id, connectorInstanceId, input);
  const preview = previewFor(projectId, connectorInstanceId, input, row);
  const previewDigest = auditHash(preview);
  return { preview, previewDigest, confirmation: "ADD PROJECT MEMBER", approval: {
    resourceType: "connector", resourceId: String(connectorInstanceId), action: PROJECT_MEMBER_WRITE_POLICY.approvalAction,
    context: { previewDigest },
  } };
}

export async function submitFlightHubProjectMemberWrite(projectId: number, rawInput: unknown, requestId?: string | null) {
  const input = flightHubProjectMemberWriteInputSchema.parse(rawInput);
  const { user, access } = await requireCurrentProjectPermission(projectId, "project:view");
  const jobId = randomUUID();
  return withAuditedProjectWrite({ projectId, teamId: access.teamId, actorUserId: user.id,
    requestId: correlationId(requestId), idempotencyKey: input.idempotencyKey,
    action: "connector.project-member-upsert", resourceType: "connector", resourceId: String(input.connectorInstanceId),
    input: { connectorInstanceId: input.connectorInstanceId, previewDigest: input.previewDigest, confirmed: true },
    policyResult: { permission: "organization:manage", capability: PROJECT_MEMBER_WRITE_POLICY.capability,
      featureFlag: PROJECT_MEMBER_WRITE_POLICY.featureFlag, evidence: "field-write", approval: "approved", completion: "worker-readback" },
  }, async (client) => {
    const row = await loadTargets(client, projectId, access.teamId, user.id, input.connectorInstanceId, input, input.approvalRequestId);
    const preview = previewFor(projectId, input.connectorInstanceId, input, row);
    const currentPreviewDigest = auditHash(preview);
    const plan = authorizeProjectMemberWrite(projectId, input, {
      ...row, featureEnabled: Boolean(row.featureEnabled), capabilityVerified: Boolean(row.capabilityVerified), currentPreviewDigest,
      approvalProjectId: row.approvalProjectId ?? null, approvalTeamId: row.approvalTeamId ?? null,
      approvalResourceType: row.approvalResourceType ?? null, approvalResourceId: row.approvalResourceId ?? null,
      approvalAction: row.approvalAction ?? null, approvalStatus: row.approvalStatus ?? null,
      approvalUnexpired: Boolean(row.approvalUnexpired), approvalPreviewDigest: row.approvalPreviewDigest ?? null,
    });
    const request = { add_users: input.members.map((item) => ({ user_id: item.userId, role: item.role, nickname: item.nickname })) };
    const requestDigest = auditHash(request);
    const envelope = encryptCredentialObject(request, getWebRuntimeConfig().authSecret,
      credentialAAD("flighthub-management-write", jobId, projectId));
    const inserted = await client.query<{ id: string; status: string }>(`insert into connector_management_write_jobs(
        id,project_id,team_id,connector_instance_id,requested_by_user_id,approval_request_id,action_kind,capability_code,
        feature_flag,idempotency_key,request_digest,request_envelope_json,preview_digest,preview_json)
      values($1,$2,$3,$4,$5,$6,'project-member-upsert',$7,$8,$9,$10,$11,$12,$13)
      on conflict(project_id,connector_instance_id,action_kind,idempotency_key) do nothing returning id::text,status`,
    [jobId,projectId,access.teamId,input.connectorInstanceId,user.id,input.approvalRequestId,plan.capability,plan.featureFlag,
      input.idempotencyKey,requestDigest,envelope,input.previewDigest,preview]);
    let job = inserted.rows[0], reused = false;
    if (!job) {
      const existing = (await client.query<{ id: string; status: string; requestDigest: string; requestedByUserId: number }>(
        `select id::text,status,request_digest as "requestDigest",requested_by_user_id as "requestedByUserId"
          from connector_management_write_jobs where project_id=$1 and connector_instance_id=$2
            and action_kind='project-member-upsert' and idempotency_key=$3`,
      [projectId,input.connectorInstanceId,input.idempotencyKey])).rows[0];
      if (!existing || existing.requestDigest !== requestDigest || existing.requestedByUserId !== user.id) {
        throw new Error("IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST");
      }
      job = { id: existing.id, status: existing.status }; reused = true;
    }
    await client.query(`insert into outbox_events(project_id,team_id,event_id,event_type,aggregate_type,aggregate_id,payload_json,max_attempts)
      values($1,$2,$3,'flighthub.management_write.requested','connector_management_write_job',$4,$5,8)
      on conflict(event_id) do nothing`, [projectId,access.teamId,`flighthub-management-write:${job.id}`,job.id,{ jobId: job.id }]);
    return { ...job, reused, previewDigest: input.previewDigest, completion: "worker-readback" };
  });
}

export async function readFlightHubManagementWriteJob(projectId: number, connectorInstanceId: number, jobId: string) {
  const { user, access } = await requireCurrentProjectPermission(projectId, "project:view");
  const grant = access.role === "owner" || Boolean((await query(`select 1 from project_permissions where project_id=$1 and team_id=$2
    and user_id=$3 and permission='organization:manage'`, [projectId,access.teamId,user.id])).rows[0]);
  if (!grant) throw new Error("FLIGHTHUB_MANAGEMENT_WRITE_PERMISSION_DENIED");
  const row = (await query(`select id::text,action_kind as action,capability_code as "capabilityCode",preview_digest as "previewDigest",
      status,attempt_count as "attemptCount",reconciliation_count as "reconciliationCount",last_error_code as "lastErrorCode",
      result_json as result,attempted_at as "attemptedAt",unknown_at as "unknownAt",completed_at as "completedAt",
      created_at as "createdAt",updated_at as "updatedAt"
    from connector_management_write_jobs where id=$1::uuid and project_id=$2 and connector_instance_id=$3`,
  [jobId,projectId,connectorInstanceId])).rows[0];
  if (!row) throw new Error("FLIGHTHUB_MANAGEMENT_WRITE_NOT_FOUND");
  return row;
}
