import type { ProjectSituationSnapshot, ProjectSnapshotDevice } from "./project-snapshot-core.ts";

export type RealtimeWorkbenchSelection = {
  deviceId: number | null;
  streamId: number | null;
};

export type SnapshotRefresh = (options?: { selectStreamId?: number; reason?: "command" | "stream" | "manual" }) => Promise<ProjectSituationSnapshot | null>;

export function activeProjectStreams(snapshot: ProjectSituationSnapshot) {
  return snapshot.liveStreams.filter((stream) => ["requested", "starting", "live", "degraded", "stopping"].includes(String(stream.status)));
}

export function findProjectDevice(snapshot: ProjectSituationSnapshot, deviceId: number | null): ProjectSnapshotDevice | null {
  if (!deviceId) return null;
  return snapshot.devices.find((device) => Number(device.id) === deviceId) ?? null;
}

export function deviceHasLiveSignal(snapshot: ProjectSituationSnapshot, deviceId: number, now = Date.now()) {
  if (findProjectDevice(snapshot, deviceId)?.status !== "online") return false;
  return (snapshot.realtimeChannels ?? []).some(channel => {
    if (Number(channel.deviceId) !== deviceId) return false;
    const captured = Date.parse(String(channel.latestCapturedAt ?? ""));
    const payload = channel.latestPayload as { live?: { available?: boolean; active?: boolean } } | null;
    return Number.isFinite(captured) && captured <= now + 1000 && now - captured <= 30_000
      && payload?.live?.available === true && payload.live.active === true;
  });
}

export function relatedLiveDevices(snapshot: ProjectSituationSnapshot, deviceId: number | null) {
  const selected = findProjectDevice(snapshot, deviceId);
  if (!selected) return [];
  const ids = new Set([Number(selected.id)]);
  for (const relation of snapshot.deviceRelations ?? []) {
    if (!["contains", "mounted-on"].includes(relation.relationType)) continue;
    if (relation.fromDeviceId === deviceId) ids.add(relation.toDeviceId);
    if (relation.toDeviceId === deviceId) ids.add(relation.fromDeviceId);
  }
  return snapshot.devices.filter(device => ids.has(Number(device.id)) && realtimeDeviceModules(device).live);
}

export function realtimeDeviceModules(device: ProjectSnapshotDevice | null) {
  const codes = new Set((device?.capabilities ?? []).map((capability) => capability.code));
  return {
    live: codes.has("stream.video.read") || codes.has("stream.video.control"),
    timeline: codes.has("state.read") || (device?.channels ?? []).some((channel) => channel.dataType !== "video"),
  };
}

export function resolveWorkbenchSelection(
  snapshot: ProjectSituationSnapshot,
  requested: { deviceId?: string | number | null; streamId?: string | number | null }
): RealtimeWorkbenchSelection {
  const requestedStreamId = Number(requested.streamId);
  const stream = Number.isSafeInteger(requestedStreamId) && requestedStreamId > 0
    ? activeProjectStreams(snapshot).find((item) => Number(item.id) === requestedStreamId) : null;
  if (stream) {
    const streamDeviceId = Number(stream.deviceId);
    if (findProjectDevice(snapshot, streamDeviceId)) return { deviceId: streamDeviceId, streamId: requestedStreamId };
  }
  const requestedDeviceId = Number(requested.deviceId);
  if (Number.isSafeInteger(requestedDeviceId) && requestedDeviceId > 0 && findProjectDevice(snapshot, requestedDeviceId)) {
    const deviceStream = activeProjectStreams(snapshot).find((item) => Number(item.deviceId) === requestedDeviceId);
    return { deviceId: requestedDeviceId, streamId: deviceStream ? Number(deviceStream.id) : null };
  }
  return { deviceId: null, streamId: null };
}

export function workbenchQuery(selection: RealtimeWorkbenchSelection, projectId: number, currentQuery = "") {
  const query = new URLSearchParams(currentQuery);
  query.set("projectId", String(projectId));
  query.delete("deviceId");
  query.delete("streamId");
  if (selection.deviceId) query.set("deviceId", String(selection.deviceId));
  if (selection.streamId) query.set("streamId", String(selection.streamId));
  return query.toString();
}

export function hasTransitionalLiveStream(snapshot: ProjectSituationSnapshot) {
  return activeProjectStreams(snapshot).some((stream) => ["requested", "starting", "stopping"].includes(String(stream.status)));
}

export function isLiveStreamPlayable(status: unknown) {
  return ["live", "degraded"].includes(String(status));
}

export function liveStreamPollDecision(snapshot: ProjectSituationSnapshot, completedAttempts: number, maximumAttempts = 15) {
  if (!hasTransitionalLiveStream(snapshot)) return "stable" as const;
  return completedAttempts >= maximumAttempts ? "timeout" as const : "poll" as const;
}
