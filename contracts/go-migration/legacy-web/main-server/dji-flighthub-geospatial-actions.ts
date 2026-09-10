import "server-only";

import { randomUUID } from "node:crypto";
import type { PoolClient } from "pg";

import { withAuditedProjectWrite } from "@/lib/audit";
import { auditHash } from "@/lib/audit-boundary";
import { credentialAAD, encryptCredentialObject } from "@/lib/credential-encryption";
import { requireCurrentProjectPermission } from "@/lib/data";
import { authorizeFlightHubGeospatialAction, flightHubGeospatialActionInputSchema, GEOSPATIAL_ACTION_POLICY,
  type FlightHubGeospatialActionAuthorization, type FlightHubGeospatialActionInput } from "@/lib/dji-flighthub-geospatial-actions-core";
import { query } from "@/lib/db";
import { correlationId } from "@/lib/observability";
import { getWebRuntimeConfig } from "@/lib/runtime-config";

async function loadAuthorization(client: PoolClient, projectId: number, teamId: number, userId: number,
  input: FlightHubGeospatialActionInput): Promise<FlightHubGeospatialActionAuthorization> {
  const policy = GEOSPATIAL_ACTION_POLICY[input.action];
  const targetResourceId = "targetResourceId" in input ? input.targetResourceId : null;
  const result = await client.query<FlightHubGeospatialActionAuthorization>(
    `select $3::int as "teamId",member.role,
      (member.role in('owner','admin') or exists(select 1 from project_permissions permission
        where permission.project_id=adapter.project_id and permission.team_id=adapter.team_id
          and permission.user_id=member.user_id and permission.permission='mission:operate')) as "hasOperatePermission",
      adapter.project_id as "connectorProjectId",adapter.team_id as "connectorTeamId",adapter.status as "connectorStatus",
      coalesce(flags.flighthub_action_flags_json @> jsonb_build_object($7::text,true),false) as "actionEnabled",
      exists(select 1 from connector_capability_snapshots capability
        where capability.project_id=adapter.project_id and capability.connector_instance_id=adapter.id
          and capability.capability_code=$6 and capability.status='supported'
		  and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
		  and capability.region='cn' and capability.deployment='cn-public-cloud'
          and capability.evidence_level='field-write' and (capability.expires_at is null or capability.expires_at>now())
          and capability.device_model is null and capability.firmware_version is null) as "capabilityFieldVerified",
      target.project_id as "targetProjectId",target.connector_instance_id as "targetConnectorId",
      target.resource_kind as "targetKind",target.status as "targetStatus",target.remote_version as "targetRemoteVersion"
     from device_adapters adapter
     join team_members member on member.team_id=adapter.team_id and member.user_id=$4
     left join project_feature_flags flags on flags.project_id=adapter.project_id
     left join connector_remote_resources target on target.id=$5 and target.project_id=adapter.project_id
       and target.connector_instance_id=adapter.id
     where adapter.id=$2 and adapter.project_id=$1 and adapter.team_id=$3`,
    [projectId,input.connectorInstanceId,teamId,userId,targetResourceId,policy.capability,policy.featureFlag]
  );
  if (!result.rows[0]) throw new Error("FLIGHTHUB_GEOSPATIAL_ACTION_SCOPE_MISMATCH");
  return result.rows[0];
}

export async function submitFlightHubGeospatialAction(projectId: number, rawInput: unknown, requestId?: string | null) {
  const input = flightHubGeospatialActionInputSchema.parse(rawInput);
  const policy = GEOSPATIAL_ACTION_POLICY[input.action];
  const { user, access } = await requireCurrentProjectPermission(projectId, "mission:operate");
  const targetResourceId = "targetResourceId" in input ? input.targetResourceId : null;
  const expectedRemoteVersion = "expectedRemoteVersion" in input ? input.expectedRemoteVersion : null;
  const jobId = randomUUID();
  const requestDigest = auditHash({ action: input.action, connectorInstanceId: input.connectorInstanceId,
    targetResourceId, expectedRemoteVersion, request: input.request });
  const envelope = encryptCredentialObject(input.request, getWebRuntimeConfig().authSecret,
    credentialAAD("flighthub-geospatial-action", jobId, projectId));
  return withAuditedProjectWrite({
    projectId, teamId: access.teamId, actorUserId: user.id, requestId: correlationId(requestId),
    idempotencyKey: input.idempotencyKey, action: `connector.${input.action}`, resourceType: "connector_remote_resource",
    resourceId: targetResourceId ? String(targetResourceId) : undefined,
    input: { action: input.action, connectorInstanceId: input.connectorInstanceId, targetResourceId,
      expectedRemoteVersion, request: { digest: requestDigest } },
    policyResult: { permission: policy.permission, capability: policy.capability,
      featureFlag: policy.featureFlag, evidence: "field-write", completion: "worker-final", concurrency: "remote-version" }
  }, async (client) => {
    const authorization = await loadAuthorization(client, projectId, access.teamId, user.id, input);
    const plan = authorizeFlightHubGeospatialAction(projectId, input, authorization);
    const inserted = await client.query<{ id: string; status: string }>(
      `insert into connector_geospatial_action_jobs(
        id,project_id,team_id,connector_instance_id,target_resource_id,requested_by_user_id,
        action_kind,capability_code,feature_flag,idempotency_key,expected_remote_version,request_digest,request_envelope_json
      ) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
      on conflict do nothing returning id::text,status`,
      [jobId,projectId,access.teamId,input.connectorInstanceId,targetResourceId,user.id,input.action,
        plan.capability,plan.featureFlag,input.idempotencyKey,expectedRemoteVersion,requestDigest,envelope]
    );
    let job = inserted.rows[0];
    let reused = false;
    if (!job) {
      const existing = (await client.query<{ id: string; status: string; requestDigest: string;
        targetResourceId: number | null; expectedRemoteVersion: string | null; requestedByUserId: number }>(
        `select id::text,status,request_digest as "requestDigest",target_resource_id as "targetResourceId",
          expected_remote_version as "expectedRemoteVersion",requested_by_user_id as "requestedByUserId"
         from connector_geospatial_action_jobs
         where project_id=$1 and connector_instance_id=$2 and action_kind=$3 and idempotency_key=$4`,
        [projectId,input.connectorInstanceId,input.action,input.idempotencyKey]
      )).rows[0];
      if (!existing) throw new Error("FLIGHTHUB_GEOSPATIAL_ACTION_VERSION_CONFLICT");
      if (existing.requestDigest !== requestDigest || existing.targetResourceId !== targetResourceId
        || existing.expectedRemoteVersion !== expectedRemoteVersion || existing.requestedByUserId !== user.id) {
        throw new Error("IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST");
      }
      job = { id: existing.id, status: existing.status };
      reused = true;
    }
    await client.query(
      `insert into outbox_events(project_id,team_id,event_id,event_type,aggregate_type,aggregate_id,payload_json,max_attempts)
       values($1,$2,$3,'flighthub.geospatial_action.requested','connector_geospatial_action_job',$4,$5,8)
       on conflict(event_id) do nothing`,
      [projectId,access.teamId,`flighthub-geospatial-action:${job.id}`,job.id,{ jobId: job.id }]
    );
    return { id: job.id, status: job.status, reused, completion: plan.completion };
  });
}

export async function readFlightHubGeospatialActionJob(projectId: number, connectorInstanceId: number, jobId: string) {
  await requireCurrentProjectPermission(projectId, "project:view");
  const result = await query(
    `select id::text,action_kind as action,capability_code as "capabilityCode",target_resource_id as "targetResourceId",
      expected_remote_version as "expectedRemoteVersion",status,attempt_count as "attemptCount",last_error_code as "lastErrorCode",
      result_json as result,attempted_at as "attemptedAt",unknown_at as "unknownAt",completed_at as "completedAt",
      created_at as "createdAt",updated_at as "updatedAt"
     from connector_geospatial_action_jobs where id=$1::uuid and project_id=$2 and connector_instance_id=$3`,
    [jobId,projectId,connectorInstanceId]
  );
  if (!result.rows[0]) throw new Error("FLIGHTHUB_GEOSPATIAL_ACTION_NOT_FOUND");
  return result.rows[0];
}
