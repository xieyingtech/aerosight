import "server-only";

import { randomUUID } from "node:crypto";
import type { PoolClient } from "pg";

import { withAuditedProjectWrite } from "@/lib/audit";
import { auditHash } from "@/lib/audit-boundary";
import { credentialAAD, encryptCredentialObject } from "@/lib/credential-encryption";
import { requireCurrentProjectPermission } from "@/lib/data";
import { authorizeFlightHubModelDelete, flightHubModelDeleteInputSchema, MODEL_DELETE_POLICY,
  modelDeletePreview, type FlightHubModelDeleteAuthorization, type FlightHubModelDeleteInput,
  type FlightHubModelDeletePreview } from "@/lib/dji-flighthub-model-actions-core";
import { query } from "@/lib/db";
import { correlationId } from "@/lib/observability";
import { getWebRuntimeConfig } from "@/lib/runtime-config";

type ModelDeleteTargetRow = {
  teamId: number;
  role: string;
  connectorProjectId: number;
  connectorTeamId: number;
  connectorStatus: string;
  targetProjectId: number | null;
  targetConnectorId: number | null;
  targetKind: string | null;
  targetStatus: string | null;
  targetRemoteVersion: string | null;
  assetId: string | null;
  assetStatus: string | null;
  dependentReferenceCount: number;
};

async function loadTarget(client: Pick<PoolClient, "query">, projectId: number, teamId: number, userId: number,
  connectorInstanceId: number, targetResourceId: number): Promise<ModelDeleteTargetRow> {
  const result = await client.query<ModelDeleteTargetRow>(
    `select $3::int as "teamId",member.role,adapter.project_id as "connectorProjectId",
      adapter.team_id as "connectorTeamId",adapter.status as "connectorStatus",
      target.project_id as "targetProjectId",target.connector_instance_id as "targetConnectorId",
      target.resource_kind as "targetKind",target.status as "targetStatus",target.remote_version as "targetRemoteVersion",
      case when target.canonical_target_type='asset' then target.canonical_target_id end as "assetId",
      asset.status as "assetStatus",
      coalesce((select count(*)::int from connector_asset_access_refs ref
        where ref.project_id=adapter.project_id and ref.connector_instance_id=adapter.id
          and ref.remote_resource_id=target.id),0) as "dependentReferenceCount"
     from device_adapters adapter
     join team_members member on member.team_id=adapter.team_id and member.user_id=$4
     left join connector_remote_resources target on target.id=$5 and target.project_id=adapter.project_id
       and target.connector_instance_id=adapter.id
     left join assets asset on target.canonical_target_type='asset' and asset.project_id=target.project_id
       and asset.id::text=target.canonical_target_id
     where adapter.id=$2 and adapter.project_id=$1 and adapter.team_id=$3`,
    [projectId,connectorInstanceId,teamId,userId,targetResourceId]
  );
  if (!result.rows[0]) throw new Error("FLIGHTHUB_MODEL_DELETE_SCOPE_MISMATCH");
  return result.rows[0];
}

function previewFromTarget(targetResourceId: number, row: ModelDeleteTargetRow): FlightHubModelDeletePreview {
  if (row.targetProjectId === null || row.targetKind === null || row.targetStatus !== "active"
    || row.targetRemoteVersion === null) throw new Error("FLIGHTHUB_MODEL_DELETE_SCOPE_MISMATCH");
  return modelDeletePreview({ targetResourceId, resourceKind: row.targetKind, remoteVersion: row.targetRemoteVersion,
    assetId: row.assetId, assetStatus: row.assetStatus, dependentReferenceCount: row.dependentReferenceCount });
}

export async function previewFlightHubModelDelete(projectId: number, connectorInstanceId: number,
  targetResourceId: number, action: keyof typeof MODEL_DELETE_POLICY) {
  const { user, access } = await requireCurrentProjectPermission(projectId, "project:view");
  const result = await query<ModelDeleteTargetRow>(
    `select $3::int as "teamId",member.role,adapter.project_id as "connectorProjectId",
      adapter.team_id as "connectorTeamId",adapter.status as "connectorStatus",
      target.project_id as "targetProjectId",target.connector_instance_id as "targetConnectorId",
      target.resource_kind as "targetKind",target.status as "targetStatus",target.remote_version as "targetRemoteVersion",
      case when target.canonical_target_type='asset' then target.canonical_target_id end as "assetId",
      asset.status as "assetStatus",
      coalesce((select count(*)::int from connector_asset_access_refs ref
        where ref.project_id=adapter.project_id and ref.connector_instance_id=adapter.id
          and ref.remote_resource_id=target.id),0) as "dependentReferenceCount"
     from device_adapters adapter
     join team_members member on member.team_id=adapter.team_id and member.user_id=$4
     left join connector_remote_resources target on target.id=$5 and target.project_id=adapter.project_id
       and target.connector_instance_id=adapter.id
     left join assets asset on target.canonical_target_type='asset' and asset.project_id=target.project_id
       and asset.id::text=target.canonical_target_id
     where adapter.id=$2 and adapter.project_id=$1 and adapter.team_id=$3`,
    [projectId,connectorInstanceId,access.teamId,user.id,targetResourceId]
  );
  const row = result.rows[0];
  const policy = MODEL_DELETE_POLICY[action];
  if (!row || !new Set(["owner", "admin"]).has(row.role) || row.targetKind !== policy.targetKind) {
    throw new Error("FLIGHTHUB_MODEL_DELETE_SCOPE_MISMATCH");
  }
  const preview = previewFromTarget(targetResourceId, row);
  return { preview, previewDigest: auditHash(preview), approval: {
    resourceType: "connector_remote_resource", resourceId: String(targetResourceId), action: policy.approvalAction,
    context: { previewDigest: auditHash(preview), expectedRemoteVersion: preview.remoteVersion }
  } };
}

async function loadAuthorization(client: PoolClient, projectId: number, teamId: number, userId: number,
  input: FlightHubModelDeleteInput): Promise<FlightHubModelDeleteAuthorization> {
  const target = await loadTarget(client, projectId, teamId, userId, input.connectorInstanceId, input.targetResourceId);
  const policy = MODEL_DELETE_POLICY[input.action];
  const preview = previewFromTarget(input.targetResourceId, target);
  const gate = (await client.query<{
    actionEnabled: boolean; capabilityFieldVerified: boolean; approvalProjectId: number | null;
    approvalTeamId: number | null; approvalResourceType: string | null; approvalResourceId: string | null;
    approvalAction: string | null; approvalStatus: string | null; approvalUnexpired: boolean;
    approvalPreviewDigest: string | null; approvalRemoteVersion: string | null;
  }>(`select
      coalesce(flags.flighthub_action_flags_json @> jsonb_build_object($4::text,true),false) as "actionEnabled",
      exists(select 1 from connector_capability_snapshots capability
        where capability.project_id=adapter.project_id and capability.connector_instance_id=adapter.id
          and capability.capability_code=$3 and capability.status='supported'
          and capability.evidence_level='field-write' and (capability.expires_at is null or capability.expires_at>now())
          and capability.device_model is null and capability.firmware_version is null) as "capabilityFieldVerified",
      approval.project_id as "approvalProjectId",approval.team_id as "approvalTeamId",
      approval.resource_type as "approvalResourceType",approval.resource_id as "approvalResourceId",
      approval.action as "approvalAction",approval.status as "approvalStatus",
      coalesce(approval.expires_at>now(),false) as "approvalUnexpired",
      approval.context_json->>'previewDigest' as "approvalPreviewDigest",
      approval.context_json->>'expectedRemoteVersion' as "approvalRemoteVersion"
     from device_adapters adapter
     left join project_feature_flags flags on flags.project_id=adapter.project_id
     left join approval_requests approval on approval.id=$5::uuid and approval.project_id=adapter.project_id
     where adapter.id=$2 and adapter.project_id=$1`,
  [projectId,input.connectorInstanceId,policy.capability,policy.featureFlag,input.approvalRequestId])).rows[0];
  if (!gate) throw new Error("FLIGHTHUB_MODEL_DELETE_SCOPE_MISMATCH");
  return { ...target, ...gate, currentPreviewDigest: auditHash(preview) };
}

export async function submitFlightHubModelDelete(projectId: number, rawInput: unknown, requestId?: string | null) {
  const input = flightHubModelDeleteInputSchema.parse(rawInput);
  const policy = MODEL_DELETE_POLICY[input.action];
  const { user, access } = await requireCurrentProjectPermission(projectId, "project:view");
  const jobId = randomUUID();
  const requestDigest = auditHash({ action: input.action, connectorInstanceId: input.connectorInstanceId,
    targetResourceId: input.targetResourceId, approvalRequestId: input.approvalRequestId,
    expectedRemoteVersion: input.expectedRemoteVersion, previewDigest: input.previewDigest,
    request: { confirmation: true } });
  const envelope = encryptCredentialObject(input.request, getWebRuntimeConfig().authSecret,
    credentialAAD("flighthub-model-delete", jobId, projectId));
  return withAuditedProjectWrite({ projectId, teamId: access.teamId, actorUserId: user.id,
    requestId: correlationId(requestId), idempotencyKey: input.idempotencyKey,
    action: `connector.${input.action}`, resourceType: "connector_remote_resource",
    resourceId: String(input.targetResourceId), input: { action: input.action,
      connectorInstanceId: input.connectorInstanceId, targetResourceId: input.targetResourceId,
      approvalRequestId: input.approvalRequestId, expectedRemoteVersion: input.expectedRemoteVersion,
      previewDigest: input.previewDigest, request: { confirmed: true } },
    policyResult: { permission: "project:admin", capability: policy.capability,
      featureFlag: policy.featureFlag, evidence: "field-write", approval: "approved", completion: "worker-final" }
  }, async (client) => {
    const authorization = await loadAuthorization(client, projectId, access.teamId, user.id, input);
    const plan = authorizeFlightHubModelDelete(projectId, input, authorization);
    const inserted = await client.query<{ id: string; status: string }>(
      `insert into connector_model_delete_jobs(id,project_id,team_id,connector_instance_id,target_resource_id,
        approval_request_id,requested_by_user_id,action_kind,capability_code,feature_flag,idempotency_key,
        expected_remote_version,preview_digest,request_digest,request_envelope_json)
       values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
       on conflict do nothing returning id::text,status`,
      [jobId,projectId,access.teamId,input.connectorInstanceId,input.targetResourceId,input.approvalRequestId,user.id,
        input.action,plan.capability,plan.featureFlag,input.idempotencyKey,input.expectedRemoteVersion,input.previewDigest,
        requestDigest,envelope]
    );
    let job = inserted.rows[0];
    let reused = false;
    if (!job) {
      const existing = (await client.query<{ id: string; status: string; requestDigest: string;
        requestedByUserId: number }>(`select id::text,status,request_digest as "requestDigest",
          requested_by_user_id as "requestedByUserId" from connector_model_delete_jobs
         where project_id=$1 and connector_instance_id=$2 and action_kind=$3 and idempotency_key=$4`,
      [projectId,input.connectorInstanceId,input.action,input.idempotencyKey])).rows[0];
      if (!existing || existing.requestDigest !== requestDigest || existing.requestedByUserId !== user.id) {
        throw new Error("IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST");
      }
      job = { id: existing.id, status: existing.status };
      reused = true;
    }
    await client.query(`insert into outbox_events(project_id,team_id,event_id,event_type,aggregate_type,aggregate_id,payload_json,max_attempts)
      values($1,$2,$3,'flighthub.model_delete.requested','connector_model_delete_job',$4,$5,8)
      on conflict(event_id) do nothing`, [projectId,access.teamId,`flighthub-model-delete:${job.id}`,job.id,{ jobId: job.id }]);
    return { id: job.id, status: job.status, reused, completion: plan.completion };
  });
}

export async function readFlightHubModelDeleteJob(projectId: number, connectorInstanceId: number, jobId: string) {
  await requireCurrentProjectPermission(projectId, "project:view");
  const result = await query(`select id::text,action_kind as action,capability_code as "capabilityCode",
    target_resource_id as "targetResourceId",approval_request_id::text as "approvalRequestId",
    expected_remote_version as "expectedRemoteVersion",preview_digest as "previewDigest",status,
    attempt_count as "attemptCount",reconciliation_count as "reconciliationCount",last_error_code as "lastErrorCode",
    result_json as result,attempted_at as "attemptedAt",unknown_at as "unknownAt",completed_at as "completedAt",
    created_at as "createdAt",updated_at as "updatedAt"
   from connector_model_delete_jobs where id=$1::uuid and project_id=$2 and connector_instance_id=$3`,
  [jobId,projectId,connectorInstanceId]);
  if (!result.rows[0]) throw new Error("FLIGHTHUB_MODEL_DELETE_NOT_FOUND");
  return result.rows[0];
}
