import "server-only";

import { credentialAAD, decryptCredentialObject, type CredentialEnvelope } from "@/lib/credential-encryption";
import { requireCurrentProjectPermission } from "@/lib/data";
import { db, query } from "@/lib/db";
import { createFlightHubProjectClient } from "@/lib/dji-flighthub-client";
import { authorizeFlightHubManagementRead, flightHubJoinCodeLookupSchema,
  presentScopedFlightHubJoinCode, readFlightHubManagementCore } from "@/lib/dji-flighthub-management-core";
import { getWebRuntimeConfig } from "@/lib/runtime-config";

export async function readFlightHubManagement(projectId:number,connectorId:string){
  const {user}=await requireCurrentProjectPermission(projectId,"project:view");
  const result=await readFlightHubManagementCore(user.id,projectId,connectorId,()=>db.connect());
  if(!result)throw new Error("FLIGHTHUB_MANAGEMENT_NOT_FOUND");
  return result;
}

export async function lookupFlightHubJoinCode(projectId:number,connectorId:string,rawInput:unknown){
  const input=flightHubJoinCodeLookupSchema.parse(rawInput);
  const {user,access}=await requireCurrentProjectPermission(projectId,"project:view");
  const row=(await query<{role:string;connectorProjectId:number;connectorTeamId:number;teamId:number;connectorStatus:string;
    managementCapabilityVerified:boolean;credentialEnvelope:CredentialEnvelope;projectUuid:string;organizationUuid:string}>(`select membership.role,
      adapter.project_id::int as "connectorProjectId",adapter.team_id::int as "connectorTeamId",project.team_id::int as "teamId",adapter.status as "connectorStatus",
      adapter.credential_envelope_json as "credentialEnvelope",adapter.discovery_scope_json->>'projectUuid' as "projectUuid",
      adapter.discovery_scope_json->>'organizationUuid' as "organizationUuid",
      exists(select 1 from connector_capability_snapshots capability where capability.project_id=adapter.project_id
        and capability.connector_instance_id=adapter.id and capability.capability_code='organization.read'
        and capability.status='supported' and capability.evidence_level in('read-probe','field-read','field-write')
        and (capability.expires_at is null or capability.expires_at>now())) as "managementCapabilityVerified"
    from projects project join team_members membership on membership.team_id=project.team_id and membership.user_id=$1
    join device_adapters adapter on adapter.project_id=project.id and adapter.team_id=project.team_id
    join connector_definitions definition on definition.id=adapter.connector_definition_id and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'
    where project.id=$2 and adapter.id=$3`,[user.id,projectId,connectorId])).rows[0];
  if(!row)throw new Error("FLIGHTHUB_MANAGEMENT_NOT_FOUND");
  authorizeFlightHubManagementRead(projectId,row);
  if(row.teamId!==access.teamId)throw new Error("FLIGHTHUB_MANAGEMENT_SCOPE_MISMATCH");
  let token="";
  try{
    const credential=decryptCredentialObject<{token:string}>(row.credentialEnvelope,getWebRuntimeConfig().authSecret,credentialAAD("device-adapter",connectorId,projectId));
    token=typeof credential.token==="string"?credential.token.trim():"";
    if(!token)throw new Error("FLIGHTHUB_MANAGEMENT_CREDENTIAL_UNAVAILABLE");
    const result=await createFlightHubProjectClient().getJoinCodeInfo(token,input);
    return presentScopedFlightHubJoinCode(row,result);
  }finally{token="";}
}
