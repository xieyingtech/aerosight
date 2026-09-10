import type { SituationSelection } from "./situation-state.ts";
import { projectNavigationHref } from "./project-navigation.ts";

export function realtimeDeviceHref(projectId: number, selection: SituationSelection | null) {
  if (!selection?.lane.startsWith("device-")) return null;
  const deviceId = Number(selection.entityId);
  if (!Number.isSafeInteger(deviceId) || deviceId <= 0) return null;
  return projectNavigationHref(projectId, "realtime", {deviceId});
}

export function overviewSelectionHref(projectId: number, selection: SituationSelection | null) {
  const realtime = realtimeDeviceHref(projectId, selection);
  if (realtime) return { href: realtime, label: "进入实时作业" };
  if (selection?.lane === "issue") {
    const issueId = Number(selection.entityId);
    if (Number.isSafeInteger(issueId) && issueId > 0) return { href: projectNavigationHref(projectId, "issues/detail", {issueId}), label: "查看案件" };
  }
  return null;
}
