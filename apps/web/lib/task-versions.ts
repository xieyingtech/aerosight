import "server-only";

import { randomUUID } from "node:crypto";

import { withAuditedProjectWrite } from "@/lib/audit";
import { requireCurrentProjectPermission } from "@/lib/data";
import { query } from "@/lib/db";
import { correlationId } from "@/lib/observability";
import { publishProjectEvent } from "@/lib/project-events";
import { assertDraftPublishable, type TaskVersionStatus } from "@/lib/task-version-core";

type TaskVersionRow = {
  id: string;
  projectId: number;
  taskId: number;
  version: number;
  status: TaskVersionStatus;
  definition: Record<string, unknown>;
  script: string;
  inputSchema: Record<string, unknown>;
  trigger: Record<string, unknown>;
  concurrencyLimit: number;
};

const projection = `id, project_id as "projectId", task_id as "taskId", version, status,
  definition_json as definition, script, input_schema_json as "inputSchema", trigger_json as trigger,
  concurrency_limit as "concurrencyLimit"`;

const emptyInputSchema = { type: "object", properties: {}, additionalProperties: false };

function legacyTrigger(definition: Record<string, unknown>) {
  if (definition.triggerType === "schedule") {
    return { type: "schedule", cron: String(definition.schedule || "0 0 * * *"), timezone: "UTC", enabled: true };
  }
  if (definition.triggerType === "event") return { type: "webhook", source: "legacy-event" };
  return { type: "manual" };
}

export async function createTaskDraft(projectId: number, taskId: number, requestId?: string | null) {
  const { user, access } = await requireCurrentProjectPermission(projectId, "mission:operate");
  return withAuditedProjectWrite(
    {
      projectId, teamId: access.teamId, requestId: correlationId(requestId), actorUserId: user.id,
      action: "task_version.create_draft", resourceType: "task", resourceId: String(taskId), input: {},
      policyResult: { permission: "mission:operate" }
    },
    async (client) => {
      const existing = await client.query<TaskVersionRow>(
        `select ${projection} from task_versions where project_id = $1 and task_id = $2 and status = 'draft'`,
        [projectId, taskId]
      );
      if (existing.rows[0]) return { draft: existing.rows[0], replayed: true };
      const task = await client.query<{ teamId: number; currentVersionId: string | null; script: string; definition: Record<string, unknown> }>(
        `select task.team_id as "teamId", task.current_published_version_id as "currentVersionId",
                task.script, jsonb_build_object('name', task.name, 'description', task.description,
                  'triggerType', task.trigger_type, 'targetSelector', task.target_selector_json,
                  'schedule', task.schedule, 'eventRule', task.event_rule_json) as definition
           from tasks task where task.project_id = $1 and task.id = $2 for update`,
        [projectId, taskId]
      );
      if (!task.rows[0]) throw new Error("TASK_NOT_FOUND");
      const next = await client.query<{ version: number }>(
        "select coalesce(max(version), 0)::int + 1 as version from task_versions where task_id = $1", [taskId]
      );
      const source = task.rows[0].currentVersionId
        ? await client.query<{ definition: Record<string, unknown>; script: string; inputSchema: Record<string, unknown>; trigger: Record<string, unknown>; concurrencyLimit: number }>(
          `select definition_json as definition, script, input_schema_json as "inputSchema", trigger_json as trigger,
                  concurrency_limit as "concurrencyLimit" from task_versions where project_id = $1 and id = $2`,
          [projectId, task.rows[0].currentVersionId]
        ) : null;
      const inserted = await client.query<TaskVersionRow>(
        `insert into task_versions (
           project_id, team_id, task_id, version, status, definition_json, script, input_schema_json,
           trigger_json, concurrency_limit, created_by_user_id
         ) values ($1, $2, $3, $4, 'draft', $5, $6, $7, $8, $9, $10) returning ${projection}`,
        [projectId, task.rows[0].teamId, taskId, next.rows[0].version,
          source?.rows[0]?.definition ?? task.rows[0].definition,
          source?.rows[0]?.script ?? task.rows[0].script,
          source?.rows[0]?.inputSchema ?? emptyInputSchema,
          source?.rows[0]?.trigger ?? legacyTrigger(task.rows[0].definition),
          source?.rows[0]?.concurrencyLimit ?? 1, user.id]
      );
      if (task.rows[0].currentVersionId) {
        await client.query(
          `insert into task_steps (
             project_id, team_id, task_version_id, position, step_key, name, capability_code,
             action, parameters_json, failure_policy_json, media_requirements_json, uses,
             input_schema_json, output_schema_json, condition_json, depends_on_json, timeout_seconds, retry_policy_json
           ) select project_id, team_id, $3, position, step_key, name, capability_code,
                    action, parameters_json, failure_policy_json, media_requirements_json, uses,
                    input_schema_json, output_schema_json, condition_json, depends_on_json, timeout_seconds, retry_policy_json
               from task_steps where project_id = $1 and task_version_id = $2`,
          [projectId, task.rows[0].currentVersionId, inserted.rows[0].id]
        );
      }
      return { draft: inserted.rows[0], replayed: false };
    }
  );
}

export async function publishTaskDraft(projectId: number, versionId: number, requestId?: string | null) {
  const { user, access } = await requireCurrentProjectPermission(projectId, "mission:operate");
  return withAuditedProjectWrite(
    {
      projectId, teamId: access.teamId, requestId: correlationId(requestId), actorUserId: user.id,
      action: "task_version.publish", resourceType: "task_version", resourceId: String(versionId), input: {},
      policyResult: { permission: "mission:operate" }
    },
    async (client) => {
      const version = await client.query<TaskVersionRow>(
        `select ${projection} from task_versions where project_id = $1 and id = $2 for update`, [projectId, versionId]
      );
      const row = version.rows[0];
      if (!row) throw new Error("TASK_VERSION_NOT_FOUND");
      const steps = await client.query<{
        position: number; stepKey: string; name: string; capabilityCode: string; action: string;
        parameters: Record<string, unknown>; failurePolicy: Record<string, unknown>;
        mediaRequirements: Record<string, unknown>;
      }>(
        `select position, step_key as "stepKey", name, capability_code as "capabilityCode", action,
                parameters_json as parameters, failure_policy_json as "failurePolicy",
                media_requirements_json as "mediaRequirements" from task_steps
          where project_id = $1 and task_version_id = $2 order by position`, [projectId, versionId]
      );
      assertDraftPublishable({ ...row, steps: steps.rows });
      const published = await client.query<TaskVersionRow>(
        `update task_versions set status = 'published', published_by_user_id = $3, published_at = now()
          where project_id = $1 and id = $2 and status = 'draft' returning ${projection}`,
        [projectId, versionId, user.id]
      );
      await client.query(
        "update tasks set current_published_version_id = $3, updated_at = now() where project_id = $1 and id = $2",
        [projectId, row.taskId, versionId]
      );
      await publishProjectEvent(client, {
        projectId, teamId: access.teamId, eventId: randomUUID(), eventType: "task_version.published",
        payload: { taskId: row.taskId, taskVersionId: versionId, version: row.version }, enqueue: false
      });
      return published.rows[0];
    }
  );
}

export async function listTaskVersions(projectId: number, taskId: number) {
  await requireCurrentProjectPermission(projectId, "project:view");
  return (await query<TaskVersionRow>(
    `select ${projection} from task_versions where project_id = $1 and task_id = $2 order by version desc`,
    [projectId, taskId]
  )).rows;
}
