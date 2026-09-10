import "server-only";

import { randomUUID } from "node:crypto";
import type { PoolClient } from "pg";

import { withAuditedProjectWrite } from "@/lib/audit";
import { auditHash } from "@/lib/audit-boundary";
import { credentialAAD, encryptCredentialObject } from "@/lib/credential-encryption";
import { requireCurrentProjectPermission } from "@/lib/data";
import {
  authorizeFlightHubAction,
  flightHubActionInputSchema,
  type FlightHubActionAuthorization,
  type FlightHubActionInput
} from "@/lib/dji-flighthub-flight-actions-core";
import { correlationId } from "@/lib/observability";
import { getWebRuntimeConfig } from "@/lib/runtime-config";

type AuthorizationRow = FlightHubActionAuthorization;

function actionResourceIds(input: FlightHubActionInput) {
  return input.action === "flight-task-create"
    ? { waylineResourceId: input.waylineResourceId, targetResourceId: null }
    : { waylineResourceId: null, targetResourceId: input.targetResourceId };
}

async function loadAuthorization(
  client: PoolClient,
  projectId: number,
  teamId: number,
  userId: number,
  input: FlightHubActionInput
) {
  const resources = actionResourceIds(input);
  const result = await client.query<AuthorizationRow>(
    `select (member.role in('owner','admin') or exists(select 1 from project_permissions permission
        where permission.project_id=run.project_id and permission.team_id=run.team_id
          and permission.user_id=member.user_id and permission.permission='mission:operate')) as "hasPermission",$3::int as "teamId",
      adapter.project_id as "connectorProjectId",adapter.team_id as "connectorTeamId",adapter.status as "connectorStatus",
      coalesce(flags.flighthub_action_flags_json @> '{"flight.execute":true}'::jsonb,false) as "actionEnabled",
      exists(select 1 from connector_capability_snapshots capability
        where capability.project_id=adapter.project_id and capability.connector_instance_id=adapter.id
          and capability.capability_code='flight.execute' and capability.status='supported'
		  and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
		  and capability.region='cn' and capability.deployment='cn-public-cloud'
		  and capability.device_model=device.device_model and capability.firmware_version=device.firmware_version
          and capability.evidence_level='field-write' and (capability.expires_at is null or capability.expires_at>now())) as "capabilityFieldVerified",
      run.project_id as "taskRunProjectId",run.team_id as "taskRunTeamId",run.status as "taskRunStatus",
      run.selected_device_id as "selectedDeviceId",run.safety_policy_version_id as "safetyPolicyVersionId",
      coalesce((run.preflight_snapshot_json->>'allowed')::boolean,false) as "preflightAllowed",
      identity.id is not null as "deviceIdentityPresent",
      approval.project_id as "approvalProjectId",approval.team_id as "approvalTeamId",approval.status as "approvalStatus",
      approval.resource_type as "approvalResourceType",approval.resource_id as "approvalResourceId",approval.action as "approvalAction",
      coalesce(approval.expires_at>now(),false) as "approvalUnexpired",
      coalesce((approval.context_json#>>'{preflight,allowed}')::boolean,false) as "approvalPreflightAllowed",
      wayline.project_id as "waylineProjectId",wayline.connector_instance_id as "waylineConnectorId",wayline.resource_kind as "waylineKind",
      target.project_id as "targetProjectId",target.connector_instance_id as "targetConnectorId",target.resource_kind as "targetKind",
      target.canonical_target_id as "targetTaskRunId"
    from task_runs run
    join device_adapters adapter on adapter.id=$4 and adapter.project_id=run.project_id
	join devices device on device.id=run.selected_device_id and device.project_id=run.project_id
    join team_members member on member.team_id=run.team_id and member.user_id=$8
    left join project_feature_flags flags on flags.project_id=run.project_id
    left join device_external_identities identity on identity.project_id=run.project_id
      and identity.adapter_id=adapter.id and identity.device_id=run.selected_device_id
    left join approval_requests approval on approval.id=$5 and approval.project_id=run.project_id
    left join connector_remote_resources wayline on wayline.id=$6 and wayline.project_id=run.project_id
      and wayline.status='active'
    left join connector_remote_resources target on target.id=$7 and target.project_id=run.project_id
      and target.status='active'
    where run.project_id=$1 and run.id=$2`,
    [projectId, input.taskRunId, teamId, input.connectorInstanceId, input.approvalRequestId,
      resources.waylineResourceId, resources.targetResourceId, userId]
  );
  if (!result.rows[0]) throw new Error("FLIGHTHUB_ACTION_SCOPE_MISMATCH");
  if (input.action === "flight-task-create" && input.request.landingDeviceId) {
    const landing = await client.query(
      `select 1 from device_external_identities
        where project_id=$1 and adapter_id=$2 and device_id=$3`,
      [projectId, input.connectorInstanceId, input.request.landingDeviceId]
    );
    if (!landing.rows[0]) throw new Error("FLIGHTHUB_ACTION_LANDING_DEVICE_SCOPE_MISMATCH");
  }
  return result.rows[0];
}

export async function submitFlightHubAction(projectId: number, rawInput: unknown, requestId?: string | null) {
  const input = flightHubActionInputSchema.parse(rawInput);
  const { user, access } = await requireCurrentProjectPermission(projectId, "mission:operate");
  const jobId = randomUUID();
  const requestDigest = auditHash({ action: input.action, taskRunId: input.taskRunId,
    connectorInstanceId: input.connectorInstanceId, ...actionResourceIds(input), request: input.request });
  const envelope = encryptCredentialObject(input.request, getWebRuntimeConfig().authSecret,
    credentialAAD("flighthub-flight-action", jobId, projectId));
  return withAuditedProjectWrite({
    projectId, teamId: access.teamId, actorUserId: user.id, requestId: correlationId(requestId),
    idempotencyKey: input.idempotencyKey, action: `connector.${input.action}`, resourceType: "task_run",
    resourceId: String(input.taskRunId), input: { ...input, request: { digest: requestDigest } },
    policyResult: { permission: "mission:operate", capability: "flight.execute", approval: input.approvalRequestId,
      completion: "await-remote-reconciliation" }
  }, async (client) => {
    const authorization = await loadAuthorization(client, projectId, access.teamId, user.id, input);
    const plan = authorizeFlightHubAction(projectId, input, authorization);
    const resources = actionResourceIds(input);
    const inserted = await client.query<{ id: string; status: string }>(
      `insert into connector_action_jobs(
        id,project_id,team_id,connector_instance_id,task_run_id,device_id,wayline_resource_id,target_resource_id,
        approval_request_id,requested_by_user_id,action_kind,idempotency_key,request_digest,request_envelope_json
      ) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
      on conflict(project_id,connector_instance_id,action_kind,idempotency_key) do nothing
      returning id::text,status`,
      [jobId,projectId,access.teamId,input.connectorInstanceId,input.taskRunId,plan.deviceId,
        resources.waylineResourceId,resources.targetResourceId,input.approvalRequestId,user.id,input.action,
        input.idempotencyKey,requestDigest,envelope]
    );
    let job = inserted.rows[0];
    let reused = false;
    if (!job) {
      const existing = (await client.query<{
        id: string; status: string; requestDigest: string; taskRunId: number; deviceId: number;
        approvalRequestId: string; requestedByUserId: number; waylineResourceId: number | null; targetResourceId: number | null;
      }>(`select id::text,status,request_digest as "requestDigest",task_run_id as "taskRunId",device_id as "deviceId",
          approval_request_id::text as "approvalRequestId",requested_by_user_id as "requestedByUserId",
          wayline_resource_id as "waylineResourceId",target_resource_id as "targetResourceId"
        from connector_action_jobs where project_id=$1 and connector_instance_id=$2 and action_kind=$3 and idempotency_key=$4`,
        [projectId,input.connectorInstanceId,input.action,input.idempotencyKey])).rows[0];
      if (!existing || existing.requestDigest !== requestDigest || existing.taskRunId !== input.taskRunId
        || existing.deviceId !== plan.deviceId || existing.approvalRequestId !== input.approvalRequestId
        || existing.requestedByUserId !== user.id || existing.waylineResourceId !== resources.waylineResourceId
        || existing.targetResourceId !== resources.targetResourceId) throw new Error("IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST");
      job = { id: existing.id, status: existing.status };
      reused = true;
    }
    await client.query(
      `insert into outbox_events(project_id,team_id,event_id,event_type,aggregate_type,aggregate_id,payload_json,max_attempts)
       values($1,$2,$3,'flighthub.flight_action.requested','connector_action_job',$4,$5,16)
       on conflict(event_id) do nothing`,
      [projectId,access.teamId,`flighthub-flight-action:${job.id}`,job.id,{ jobId: job.id }]
    );
    return { id: job.id, status: job.status, reused, completion: plan.completion };
  });
}

export async function readFlightHubActionJob(projectId: number, connectorInstanceId: number, jobId: string) {
  await requireCurrentProjectPermission(projectId, "project:view");
  const { query } = await import("@/lib/db");
  const job = (await query<{
    id: string; taskRunId: number; action: string; status: string; attemptCount: number;
    reconciliationCount: number; lastErrorCode: string | null; acceptedAt: Date | null;
    reconciledAt: Date | null; unknownAt: Date | null; completedAt: Date | null; createdAt: Date;
  }>(`select id::text,task_run_id as "taskRunId",action_kind as action,status,
      attempt_count as "attemptCount",reconciliation_count as "reconciliationCount",
      last_error_code as "lastErrorCode",accepted_at as "acceptedAt",reconciled_at as "reconciledAt",
      unknown_at as "unknownAt",completed_at as "completedAt",created_at as "createdAt"
    from connector_action_jobs where id=$1::uuid and project_id=$2 and connector_instance_id=$3`,
    [jobId,projectId,connectorInstanceId])).rows[0];
  if (!job) throw new Error("FLIGHTHUB_ACTION_NOT_FOUND");
  return job;
}
