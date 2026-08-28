import type { SituationSelection } from "./situation-state.ts";

export function realtimeDeviceHref(projectId: number, selection: SituationSelection | null) {
  if (!selection?.lane.startsWith("device-")) return null;
  const deviceId = Number(selection.entityId);
  if (!Number.isSafeInteger(deviceId) || deviceId <= 0) return null;
  return `/projects/${projectId}/realtime?deviceId=${deviceId}`;
}

export function overviewSelectionHref(projectId: number, selection: SituationSelection | null) {
  const realtime = realtimeDeviceHref(projectId, selection);
  if (realtime) return { href: realtime, label: "进入实时作业" };
  if (selection?.lane === "issue") {
    const issueId = Number(selection.entityId);
    if (Number.isSafeInteger(issueId) && issueId > 0) return { href: `/projects/${projectId}/issues/${issueId}`, label: "查看案件" };
  }
  return null;
}
