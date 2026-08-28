import "server-only";

import { withAuditedProjectWrite } from "@/lib/audit";
import {
  alertAutomationPolicyInputSchema,
  isAutomaticAlertMode,
  type AlertAutomationMode
} from "@/lib/alert-automation-policy-core";
import { requireCurrentProjectPermission } from "@/lib/data";
import { query } from "@/lib/db";
import { correlationId } from "@/lib/observability";

const DEFAULT_POLICY_NAME = "项目默认策略";

async function requireAutomationAdmin(projectId: number) {
  const scope = await requireCurrentProjectPermission(projectId, "safety:manage");
  if (scope.access.role !== "owner" && scope.access.role !== "admin") throw new Error("PROJECT_ADMIN_REQUIRED");
  return scope;
}

export async function listAlertAutomationSettings(projectId: number) {
  await requireAutomationAdmin(projectId);
  const currentPolicy = await query<{ mode: AlertAutomationMode }>(
    `select snapshot.mode
       from alert_automation_policy_versions snapshot
       join alert_automation_policies policy
         on policy.id=snapshot.alert_automation_policy_id and policy.project_id=snapshot.project_id
      where snapshot.project_id=$1 and snapshot.status='published' and snapshot.event_rule_version_id is null
      order by snapshot.id desc
      limit 1`,
    [projectId]
  );
  return {
    currentMode: currentPolicy.rows[0]?.mode ?? "manual"
  };
}

export async function saveAlertAutomationPolicy(projectId: number, rawInput: unknown, requestId?: string | null) {
  const raw = (rawInput ?? {}) as { mode?: unknown };
  const input = alertAutomationPolicyInputSchema.parse({ mode: raw.mode });
  const { user, access } = await requireAutomationAdmin(projectId);
  return withAuditedProjectWrite({
    projectId,
    teamId: access.teamId,
    actorUserId: user.id,
    requestId: correlationId(requestId),
    action: "alert_automation.save",
    resourceType: "alert_automation_policy",
    input,
    policyResult: { role: access.role, internalConfigurationSnapshots: true }
  }, async (client) => {
    const policy = (await client.query<{ id: number }>(
      `insert into alert_automation_policies(project_id,team_id,name,created_by_user_id)
       values($1,$2,$3,$4)
       on conflict(project_id,name) do update set updated_at=now()
       returning id`,
      [projectId, access.teamId, DEFAULT_POLICY_NAME, user.id]
    )).rows[0];
    const nextSnapshotNumber = (await client.query<{ value: number }>(
      `select coalesce(max(version),0)::int+1 as value
         from alert_automation_policy_versions
        where alert_automation_policy_id=$1`,
      [policy.id]
    )).rows[0].value;
    await client.query(
      `update alert_automation_policy_versions
          set status='retired'
        where project_id=$1 and alert_automation_policy_id=$2 and status='published'`,
      [projectId, policy.id]
    );
    const snapshot = (await client.query<{ id: number }>(
      `insert into alert_automation_policy_versions(
         project_id,team_id,alert_automation_policy_id,event_rule_version_id,version,status,mode,config_json,
         created_by_user_id,published_by_user_id,published_at
       ) values($1,$2,$3,null,$4,'published',$5,'{}'::jsonb,$6,$6,now())
       returning id`,
      [projectId, access.teamId, policy.id, nextSnapshotNumber, input.mode, user.id]
    )).rows[0];
    await client.query(
      `update alert_automation_policies
          set current_published_version_id=$3,updated_at=now()
        where id=$2 and project_id=$1`,
      [projectId, policy.id, snapshot.id]
    );
    if (!isAutomaticAlertMode(input.mode)) {
      await client.query(
        `update alert_automation_runs
            set status='canceled',failure_code='AUTOMATION_MODE_DISABLED',finished_at=now()
          where project_id=$1 and status in ('queued','running')`,
        [projectId]
      );
    }
    return { policyId: policy.id, mode: input.mode };
  });
}
