"use client";

import { PlayIcon, RefreshCwIcon } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import type { ProjectSnapshotDevice } from "@/lib/project-snapshot-core";

export function LiveChannelControls({ projectId, device, onStarted }: {
  projectId: number;
  device: ProjectSnapshotDevice;
  onStarted: (streamId: number) => void | Promise<void>;
}) {
  const [pendingKey, setPendingKey] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const liveCapability = (device.capabilities ?? []).find((capability) => capability.code === "stream.video.control");
  const liveAction = liveCapability?.actions.find((action) => action.kind === "live");
  const channels = (device.channels ?? []).filter((channel) => channel.dataType === "video");
  if (!liveAction || !channels.length) return null;

  const start = async (streamKey: string) => {
    if (pendingKey) return;
    setPendingKey(streamKey);
    setError(null);
    const response = await fetch(`/api/projects/${projectId}/devices/${Number(device.id)}/live-streams`, {
      method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ streamKey })
    });
    const result = await response.json() as { error?: string; session?: { id: number } };
    setPendingKey(null);
    if (!response.ok || !result.session) { setError(result.error ?? "启动直播失败"); return; }
    await onStarted(result.session.id);
  };

  return <section className="space-y-2 rounded-xl border bg-card p-4" aria-label="视频频道控制">
    <div><h2 className="font-medium">视频频道</h2><p className="mt-1 text-xs text-muted-foreground">选择驱动声明的视频通道，启动后将在当前页面播放。</p></div>
    <div className="grid gap-2 sm:grid-cols-2">
      {channels.map((channel) => {
        const enabled = liveAction.enabled && channel.availability === "available";
        const reason = liveAction.unavailableReason ?? channel.availabilityReason;
        return <div className="flex items-center justify-between gap-3 rounded-lg border p-3" key={channel.stableChannelId}>
          <div className="min-w-0"><p className="truncate text-sm font-medium">{channel.displayName}</p><p className="truncate text-xs text-muted-foreground">{channel.channelKey} · {channel.protocol ?? "自动协议"}</p>{!enabled && reason && <p className="mt-1 text-xs text-amber-700">{reason}</p>}</div>
          <Button disabled={!enabled || Boolean(pendingKey)} onClick={() => start(channel.channelKey)} size="sm" variant="outline">
            {pendingKey === channel.channelKey ? <RefreshCwIcon className="animate-spin" /> : <PlayIcon />}启动
          </Button>
        </div>;
      })}
    </div>
    {error && <p className="text-sm text-destructive">{error}</p>}
  </section>;
}
