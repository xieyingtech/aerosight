import "server-only";

import { requireCurrentProjectPermission } from "@/lib/data";
import { query } from "@/lib/db";
import { taskOrchestrationDefinitionSchema } from "@/lib/task-orchestration-schema";

export async function readTaskTemplateWorkbench(projectId: number, taskId: number) {
  const { access } = await requireCurrentProjectPermission(projectId, "project:view");
  const task = (await query<Record<string, unknown>>(`select id,name,description,status,current_published_version_id as "currentPublishedVersionId"
    from tasks where project_id=$1 and id=$2`, [projectId,taskId])).rows[0];
  if (!task) throw new Error("TASK_NOT_FOUND");
  const versions = (await query<Record<string, unknown>>(`select id::int,version,status,definition_json as definition,
    input_schema_json as "inputSchema",trigger_json as trigger,concurrency_limit as "concurrencyLimit",
    created_at as "createdAt",published_at as "publishedAt"
    from task_versions where project_id=$1 and task_id=$2 order by version desc`, [projectId,taskId])).rows;
  const selected = versions.find((item) => item.status === "draft")
    ?? versions.find((item) => String(item.id) === String(task.currentPublishedVersionId))
    ?? versions[0];
  const steps = selected ? (await query<Record<string, unknown>>(`select id::int,position,step_key as key,name,uses,
    capability_code as "capabilityCode",action,parameters_json as "with",input_schema_json as "inputSchema",
    output_schema_json as "outputSchema",condition_json as condition,depends_on_json as "dependsOn",
    timeout_seconds as "timeoutSeconds",retry_policy_json as retry,failure_policy_json->>'onFailure' as "onFailure"
    from task_steps where project_id=$1 and task_version_id=$2 order by position`, [projectId,selected.id])).rows : [];
  const stored = selected?.definition;
  const parsed = taskOrchestrationDefinitionSchema.safeParse(stored);
  const definition = parsed.success ? parsed.data : {
    name: String(task.name),
    description: String(task.description ?? ""),
    inputSchema: selected?.inputSchema ?? { type: "object",properties: {},required: [],additionalProperties: false },
    trigger: selected?.trigger ?? { type: "manual" },
    concurrencyLimit: Number(selected?.concurrencyLimit ?? 1),
    steps: steps.map((step) => ({
      key: step.key,name: step.name,uses: step.uses,requires: [step.capabilityCode].filter(Boolean),with: step.with,
      inputSchema: step.inputSchema,outputSchema: step.outputSchema,condition: step.condition ?? undefined,
      dependsOn: step.dependsOn,timeoutSeconds: step.timeoutSeconds,retry: step.retry,onFailure: step.onFailure ?? "abort"
    }))
  };
  return { task, versions, selectedVersion: selected ?? null, steps, definition,
    canEdit: access.permissions.has("mission:operate") };
}
