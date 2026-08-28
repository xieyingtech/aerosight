import type { SituationSelection } from "./situation-state.ts";

export function realtimeDeviceHref(projectId: number, selection: SituationSelection | null) {
  if (!selection?.lane.startsWith("device-")) return null;
  const deviceId = Number(selection.entityId);
  if (!Number.isSafeInteger(deviceId) || deviceId <= 0) return null;
  return `/projects/${projectId}/realtime?deviceId=${deviceId}`;
}
