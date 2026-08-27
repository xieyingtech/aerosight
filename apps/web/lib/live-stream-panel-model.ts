import type { ProjectSituationSnapshot } from "./project-snapshot-core.ts";

type Selection = { lane: string; entityId: string } | null;

function timestamp(item: Record<string, unknown>) {
  const value = item.capturedAt ?? item.createdAt;
  return typeof value === "string" ? Date.parse(value) : value instanceof Date ? value.getTime() : 0;
}

export function createLiveStreamPanelModel(input: {
  snapshot: ProjectSituationSnapshot;
  selection: Selection;
  mode: "live" | "history";
  cursor: string | null;
}) {
  const selectedDeviceId = input.selection?.lane.includes("device")
    ? Number(input.selection.entityId)
    : input.selection?.lane === "media"
      ? Number(input.snapshot.mediaPoints.find((item) => String(item.id) === input.selection?.entityId)?.deviceId)
      : null;
  if (input.mode === "history") {
    const cursor = input.cursor ? Date.parse(input.cursor) : Number.POSITIVE_INFINITY;
    const media = input.snapshot.mediaPoints
      .filter((item) => !selectedDeviceId || Number(item.deviceId) === selectedDeviceId)
      .filter((item) => timestamp(item) <= cursor)
      .sort((left, right) => timestamp(right) - timestamp(left))[0] ?? null;
    return { mode: "history" as const, selectedDeviceId, stream: null, media };
  }
  const streams = input.snapshot.liveStreams.filter((item) =>
    ["starting", "live", "degraded"].includes(String(item.status))
  );
  const stream = streams.find((item) => selectedDeviceId && Number(item.deviceId) === selectedDeviceId)
    ?? (selectedDeviceId ? null : streams[0] ?? null);
  return { mode: "live" as const, selectedDeviceId, stream, media: null };
}
