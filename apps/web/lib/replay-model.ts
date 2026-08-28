import type { ProjectReplay } from "./project-replay-core.ts";
import type { ProjectSituationSnapshot } from "./project-snapshot-core.ts";

export function applyReplayToSnapshot(snapshot: ProjectSituationSnapshot, replay: ProjectReplay): ProjectSituationSnapshot {
  if (replay.projectId !== snapshot.project.id) throw new Error("REPLAY_PROJECT_SCOPE_MISMATCH");
  const grouped = new Map<string, Array<Record<string, unknown>>>();
  for (const pose of replay.poses) {
    const key = String(pose.deviceId);
    grouped.set(key, [...(grouped.get(key) ?? []), pose]);
  }
  const devices: Array<Record<string, unknown>> = [];
  const tracks: Array<Record<string, unknown>> = [];
  for (const [deviceId, poses] of grouped) {
    poses.sort((left, right) => Date.parse(String(left.capturedAt)) - Date.parse(String(right.capturedAt)));
    const latest = poses.at(-1)!;
    const existing = snapshot.devices.find((device) => String(device.id) === deviceId);
    devices.push({
      id: Number(deviceId), name: latest.deviceName ?? existing?.name ?? `设备 ${deviceId}`,
      type: latest.deviceType ?? existing?.type ?? "unknown", status: "historical",
      pose: { longitude: latest.longitude, latitude: latest.latitude, altitudeMeters: latest.altitudeMeters, capturedAt: latest.capturedAt, spatialQuality: latest.spatialQuality }
    });
    if (poses.length >= 2) tracks.push({
      deviceId: Number(deviceId), startedAt: poses[0].capturedAt, endedAt: latest.capturedAt,
      pointCount: poses.length, geometry: { type: "LineString", coordinates: poses.map((pose) => [Number(pose.longitude), Number(pose.latitude), Number(pose.altitudeMeters ?? 0)]) }
    });
  }
  return {
    ...snapshot, generatedAt: replay.window.to, devices, tracks, mediaPoints: replay.media,
    activeTasks: [], liveStreams: [], suspectedConstruction: [], openIssues: [], openAlerts: [],
    freshness: { latestCapturedAt: replay.window.to, isRealtime: false }
  };
}
