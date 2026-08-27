import type { ProjectPermission } from "./project-permission-policy.ts";
import type { TaskRunStatus } from "./task-run-core.ts";

export type MissionAction = "pause" | "resume" | "cancel" | "emergency_stop" | "approve";

export function requiredMissionPermission(action: MissionAction): ProjectPermission {
  return action === "approve" ? "mission:approve" : "mission:operate";
}

export function availableMissionActions(status: TaskRunStatus, permissions: ReadonlySet<ProjectPermission>): MissionAction[] {
  const actions: MissionAction[] = [];
  if (permissions.has("mission:operate")) {
    if (status === "running" || status === "dispatching") actions.push("pause");
    if (status === "paused") actions.push("resume");
    if (["queued", "blocked", "ready", "dispatching", "running", "paused"].includes(status)) actions.push("cancel");
    if (["dispatching", "running", "paused", "canceling"].includes(status)) actions.push("emergency_stop");
  }
  if (permissions.has("mission:approve") && ["blocked", "ready"].includes(status)) actions.push("approve");
  return actions;
}
