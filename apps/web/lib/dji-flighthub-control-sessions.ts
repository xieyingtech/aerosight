import "server-only";

import { randomUUID } from "node:crypto";

import { withAuditedProjectWrite } from "@/lib/audit";
import { requireCurrentProjectPermission } from "@/lib/data";
import { CONTROL_LEASE_MILLISECONDS, CONTROL_OPERATIONS_PER_SECOND, CONTROL_SESSION_MILLISECONDS, authorizeControlSession,
  nextControlLease, normalizeControlSelection } from "@/lib/dji-flighthub-control-session-core";
import { query } from "@/lib/db";
import { correlationId } from "@/lib/observability";

export type CreateFlightHubControlSessionInput = {
  projectId: number;
  deviceId: number;
  connectorInstanceId: number;
  controls: unknown;
  approvalRequestId: string;
  safetyPolicyVersionId: number;
  idempotencyKey: string;
  requestId?: string | null;
};

export async function createFlightHubControlSession(input: CreateFlightHubControlSessionInput) {
  const controls = normalizeControlSelection(input.controls);
  const approvalRequestId = input.approvalRequestId.toLowerCase();
  const { user, access } = await requireCurrentProjectPermission(input.projectId, "mission:operate");
  const sessionId = randomUUID();
  return withAuditedProjectWrite({
    projectId: input.projectId, teamId: access.teamId, actorUserId: user.id,
    requestId: correlationId(input.requestId), idempotencyKey: input.idempotencyKey,
    action: "flighthub.control_session.acquire", resourceType: "device", resourceId: String(input.deviceId),
    input: { connectorInstanceId: input.connectorInstanceId, controls, approvalRequestId,
      safetyPolicyVersionId: input.safetyPolicyVersionId },
    policyResult: { capability: "device.control", featureFlag: "device.control", evidence: "field-write",
      leaseSeconds: CONTROL_LEASE_MILLISECONDS / 1000, maximumSeconds: CONTROL_SESSION_MILLISECONDS / 1000 }
  }, async (client) => {
    const lockedDevice = await client.query(`select id from devices where project_id=$1 and id=$2 for update`,
      [input.projectId,input.deviceId]);
    if (!lockedDevice.rows[0]) throw new Error("FLIGHTHUB_CONTROL_SCOPE_MISMATCH");

    const existing = (await client.query<{ id: string; status: string; holderUserId: number; connectorInstanceId: string;
      approvalRequestId: string; safetyPolicyVersionId: string; controlsMatch: boolean }>(
      `select id::text,status,holder_user_id as "holderUserId",connector_instance_id::text as "connectorInstanceId",
        approval_request_id::text as "approvalRequestId",safety_policy_version_id::text as "safetyPolicyVersionId",
        controls_json=$4::jsonb as "controlsMatch" from connector_control_sessions
       where project_id=$1 and device_id=$2 and idempotency_key=$3`,
      [input.projectId,input.deviceId,input.idempotencyKey,JSON.stringify(controls)]
    )).rows[0];
    if (existing) {
      if (existing.holderUserId !== user.id || existing.connectorInstanceId !== String(input.connectorInstanceId)
          || existing.approvalRequestId !== approvalRequestId
          || existing.safetyPolicyVersionId !== String(input.safetyPolicyVersionId)
          || !existing.controlsMatch) {
        throw new Error("IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST");
      }
      return { id: existing.id, status: existing.status, reused: true };
    }

    const governance = (await client.query<{
      connectorProjectId: number; connectorTeamId: number; deviceProjectId: number; connectorStatus: string;
      featureEnabled: boolean; capabilityFieldVerified: boolean; deviceOnline: boolean; stateCapturedAt: Date | null;
      currentSafetyPolicyVersionId: string | null; approvalProjectId: number | null; approvalTeamId: number | null;
      approvalResourceType: string | null; approvalResourceId: string | null; approvalAction: string | null;
      approvalStatus: string | null; approvalUnexpired: boolean; conflictingSessionCount: number;
    }>(`select adapter.project_id as "connectorProjectId",adapter.team_id as "connectorTeamId",
        device.project_id as "deviceProjectId",adapter.status as "connectorStatus",
        coalesce(flags.flighthub_action_flags_json @> '{"device.control":true}'::jsonb,false) as "featureEnabled",
        exists(select 1 from connector_capability_snapshots capability where capability.project_id=adapter.project_id
          and capability.connector_instance_id=adapter.id and capability.capability_code='device.control'
          and capability.status='supported' and capability.evidence_level='field-write'
          and (capability.expires_at is null or capability.expires_at>now())) as "capabilityFieldVerified",
        device.status='online' as "deviceOnline",latest.captured_at as "stateCapturedAt",
        project.current_safety_policy_version_id::text as "currentSafetyPolicyVersionId",
        approval.project_id as "approvalProjectId",approval.team_id as "approvalTeamId",
        approval.resource_type as "approvalResourceType",approval.resource_id as "approvalResourceId",
        approval.action as "approvalAction",approval.status as "approvalStatus",
        coalesce(approval.expires_at>now(),false) as "approvalUnexpired",
        (select count(*)::int from connector_control_sessions session where session.project_id=device.project_id
          and session.device_id=device.id and session.status in('requested','acquiring','active','releasing')) as "conflictingSessionCount"
       from device_adapters adapter
       join projects project on project.id=adapter.project_id and project.team_id=adapter.team_id
       join devices device on device.project_id=adapter.project_id and device.id=$2
       join device_external_identities identity on identity.project_id=adapter.project_id and identity.adapter_id=adapter.id
         and identity.device_id=device.id
       left join project_feature_flags flags on flags.project_id=adapter.project_id
       left join device_latest_telemetry latest on latest.project_id=device.project_id and latest.device_id=device.id and latest.adapter_id=adapter.id
       left join approval_requests approval on approval.id::text=$4 and approval.project_id=adapter.project_id
       where adapter.project_id=$1 and adapter.id=$3`,
      [input.projectId,input.deviceId,input.connectorInstanceId,approvalRequestId])).rows[0];
    if (!governance) throw new Error("FLIGHTHUB_CONTROL_SCOPE_MISMATCH");
    const now = new Date();
    authorizeControlSession({
      projectId: input.projectId, teamId: access.teamId, deviceId: input.deviceId,
      connectorProjectId: governance.connectorProjectId, connectorTeamId: governance.connectorTeamId,
      deviceProjectId: governance.deviceProjectId, connectorStatus: governance.connectorStatus,
      featureEnabled: governance.featureEnabled, capabilityFieldVerified: governance.capabilityFieldVerified,
      deviceOnline: governance.deviceOnline, stateCapturedAt: governance.stateCapturedAt, now,
      requestedSafetyPolicyVersionId: input.safetyPolicyVersionId,
      currentSafetyPolicyVersionId: governance.currentSafetyPolicyVersionId ? Number(governance.currentSafetyPolicyVersionId) : null,
      approvalProjectId: governance.approvalProjectId, approvalTeamId: governance.approvalTeamId,
      approvalResourceType: governance.approvalResourceType, approvalResourceId: governance.approvalResourceId,
      approvalAction: governance.approvalAction, approvalStatus: governance.approvalStatus,
      approvalUnexpired: governance.approvalUnexpired, conflictingSessionCount: governance.conflictingSessionCount
    });
    const absoluteExpiresAt = new Date(now.getTime() + CONTROL_SESSION_MILLISECONDS);
    const leaseExpiresAt = nextControlLease(now, absoluteExpiresAt);
    const inserted = await client.query<{ id: string; status: string }>(
      `insert into connector_control_sessions(
        id,project_id,team_id,connector_instance_id,device_id,holder_user_id,approval_request_id,
        safety_policy_version_id,idempotency_key,controls_json,last_heartbeat_at,lease_expires_at,
        absolute_expires_at,operation_window_started_at
      ) values($1,$2,$3,$4,$5,$6,$7::uuid,$8,$9,$10::jsonb,$11,$12,$13,$11) returning id::text,status`,
      [sessionId,input.projectId,access.teamId,input.connectorInstanceId,input.deviceId,user.id,
        approvalRequestId,input.safetyPolicyVersionId,input.idempotencyKey,JSON.stringify(controls),
        now,leaseExpiresAt,absoluteExpiresAt]
    );
    await client.query(
      `insert into outbox_events(project_id,team_id,event_id,event_type,aggregate_type,aggregate_id,payload_json,max_attempts)
       values($1,$2,$3,'flighthub.control_session.reconcile','connector_control_session',$4,$5,8)
       on conflict(event_id) do nothing`,
      [input.projectId,access.teamId,`flighthub-control-session:${sessionId}:acquire`,sessionId,{ sessionId }]
    );
    return { ...inserted.rows[0], reused: false };
  });
}

export async function heartbeatFlightHubControlSession(projectId: number, deviceId: number, sessionId: string) {
  const { user } = await requireCurrentProjectPermission(projectId, "mission:operate");
  const now = new Date();
  const result = await query<{ id: string; status: string; leaseExpiresAt: Date }>(
    `update connector_control_sessions set last_heartbeat_at=$3,
      lease_expires_at=least(absolute_expires_at,$3+($4*interval '1 millisecond')),updated_at=now()
     where project_id=$1 and id=$2::uuid and device_id=$7 and holder_user_id=$5 and status='active'
       and lease_expires_at>$3 and absolute_expires_at>$3
       and last_heartbeat_at<=$3-($6*interval '1 millisecond')
     returning id::text,status,lease_expires_at as "leaseExpiresAt"`,
    [projectId,sessionId,now,CONTROL_LEASE_MILLISECONDS,user.id,500,deviceId]
  );
  if (!result.rows[0]) throw new Error("FLIGHTHUB_CONTROL_HEARTBEAT_REJECTED");
  return result.rows[0];
}

export async function claimFlightHubControlOperation(projectId: number, sessionId: string, deviceId: number) {
  const { user } = await requireCurrentProjectPermission(projectId, "mission:operate");
  const result = await query(
    `update connector_control_sessions set
      operation_count=case when operation_window_started_at<=now()-interval '1 second' then 1 else operation_count+1 end,
      operation_window_started_at=case when operation_window_started_at<=now()-interval '1 second' then now() else operation_window_started_at end,
      last_operation_at=now(),updated_at=now()
     where project_id=$1 and id=$2::uuid and device_id=$3 and holder_user_id=$4 and status='active'
       and lease_expires_at>now() and absolute_expires_at>now()
       and (operation_window_started_at<=now()-interval '1 second' or operation_count<$5)
     returning id`, [projectId,sessionId,deviceId,user.id,CONTROL_OPERATIONS_PER_SECOND]
  );
  if (!result.rows[0]) throw new Error("FLIGHTHUB_CONTROL_OPERATION_RATE_LIMITED");
  return true;
}

export async function releaseFlightHubControlSession(projectId: number, deviceId: number, sessionId: string, requestId?: string | null) {
  const { user, access } = await requireCurrentProjectPermission(projectId, "mission:operate");
  return withAuditedProjectWrite({
    projectId, teamId: access.teamId, actorUserId: user.id, requestId: correlationId(requestId),
    idempotencyKey: `${sessionId}:release`, action: "flighthub.control_session.release",
    resourceType: "connector_control_session", resourceId: sessionId,
    input: { sessionPresent: true }, policyResult: { holderRequired: true, remoteRelease: "single-attempt" }
  }, async (client) => {
    const session = (await client.query<{ status: string; holderUserId: number }>(
      `select status,holder_user_id as "holderUserId" from connector_control_sessions
       where project_id=$1 and device_id=$3 and id=$2::uuid for update`, [projectId,sessionId,deviceId]
    )).rows[0];
    if (!session || session.holderUserId !== user.id) throw new Error("FLIGHTHUB_CONTROL_SESSION_NOT_FOUND");
    if (["released","failed","expired"].includes(session.status)) return { id: sessionId, status: session.status, reused: true };
    await client.query(`update connector_control_sessions set status='releasing',release_requested_at=coalesce(release_requested_at,now()),updated_at=now()
      where project_id=$1 and id=$2::uuid`, [projectId,sessionId]);
    await client.query(
      `insert into outbox_events(project_id,team_id,event_id,event_type,aggregate_type,aggregate_id,payload_json,max_attempts)
       values($1,$2,$3,'flighthub.control_session.reconcile','connector_control_session',$4,$5,8)
       on conflict(event_id) do nothing`,
      [projectId,access.teamId,`flighthub-control-session:${sessionId}:release`,sessionId,{ sessionId }]
    );
    return { id: sessionId, status: "releasing", reused: session.status === "releasing" };
  });
}
