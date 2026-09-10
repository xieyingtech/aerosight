import "server-only";

import { randomUUID } from "node:crypto";
import type { PoolClient } from "pg";

import { withAuditedProjectWrite } from "@/lib/audit";
import { auditHash } from "@/lib/audit-boundary";
import { credentialAAD, encryptCredentialObject } from "@/lib/credential-encryption";
import { requireCurrentProjectPermission } from "@/lib/data";
import { authorizeFlightHubLiveAction, flightHubLiveActionInputSchema, LIVE_ACTION_POLICY,
  type FlightHubLiveActionAuthorization, type FlightHubLiveActionInput } from "@/lib/dji-flighthub-live-actions-core";
import { query } from "@/lib/db";
import { correlationId } from "@/lib/observability";
import { getWebRuntimeConfig } from "@/lib/runtime-config";

function actionTargets(input: FlightHubLiveActionInput) {
  return "deviceId" in input
    ? { deviceId: input.deviceId, targetResourceId: null }
    : { deviceId: null, targetResourceId: input.targetResourceId };
}

async function loadAuthorization(client: PoolClient, projectId: number, teamId: number, userId: number,
  input: FlightHubLiveActionInput): Promise<FlightHubLiveActionAuthorization> {
  const policy = LIVE_ACTION_POLICY[input.action];
  const target = actionTargets(input);
  const result = await client.query<FlightHubLiveActionAuthorization>(
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
          and (($9='live-quality-set' and capability.device_model=device.device_model
                and capability.firmware_version=device.firmware_version)
            or ($9<>'live-quality-set' and capability.device_model is null and capability.firmware_version is null))) as "capabilityFieldVerified",
      device.project_id as "deviceProjectId",identity.id is not null as "deviceConnectorIdentityPresent",
      target.project_id as "targetProjectId",target.connector_instance_id as "targetConnectorId",
      target.resource_kind as "targetKind",target.status as "targetStatus"
     from device_adapters adapter
     join team_members member on member.team_id=adapter.team_id and member.user_id=$4
     left join project_feature_flags flags on flags.project_id=adapter.project_id
     left join devices device on device.id=$5 and device.project_id=adapter.project_id
     left join device_external_identities identity on identity.project_id=adapter.project_id and identity.adapter_id=adapter.id
       and identity.device_id=device.id
     left join connector_remote_resources target on target.id=$8 and target.project_id=adapter.project_id
     where adapter.id=$2 and adapter.project_id=$1 and adapter.team_id=$3`,
    [projectId,input.connectorInstanceId,teamId,userId,target.deviceId,policy.capability,policy.featureFlag,target.targetResourceId,input.action]
  );
  if (!result.rows[0]) throw new Error("FLIGHTHUB_LIVE_ACTION_SCOPE_MISMATCH");
  return result.rows[0];
}

export async function submitFlightHubLiveAction(projectId: number, rawInput: unknown, requestId?: string | null) {
  const input = flightHubLiveActionInputSchema.parse(rawInput);
  const { user, access } = await requireCurrentProjectPermission(projectId, "mission:operate");
  const policy = LIVE_ACTION_POLICY[input.action];
  const targets = actionTargets(input);
  const jobId = randomUUID();
  const requestDigest = auditHash({ action: input.action, connectorInstanceId: input.connectorInstanceId,
    ...targets, request: input.request });
  const envelope = encryptCredentialObject(input.request, getWebRuntimeConfig().authSecret,
    credentialAAD("flighthub-live-action", jobId, projectId));
  const resourceType = targets.deviceId ? "device" : "connector_remote_resource";
  const resourceId = String(targets.deviceId ?? targets.targetResourceId);
  return withAuditedProjectWrite({
    projectId, teamId: access.teamId, actorUserId: user.id, requestId: correlationId(requestId),
    idempotencyKey: input.idempotencyKey, action: `connector.${input.action}`, resourceType, resourceId,
    input: { action: input.action, connectorInstanceId: input.connectorInstanceId, ...targets, request: { digest: requestDigest } },
    policyResult: { permission: policy.ownerOnly ? "project:admin" : "mission:operate", capability: policy.capability,
      featureFlag: policy.featureFlag, evidence: "field-write", completion: "worker-final" }
  }, async (client) => {
    const authorization = await loadAuthorization(client, projectId, access.teamId, user.id, input);
    const plan = authorizeFlightHubLiveAction(projectId, input, authorization);
    const inserted = await client.query<{ id: string; status: string }>(
      `insert into connector_live_action_jobs(
        id,project_id,team_id,connector_instance_id,device_id,target_resource_id,requested_by_user_id,
        action_kind,capability_code,feature_flag,idempotency_key,request_digest,request_envelope_json
      ) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
      on conflict(project_id,connector_instance_id,action_kind,idempotency_key) do nothing returning id::text,status`,
      [jobId,projectId,access.teamId,input.connectorInstanceId,targets.deviceId,targets.targetResourceId,user.id,
        input.action,plan.capability,plan.featureFlag,input.idempotencyKey,requestDigest,envelope]
    );
    let job = inserted.rows[0];
    let reused = false;
    if (!job) {
      const existing = (await client.query<{ id: string; status: string; requestDigest: string; deviceId: number | null;
        targetResourceId: number | null; requestedByUserId: number }>(
        `select id::text,status,request_digest as "requestDigest",device_id as "deviceId",
          target_resource_id as "targetResourceId",requested_by_user_id as "requestedByUserId"
         from connector_live_action_jobs
         where project_id=$1 and connector_instance_id=$2 and action_kind=$3 and idempotency_key=$4`,
        [projectId,input.connectorInstanceId,input.action,input.idempotencyKey]
      )).rows[0];
      if (!existing || existing.requestDigest !== requestDigest || existing.deviceId !== targets.deviceId
        || existing.targetResourceId !== targets.targetResourceId || existing.requestedByUserId !== user.id) {
        throw new Error("IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST");
      }
      job = { id: existing.id, status: existing.status };
      reused = true;
    }
    await client.query(
      `insert into outbox_events(project_id,team_id,event_id,event_type,aggregate_type,aggregate_id,payload_json,max_attempts)
       values($1,$2,$3,'flighthub.live_action.requested','connector_live_action_job',$4,$5,8)
       on conflict(event_id) do nothing`,
      [projectId,access.teamId,`flighthub-live-action:${job.id}`,job.id,{ jobId: job.id }]
    );
    return { id: job.id, status: job.status, reused, completion: plan.completion };
  });
}

export async function readFlightHubLiveActionJob(projectId: number, connectorInstanceId: number, jobId: string) {
  await requireCurrentProjectPermission(projectId, "project:view");
  const result = await query(
    `select id::text,action_kind as action,capability_code as "capabilityCode",status,attempt_count as "attemptCount",
      last_error_code as "lastErrorCode",result_json as result,attempted_at as "attemptedAt",unknown_at as "unknownAt",
      completed_at as "completedAt",created_at as "createdAt",updated_at as "updatedAt"
     from connector_live_action_jobs where id=$1::uuid and project_id=$2 and connector_instance_id=$3`,
    [jobId,projectId,connectorInstanceId]
  );
  if (!result.rows[0]) throw new Error("FLIGHTHUB_LIVE_ACTION_NOT_FOUND");
  return result.rows[0];
}
