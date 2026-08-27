export type TaskVersionStatus = "draft" | "published" | "retired";

export function assertDraftPublishable(input: {
  status: TaskVersionStatus;
  definition: Record<string, unknown>;
  script: string;
  steps: Array<{ position: number; stepKey: string; action: string }>;
}) {
  if (input.status !== "draft") throw new Error("TASK_VERSION_NOT_DRAFT");
  if (!input.script.trim()) throw new Error("TASK_VERSION_SCRIPT_REQUIRED");
  if (!input.definition.name || typeof input.definition.name !== "string") throw new Error("TASK_VERSION_NAME_REQUIRED");
  if (input.steps.length === 0) throw new Error("TASK_VERSION_STEPS_REQUIRED");
  const positions = new Set<number>();
  const keys = new Set<string>();
  for (const step of input.steps) {
    if (!Number.isInteger(step.position) || step.position <= 0 || positions.has(step.position)) {
      throw new Error("TASK_STEP_POSITION_INVALID");
    }
    if (!step.stepKey.trim() || keys.has(step.stepKey) || !step.action.trim()) throw new Error("TASK_STEP_INVALID");
    positions.add(step.position);
    keys.add(step.stepKey);
  }
}
