import type { ProjectSnapshotChannel } from "./project-snapshot-core.ts";

// DJI camera type 176 is Vision Assist. Prefer a reported available payload
// camera; retain Vision Assist as an explicit user-selectable channel.
export function defaultVideoChannel(channels: ProjectSnapshotChannel[]) {
  const available = channels.filter(channel => channel.availability === "available");
  const candidates = available.length ? available : channels;
  return candidates.find(channel => !channel.channelKey.startsWith("176-")) ?? candidates[0];
}
