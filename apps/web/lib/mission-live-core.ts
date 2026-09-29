import { projectPageHref } from "./page-routes.ts";

export function missionLiveHref(projectId: number, run: Record<string, unknown>) {
  if (!["queued", "ready", "dispatching", "running", "paused"].includes(String(run.status))) return null;
  const flight = run.realtimeFlight as { deviceId?: number; taskUuid?: string } | undefined;
  if (!flight?.taskUuid || !Number.isSafeInteger(flight.deviceId) || Number(flight.deviceId) <= 0) return null;
  return projectPageHref(projectId, "realtime", { deviceId: String(flight.deviceId), autoLive: "1", runId: String(run.id) });
}
