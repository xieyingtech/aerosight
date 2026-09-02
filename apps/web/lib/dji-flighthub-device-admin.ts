import "server-only";

import { randomUUID } from "node:crypto";
import type { PoolClient } from "pg";
import { withAuditedProjectWrite } from "@/lib/audit";
import { auditHash } from "@/lib/audit-boundary";
import { credentialAAD, decryptCredentialObject, encryptCredentialObject, type CredentialEnvelope } from "@/lib/credential-encryption";
import { requireCurrentProjectPermission } from "@/lib/data";
import { authorizeFlightHubDeviceAdmin, DEVICE_ADMIN_POLICY, flightHubDeviceAdminInputSchema, type FlightHubDeviceAdminInput } from "@/lib/dji-flighthub-device-admin-core";
import { query } from "@/lib/db";
import { correlationId } from "@/lib/observability";
import { getWebRuntimeConfig } from "@/lib/runtime-config";

async function loadAuthorization(client: PoolClient, projectId: number, teamId: number, userId: number, input: FlightHubDeviceAdminInput) {
  const policy = DEVICE_ADMIN_POLICY[input.action];
  const deviceId = "deviceId" in input ? input.deviceId : null;
  const result = await client.query(`select $3::int as "teamId",member.role,adapter.project_id as "connectorProjectId",adapter.team_id as "connectorTeamId",
    adapter.status as "connectorStatus",coalesce(flags.flighthub_action_flags_json @> jsonb_build_object($7::text,true),false) as "featureEnabled",
    exists(select 1 from connector_capability_snapshots capability where capability.project_id=adapter.project_id
      and capability.connector_instance_id=adapter.id and capability.capability_code=$6 and capability.status='supported'
      and capability.evidence_level='field-write' and (capability.expires_at is null or capability.expires_at>now())
      and (($5::int is null and capability.device_model is null and capability.firmware_version is null)
        or ($5::int is not null and capability.device_model=device.device_model and capability.firmware_version=device.firmware_version))) as "capabilityVerified",
    device.project_id as "deviceProjectId",identity.id is not null as "identityPresent",coalesce(device.status='online',false) as "deviceOnline",
    coalesce(latest.captured_at>now()-interval '30 seconds' and latest.captured_at<=now()+interval '1 second',false) as "stateFresh",
    approval.project_id as "approvalProjectId",approval.team_id as "approvalTeamId",approval.resource_type as "approvalResourceType",
    approval.resource_id as "approvalResourceId",approval.action as "approvalAction",approval.status as "approvalStatus",
    coalesce(approval.expires_at>now(),false) as "approvalUnexpired"
   from device_adapters adapter join team_members member on member.team_id=adapter.team_id and member.user_id=$4
   left join project_feature_flags flags on flags.project_id=adapter.project_id
   left join devices device on device.id=$5 and device.project_id=adapter.project_id
   left join device_external_identities identity on identity.project_id=adapter.project_id and identity.adapter_id=adapter.id and identity.device_id=device.id
   left join device_latest_telemetry latest on latest.project_id=adapter.project_id and latest.adapter_id=adapter.id and latest.device_id=device.id
   left join approval_requests approval on approval.id=$8::uuid and approval.project_id=adapter.project_id
   where adapter.id=$2 and adapter.project_id=$1 and adapter.team_id=$3`,
    [projectId,input.connectorInstanceId,teamId,userId,deviceId,policy.capability,policy.featureFlag,input.approvalRequestId]);
  if (!result.rows[0]) throw new Error("FLIGHTHUB_DEVICE_ADMIN_SCOPE_MISMATCH");
  return result.rows[0];
}

export async function submitFlightHubDeviceAdminAction(projectId: number, rawInput: unknown, requestId?: string | null) {
  const input = flightHubDeviceAdminInputSchema.parse(rawInput);
  const { user, access } = await requireCurrentProjectPermission(projectId, "device:configure");
  const policy = DEVICE_ADMIN_POLICY[input.action];
  const deviceId = "deviceId" in input ? input.deviceId : null;
  const jobId = randomUUID();
  const requestDigest = auditHash({ action: input.action, connectorInstanceId: input.connectorInstanceId, deviceId, request: input.request });
  const envelope = encryptCredentialObject(input.request, getWebRuntimeConfig().authSecret, credentialAAD("flighthub-device-admin-action", jobId, projectId));
  return withAuditedProjectWrite({
    projectId,teamId:access.teamId,actorUserId:user.id,requestId:correlationId(requestId),idempotencyKey:input.idempotencyKey,
    action:`connector.${input.action}`,resourceType:deviceId===null?"connector":"device",resourceId:String(deviceId??input.connectorInstanceId),
    input:{action:input.action,connectorInstanceId:input.connectorInstanceId,deviceId,request:{digest:requestDigest}},
    policyResult:{permission:"project:admin",capability:policy.capability,featureFlag:policy.featureFlag,evidence:"field-write",approval:input.approvalRequestId}
  }, async (client) => {
    const authorization = await loadAuthorization(client,projectId,access.teamId,user.id,input);
    const plan = authorizeFlightHubDeviceAdmin(projectId,input,authorization);
    const inserted = await client.query<{id:string;status:string}>(`insert into connector_device_admin_jobs(
      id,project_id,team_id,connector_instance_id,device_id,requested_by_user_id,approval_request_id,action_kind,
      capability_code,feature_flag,idempotency_key,request_digest,request_envelope_json
    ) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) on conflict(project_id,connector_instance_id,action_kind,idempotency_key) do nothing returning id::text,status`,
      [jobId,projectId,access.teamId,input.connectorInstanceId,deviceId,user.id,input.approvalRequestId,input.action,plan.capability,plan.featureFlag,input.idempotencyKey,requestDigest,envelope]);
    let job=inserted.rows[0],reused=false;
    if(!job){
      const existing=(await client.query<{id:string;status:string;requestDigest:string;requestedByUserId:number}>(`select id::text,status,request_digest as "requestDigest",requested_by_user_id as "requestedByUserId" from connector_device_admin_jobs where project_id=$1 and connector_instance_id=$2 and action_kind=$3 and idempotency_key=$4`,[projectId,input.connectorInstanceId,input.action,input.idempotencyKey])).rows[0];
      if(!existing||existing.requestDigest!==requestDigest||existing.requestedByUserId!==user.id) throw new Error("IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST");
      job={id:existing.id,status:existing.status};reused=true;
    }
    await client.query(`insert into outbox_events(project_id,team_id,event_id,event_type,aggregate_type,aggregate_id,payload_json,max_attempts) values($1,$2,$3,'flighthub.device_admin.requested','connector_device_admin_job',$4,$5,8) on conflict(event_id) do nothing`,[projectId,access.teamId,`flighthub-device-admin:${job.id}`,job.id,{jobId:job.id}]);
    return {...job,reused};
  });
}

export async function readFlightHubDeviceAdminJob(projectId:number,connectorInstanceId:number,jobId:string){
  const {access}=await requireCurrentProjectPermission(projectId,"project:view");
  if(!new Set(["owner","admin"]).has(access.role)) throw new Error("FLIGHTHUB_DEVICE_ADMIN_PERMISSION_DENIED");
  const row=(await query<{action:string;status:string;attemptCount:number;lastErrorCode:string|null;result:Record<string,unknown>;resultEnvelope:CredentialEnvelope|null}>(`select action_kind as action,status,attempt_count as "attemptCount",last_error_code as "lastErrorCode",result_json as result,result_envelope_json as "resultEnvelope" from connector_device_admin_jobs where id=$1::uuid and project_id=$2 and connector_instance_id=$3`,[jobId,projectId,connectorInstanceId])).rows[0];
  if(!row) throw new Error("FLIGHTHUB_DEVICE_ADMIN_NOT_FOUND");
  let sensitiveResult:Record<string,unknown>|null=null;
  if(row.action==="sn-decrypt"&&row.status==="succeeded"&&row.resultEnvelope) sensitiveResult=decryptCredentialObject(row.resultEnvelope,getWebRuntimeConfig().authSecret,credentialAAD("flighthub-device-admin-result",jobId,projectId));
  return {action:row.action,status:row.status,attemptCount:row.attemptCount,lastErrorCode:row.lastErrorCode,result:row.result,sensitiveResult};
}
