import "server-only";

import {withAuditedProjectWrite} from "@/lib/audit";
import {requireCurrentProjectPermission} from "@/lib/data";
import {query} from "@/lib/db";
import {alertAutomationPolicyInputSchema} from "@/lib/alert-automation-policy-core";
import {correlationId} from "@/lib/observability";

async function requireAutomationAdmin(projectId:number){const scope=await requireCurrentProjectPermission(projectId,"safety:manage");if(scope.access.role!=="owner"&&scope.access.role!=="admin")throw new Error("PROJECT_ADMIN_REQUIRED");return scope}

export async function listAlertAutomationSettings(projectId:number){
  await requireAutomationAdmin(projectId);
  const feature=(await query<{automaticAi:boolean}>(`select coalesce(flags.automatic_ai_enabled,false) as "automaticAi" from projects project left join project_feature_flags flags on flags.project_id=project.id where project.id=$1`,[projectId])).rows[0];
  const policies=(await query<Record<string,unknown>>(`select policy.id,policy.name,version.mode,version.event_rule_version_id as "eventRuleVersionId" from alert_automation_policies policy left join alert_automation_policy_versions version on version.id=policy.current_published_version_id and version.project_id=policy.project_id where policy.project_id=$1 order by policy.name`,[projectId])).rows;
  return {automaticAi:feature?.automaticAi??false,policies};
}

export async function setAutomaticAiEnabled(projectId:number,enabled:boolean,requestId?:string|null){const {user,access}=await requireAutomationAdmin(projectId);return withAuditedProjectWrite({projectId,teamId:access.teamId,actorUserId:user.id,requestId:correlationId(requestId),action:"alert_automation.kill_switch",resourceType:"project",resourceId:String(projectId),input:{enabled},policyResult:{role:access.role}},async(client)=>{
  await client.query(`insert into project_feature_flags(project_id,automatic_ai_enabled,updated_by_user_id) values($1,$2,$3) on conflict(project_id) do update set automatic_ai_enabled=excluded.automatic_ai_enabled,updated_by_user_id=excluded.updated_by_user_id,updated_at=now()`,[projectId,enabled,user.id]);
  if(!enabled)await client.query(`update alert_automation_runs set status='canceled',failure_code='AUTOMATIC_AI_DISABLED',finished_at=now() where project_id=$1 and status='queued'`,[projectId]);
  return {automaticAi:enabled};
})}

export async function saveAlertAutomationPolicy(projectId:number,rawInput:unknown,requestId?:string|null){const raw=rawInput as {name?:unknown;mode?:unknown;eventRuleVersionId?:unknown;config?:unknown};const input=alertAutomationPolicyInputSchema.parse({mode:raw.mode,eventRuleVersionId:raw.eventRuleVersionId,config:raw.config});const {user,access}=await requireAutomationAdmin(projectId);const name=typeof raw.name==="string"?raw.name.trim():"";if(!name)throw new Error("ALERT_AUTOMATION_POLICY_NAME_REQUIRED");return withAuditedProjectWrite({projectId,teamId:access.teamId,actorUserId:user.id,requestId:correlationId(requestId),action:"alert_automation.save",resourceType:"alert_automation_policy",input:{name,...input},policyResult:{role:access.role}},async(client)=>{
  const policy=(await client.query<{id:number}>(`insert into alert_automation_policies(project_id,team_id,name,created_by_user_id) values($1,$2,$3,$4) on conflict(project_id,name) do update set updated_at=now() returning id`,[projectId,access.teamId,name,user.id])).rows[0];
  const next=(await client.query<{version:number}>(`select coalesce(max(version),0)::int+1 as version from alert_automation_policy_versions where alert_automation_policy_id=$1`,[policy.id])).rows[0].version;
  await client.query(`update alert_automation_policy_versions set status='retired' where project_id=$1 and alert_automation_policy_id=$2 and status='published'`,[projectId,policy.id]);
  const version=(await client.query<{id:number}>(`insert into alert_automation_policy_versions(project_id,team_id,alert_automation_policy_id,event_rule_version_id,version,status,mode,config_json,created_by_user_id,published_by_user_id,published_at) values($1,$2,$3,$4,$5,'published',$6,$7,$8,$8,now()) returning id`,[projectId,access.teamId,policy.id,input.eventRuleVersionId,next,input.mode,input.config,user.id])).rows[0];
  await client.query(`update alert_automation_policies set current_published_version_id=$3,updated_at=now() where id=$2 and project_id=$1`,[projectId,policy.id,version.id]);return {policyId:policy.id,mode:input.mode};
})}
