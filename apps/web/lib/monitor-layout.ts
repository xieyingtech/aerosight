import type { ProjectSituationSnapshot } from "./project-snapshot-core.ts";
import { activeProjectStreams } from "./realtime-workbench-core.ts";

export type MonitorView = { id: string; kind: "map" | "video"; label: string; deviceId?: number; channelKey?: string; streamId?: number };
export const videoViewId = (deviceId: number, channelKey: string) => `video:${deviceId}:${channelKey}`;

export function projectMonitorViews(snapshot: ProjectSituationSnapshot, added: readonly string[] = []): MonitorView[] {
  const views = new Map<string, MonitorView>();
  for (const device of snapshot.devices) {
    const deviceId = Number(device.id);
    for (const channel of (device.channels ?? []).filter(channel => channel.dataType === "video")) {
      const id = videoViewId(deviceId, channel.channelKey);
      if (added.includes(id)) views.set(id, { id, kind: "video", deviceId, channelKey: channel.channelKey, label: `${device.name}（${channel.displayName}）` });
    }
  }
  for (const stream of activeProjectStreams(snapshot)) {
    const device = snapshot.devices.find(device => Number(device.id) === Number(stream.deviceId));
    if (!device) continue;
    const channelKey = String(stream.streamKey ?? "");
    const id = videoViewId(Number(device.id), channelKey);
    const channel = device.channels?.find(channel => channel.channelKey === channelKey);
    views.set(id, { id, kind: "video", deviceId: Number(device.id), channelKey, streamId: Number(stream.id), label: `${device.name}（${channel?.displayName ?? (channelKey || "直播")}）` });
  }
  return [...views.values(), { id: "map", kind: "map", label: "项目地图" }];
}

export function mergeMonitorOrder(order: readonly string[], ids: readonly string[]) {
  return [...new Set([...order.filter(id => ids.includes(id)), ...ids])];
}

export function swapMonitorViews(order: readonly string[], source: string, target: string) {
  const next = [...order], from = next.indexOf(source), to = next.indexOf(target);
  if (from < 0 || to < 0 || from === to) return next;
  [next[from], next[to]] = [next[to], next[from]];
  return next;
}

export function closestMonitorCorner(centerX: number, centerY: number, width: number, height: number) {
  return `${centerY < height / 2 ? "top" : "bottom"}-${centerX < width / 2 ? "left" : "right"}`;
}
