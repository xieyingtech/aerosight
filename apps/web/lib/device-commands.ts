import "server-only";

import { randomUUID } from "node:crypto";

import { withAuditedProjectWrite } from "@/lib/audit";
import { requireCurrentProjectPermission } from "@/lib/data";
import { actionPatternMatches, assertDeviceCommandSafety, authorizeCapabilityAction } from "@/lib/device-command-core";
import { authorizeFlightHubDiscreteCommand, flightHubDiscretePolicy, validateFlightHubCommandParameters } from "@/lib/dji-flighthub-device-command-core";
import { correlationId } from "@/lib/observability";
import { publishProjectEvent } from "@/lib/project-events";

type SubmitDeviceCommandInput = {
  projectId: number;
  deviceId: number;
  capabilityCode: string;
  commandKey: string;
  parameters: Record<string, unknown>;
  idempotencyKey: string;
  confirmation: string | null;
  reason: string;
  approvalRequestId?: string | null;
  safetyPolicyVersionId?: number | null;
  deadlineSeconds?: number;
  requestId?: string | null;
};

export async function submitDeviceCommand(input: SubmitDeviceCommandInput) {
  const { user, access } = await requireCurrentProjectPermission(input.projectId, "project:view");
  return withAuditedProjectWrite({
    projectId: input.projectId, teamId: access.teamId, requestId: correlationId(input.requestId),
    idempotencyKey: input.idempotencyKey, actorUserId: user.id,
    action: "device_command.submit", resourceType: "device", resourceId: String(input.deviceId),
    input: { deviceId: input.deviceId, capabilityCode: input.capabilityCode, commandKey: input.commandKey,
      parameters: input.parameters, reason: input.reason, confirmationPresent: Boolean(input.confirmation),
      approvalRequestId: input.approvalRequestId, safetyPolicyVersionId: input.safetyPolicyVersionId },
    policyResult: { boundary: "capability-rbac+safety-interlock+second-confirmation+connector-worker-recheck" }
  }, async (client) => {
    const target = await client.query<{
      projectId: number; deviceTypeId: string; deviceTypeKey: string; status: "online" | "degraded" | "offline" | "unknown" | "unavailable";
      availability: "available" | "degraded" | "unavailable"; riskLevel: "low" | "medium" | "high" | "critical";
    }>(`select device.project_id as "projectId",device.device_type_id as "deviceTypeId",device_type.type_key as "deviceTypeKey",device.status,
              capability.availability,capability.risk_level as "riskLevel"
         from devices device
         join device_types device_type on device_type.id=device.device_type_id
         join device_capabilities capability on capability.device_id=device.id and capability.project_id=device.project_id
        where device.project_id=$1 and device.id=$2 and capability.capability_code=$3
        for update of device`, [input.projectId, input.deviceId, input.capabilityCode]);
    const device = target.rows[0];
    if (!device) throw new Error("DEVICE_CAPABILITY_NOT_FOUND");

    const routes = await client.query<{ connectorInstanceId: string; connectorKey: string; connectorStatus: string; priority: number }>(
      `select binding.connector_instance_id::text as "connectorInstanceId",definition.connector_key as "connectorKey",
              adapter.status as "connectorStatus",binding.priority
         from device_connector_bindings binding
         join device_adapters adapter on adapter.id=binding.connector_instance_id and adapter.project_id=binding.project_id
         join connector_definitions definition on definition.id=adapter.connector_definition_id
        where binding.project_id=$1 and binding.device_id=$2 and binding.status='active'
        order by binding.priority desc,binding.connector_instance_id limit 2`, [input.projectId, input.deviceId]
    );
    if (!routes.rows[0] || (routes.rows[1] && routes.rows[1].priority === routes.rows[0].priority)) {
      throw new Error("DEVICE_COMMAND_ROUTE_UNAVAILABLE");
    }

    const grantRows = await client.query<{ actionPattern: string; effect: "allow" | "deny" }>(
      `select action_pattern as "actionPattern",effect from device_capability_grants
       where project_id=$1 and team_id=$2 and user_id=$3 and (expires_at is null or expires_at>now())
         and (scope_type='project' or (scope_type='device_type' and device_type_id=$4)
              or (scope_type='device' and device_id=$5))`,
      [input.projectId, access.teamId, user.id, device.deviceTypeId, input.deviceId]
    );
    authorizeCapabilityAction({ role: access.role, action: input.capabilityCode,
      grants: grantRows.rows.filter((grant) => actionPatternMatches(grant.actionPattern, input.capabilityCode)) });

    const existing = await client.query<{ id: string; status: string }>(
      `select id::text,status from device_commands
       where project_id=$1 and device_id=$2 and idempotency_key=$3 for update`,
      [input.projectId, input.deviceId, input.idempotencyKey]
    );
    if (existing.rows[0]) return { ...existing.rows[0], reused: true };

    const conflicts = await client.query<{ count: number }>(
      `select count(*)::int as count from task_runs run
       where run.project_id=$1 and run.status in ('dispatching','running','paused','canceling')
         and (run.selected_device_id=$2 or run.selected_device_id in (
           select relation.to_device_id from device_relationships relation
           where relation.project_id=$1 and relation.from_device_id=$2 and relation.valid_until is null
         ))`, [input.projectId, input.deviceId]
    );
    const safety = assertDeviceCommandSafety({
      requestProjectId: input.projectId, deviceProjectId: device.projectId, deviceId: input.deviceId,
      capabilityCode: input.capabilityCode, riskLevel: device.riskLevel,
      capabilityAvailability: device.availability, deviceStatus: device.status,
      activeTaskCount: conflicts.rows[0]?.count ?? 0, confirmation: input.confirmation
    });

    let flightHubSafety: Record<string, unknown> = {};
    if (routes.rows[0].connectorKey === "dji.flighthub2") {
      const policy = flightHubDiscretePolicy(input.commandKey);
      if (!policy || policy.capabilityCode !== input.capabilityCode) throw new Error("FLIGHTHUB_COMMAND_POLICY_MISMATCH");
      const governance = (await client.query<{
        featureEnabled: boolean; capabilityFieldVerified: boolean; stateCapturedAt: Date | null;
        currentSafetyPolicyVersionId: string | null; approvalProjectId: number | null; approvalTeamId: number | null;
        approvalResourceType: string | null; approvalResourceId: string | null; approvalAction: string | null;
        approvalStatus: string | null; approvalUnexpired: boolean;
      }>(`select coalesce(flags.flighthub_action_flags_json @> jsonb_build_object($5::text,true),false) as "featureEnabled",
          exists(select 1 from connector_capability_snapshots capability where capability.project_id=$1
            and capability.connector_instance_id=$3::bigint and capability.capability_code=$6
			and capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'
			and capability.region='cn' and capability.deployment='cn-public-cloud'
            and capability.status='supported' and capability.evidence_level='field-write'
            and capability.device_model=device.device_model and capability.firmware_version=device.firmware_version
            and (capability.expires_at is null or capability.expires_at>now())) as "capabilityFieldVerified",
          latest.captured_at as "stateCapturedAt",project.current_safety_policy_version_id::text as "currentSafetyPolicyVersionId",
          approval.project_id as "approvalProjectId",approval.team_id as "approvalTeamId",
          approval.resource_type as "approvalResourceType",approval.resource_id as "approvalResourceId",
          approval.action as "approvalAction",approval.status as "approvalStatus",
          coalesce(approval.expires_at>now(),false) as "approvalUnexpired"
        from projects project
        join devices device on device.project_id=project.id and device.id=$2
		join device_adapters adapter on adapter.id=$3::bigint and adapter.project_id=project.id
        left join project_feature_flags flags on flags.project_id=project.id
        left join device_latest_telemetry latest on latest.project_id=project.id and latest.device_id=$2 and latest.adapter_id=$3::bigint
        left join approval_requests approval on approval.id=$4::uuid and approval.project_id=project.id
        where project.id=$1`, [input.projectId, input.deviceId, routes.rows[0].connectorInstanceId, input.approvalRequestId ?? null,
          policy.featureFlag, policy.connectorCapabilityCode])).rows[0];
      if (!governance) throw new Error("FLIGHTHUB_COMMAND_SCOPE_DENIED");
      const authorized = authorizeFlightHubDiscreteCommand({
        projectId: input.projectId, teamId: access.teamId, deviceId: input.deviceId,
        capabilityCode: input.capabilityCode, commandKey: input.commandKey,
        parametersValid: validateFlightHubCommandParameters(input.commandKey, input.parameters),
        deviceTypeKey: device.deviceTypeKey, deviceOnline: device.status === "online",
        connectorStatus: routes.rows[0].connectorStatus, featureEnabled: governance.featureEnabled,
        capabilityFieldVerified: governance.capabilityFieldVerified, stateCapturedAt: governance.stateCapturedAt, now: new Date(),
        safetyPolicyVersionId: input.safetyPolicyVersionId ?? null,
        currentSafetyPolicyVersionId: governance.currentSafetyPolicyVersionId ? Number(governance.currentSafetyPolicyVersionId) : null,
        approvalProjectId: governance.approvalProjectId, approvalTeamId: governance.approvalTeamId,
        approvalResourceType: governance.approvalResourceType, approvalResourceId: governance.approvalResourceId,
        approvalAction: governance.approvalAction, approvalStatus: governance.approvalStatus,
        approvalUnexpired: governance.approvalUnexpired
      });
      flightHubSafety = { connectorKey: routes.rows[0].connectorKey, connectorInstanceId: routes.rows[0].connectorInstanceId,
        connectorCapabilityCode: policy.connectorCapabilityCode, featureFlag: policy.featureFlag,
        approvalRequestId: input.approvalRequestId, safetyPolicyVersionId: input.safetyPolicyVersionId,
        stateFresh: authorized.stateFresh, capabilityFieldVerified: governance.capabilityFieldVerified };
    }

    const commandId = randomUUID();
    const priority = input.capabilityCode === "flight.return_home" ? 100
      : device.riskLevel === "critical" ? 90 : device.riskLevel === "high" ? 80 : 10;
    const deadlineSeconds = Math.min(300, Math.max(5, input.deadlineSeconds ?? 30));
    const inserted = await client.query<{ id: string; status: string }>(
      `insert into device_commands(
         id,project_id,team_id,device_id,command_key,idempotency_key,capability_code,
         parameters_json,safety_context_json,status,priority,deadline_at,requested_by_user_id
       ) values($1,$2,$3,$4,$5,$6,$7,$8,$9,'dispatchable',$10,now()+($11*interval '1 second'),$12)
       on conflict(device_id,idempotency_key) do update set idempotency_key=excluded.idempotency_key
       returning id::text,status`, [commandId, input.projectId, access.teamId, input.deviceId,
        input.commandKey, input.idempotencyKey, input.capabilityCode, input.parameters,
        { reason: input.reason, confirmation: input.confirmation, confirmedByUserId: user.id,
          confirmedAt: new Date().toISOString(), ...safety, ...flightHubSafety }, priority, deadlineSeconds, user.id]
    );
    const command = inserted.rows[0];
    await publishProjectEvent(client, {
      projectId: input.projectId, teamId: access.teamId,
      eventId: `device.command.dispatch:${command.id}`, eventType: "device.command.dispatch",
      payload: { commandId: command.id }, enqueue: true
    });
    return { ...command, reused: command.id !== commandId };
  });
}
