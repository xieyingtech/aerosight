import { z } from "zod";
import type { PoolClient, QueryResult, QueryResultRow } from "pg";

export const flightHubJoinCodeLookupSchema = z.object({
  projectCode: z.string().trim().min(1).max(256).regex(/^[A-Za-z0-9._:-]+$/),
  fastJoinCode: z.string().trim().min(1).max(256).regex(/^[A-Za-z0-9._:-]+$/),
  associationDroneSN: z.string().trim().min(1).max(256).regex(/^[A-Za-z0-9._:-]+$/).optional(),
}).strict();

export type FlightHubJoinCodeLookup = z.infer<typeof flightHubJoinCodeLookupSchema>;

type Access = { role: string; connectorProjectId: number; connectorTeamId: number; teamId: number;
  connectorStatus: string; managementCapabilityVerified: boolean };

export function authorizeFlightHubManagementRead(projectId: number, access: Access) {
  if (!new Set(["owner", "admin"]).has(access.role)) throw new Error("FLIGHTHUB_MANAGEMENT_PERMISSION_DENIED");
  if (access.connectorProjectId !== projectId || access.connectorTeamId !== access.teamId) throw new Error("FLIGHTHUB_MANAGEMENT_SCOPE_MISMATCH");
  if (access.connectorStatus !== "connected") throw new Error("FLIGHTHUB_MANAGEMENT_CONNECTOR_OFFLINE");
  if (!access.managementCapabilityVerified) throw new Error("FLIGHTHUB_MANAGEMENT_CAPABILITY_REQUIRED");
}

export function presentScopedFlightHubJoinCode(expected:{projectUuid:string;organizationUuid:string},result:{projectUuid:string;organizationUuid:string;
  projectName:string;organizationName:string;userInOrganization:boolean;recommendedUserCallsign:string;recommendedDroneCallsign:string|null}){
  if(result.projectUuid!==expected.projectUuid.toLowerCase()||result.organizationUuid!==expected.organizationUuid.toLowerCase()){
    throw new Error("FLIGHTHUB_MANAGEMENT_JOIN_CODE_SCOPE_MISMATCH");
  }
  return {projectName:result.projectName,organizationName:result.organizationName,userInOrganization:result.userInOrganization,
    recommendedUserCallsign:result.recommendedUserCallsign,recommendedDroneCallsign:result.recommendedDroneCallsign};
}

const summaryFields = Object.freeze({
  organization: ["name","status","industryType","industrySubtype","measureUnits","temperatureUnits","mfaEnabled","currentUserRole","source"],
  "organization-user": ["scope","account","accountSecond","nickname","role","sourceType","projectCount","organizationName","mfaEnabled","source"],
  "organization-role": ["name","description","roleType","preset","addToOrganization","permissionCount","source"],
  "organization-permission": ["sourceScope","name","description","permissionType","level","visible","basic","childCount","parentReference","source"],
  "project-user": ["account","nickname","callsign","projectRole","organizationRole","callsignUpdated","phoneFilled","emailFilled","source"],
  "project-member": ["account","projectCallsign","projectRole","organizationCallsign","organizationRole","online","pendingOffline","platform","source"],
} as const);

export type FlightHubManagementKind = keyof typeof summaryFields;

export function sanitizeFlightHubManagementSummary(kind: FlightHubManagementKind, value: unknown) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  const source=value as Record<string,unknown>, result:Record<string,string|number|boolean|null>={};
  for (const key of summaryFields[kind]) {
    const item=source[key];
    if (typeof item === "string") result[key]=item.slice(0,512);
    else if (typeof item === "number" && Number.isFinite(item)) result[key]=item;
    else if (typeof item === "boolean" || item === null) result[key]=item;
  }
  return result;
}

export function presentFlightHubManagementResources(rows: Array<{id:string;connectorId:string;kind:string;status:string;
  summary:unknown;lastSeenAt:string|Date;missingAt:string|Date|null}>) {
  return rows.flatMap((row) => {
    if (!(row.kind in summaryFields)) return [];
    const kind=row.kind as FlightHubManagementKind;
    return [{id:row.id,connectorId:row.connectorId,kind,status:row.status,
      summary:sanitizeFlightHubManagementSummary(kind,row.summary),lastSeenAt:row.lastSeenAt,missingAt:row.missingAt}];
  });
}

type ManagementClient=Pick<PoolClient,"query"|"release">;
async function managementQuery<T extends QueryResultRow>(client:ManagementClient,text:string,values:unknown[]=[]){
  return client.query<T>(text,values) as Promise<QueryResult<T>>;
}

export async function readFlightHubManagementCore(userId:number,projectId:number,connectorId:string,connect:()=>Promise<ManagementClient>){
  if(!Number.isSafeInteger(userId)||userId<=0||!Number.isSafeInteger(projectId)||projectId<=0||!/^\d+$/.test(connectorId))return null;
  const client=await connect();
  try{
    await client.query("begin isolation level repeatable read read only");
    const access=(await managementQuery<QueryResultRow&Access&{connectorId:string;connectorName:string}>(client,`/* flighthub-management:access */
      select membership.role,project.team_id::int as "teamId",adapter.project_id::int as "connectorProjectId",
        adapter.team_id::int as "connectorTeamId",adapter.id::text as "connectorId",adapter.name as "connectorName",adapter.status as "connectorStatus",
        exists(select 1 from connector_capability_snapshots capability where capability.project_id=adapter.project_id
          and capability.connector_instance_id=adapter.id and capability.capability_code='organization.read'
          and capability.status='supported' and capability.evidence_level in('read-probe','field-read','field-write')
          and (capability.expires_at is null or capability.expires_at>now())) as "managementCapabilityVerified"
      from projects project join team_members membership on membership.team_id=project.team_id and membership.user_id=$1
      join device_adapters adapter on adapter.project_id=project.id and adapter.team_id=project.team_id
      join connector_definitions definition on definition.id=adapter.connector_definition_id and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'
      where project.id=$2 and adapter.id=$3`,[userId,projectId,connectorId])).rows[0];
    if(!access){await client.query("commit");return null;}
    authorizeFlightHubManagementRead(projectId,access);
    const state=(await managementQuery(client,`/* flighthub-management:state */
      select status,attempt_count::int as "attemptCount",last_error_code as "lastErrorCode",last_started_at as "lastStartedAt",
        last_succeeded_at as "lastSucceededAt",next_attempt_at as "nextAttemptAt"
      from connector_resource_sync_states where project_id=$1 and team_id=$2 and connector_instance_id=$3 and resource_kind='organization'`,[projectId,access.teamId,connectorId])).rows[0]??null;
    const resources=await managementQuery<{id:string;connectorId:string;kind:string;status:string;summary:unknown;lastSeenAt:string|Date;missingAt:string|Date|null}>(client,`/* flighthub-management:resources */
      select id::text,connector_instance_id::text as "connectorId",resource_kind as kind,status,summary_json as summary,
        last_seen_at as "lastSeenAt",missing_at as "missingAt"
      from connector_remote_resources where project_id=$1 and team_id=$2 and connector_instance_id=$3
        and resource_kind in('organization','organization-user','organization-role','organization-permission','project-user','project-member')
      order by resource_kind,last_seen_at desc,id desc limit 2000`,[projectId,access.teamId,connectorId]);
    await client.query("commit");
    return {connector:{id:access.connectorId,name:access.connectorName,status:access.connectorStatus},managementCapabilityVerified:true,
      syncState:state,resources:presentFlightHubManagementResources(resources.rows)};
  }catch(error){await client.query("rollback").catch(()=>undefined);throw error;}finally{client.release();}
}
