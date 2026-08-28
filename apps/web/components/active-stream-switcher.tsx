"use client";

import { RadioTowerIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { activeProjectStreams } from "@/lib/realtime-workbench-core";
import type { ProjectSituationSnapshot } from "@/lib/project-snapshot-core";
import { cn } from "@/lib/utils";

export function ActiveStreamSwitcher({ snapshot, selectedStreamId, onSelect }: {
  snapshot: ProjectSituationSnapshot;
  selectedStreamId: number | null;
  onSelect: (streamId: number, deviceId: number) => void;
}) {
  const streams = activeProjectStreams(snapshot);
  return <section className="rounded-xl border bg-card p-4" aria-label="活跃直播频道">
    <div className="mb-3 flex items-center justify-between">
      <h2 className="flex items-center gap-2 font-medium"><RadioTowerIcon className="size-4" />活跃直播</h2>
      <span className="text-xs text-muted-foreground">{streams.length} 个频道</span>
    </div>
    {streams.length ? <div className="flex gap-2 overflow-x-auto pb-1">
      {streams.map((stream) => {
        const device = snapshot.devices.find((item) => Number(item.id) === Number(stream.deviceId));
        const selected = Number(stream.id) === selectedStreamId;
        return <button className={cn("min-w-44 rounded-lg border p-3 text-left transition-colors", selected ? "border-primary bg-primary/5" : "hover:bg-muted/50")} key={String(stream.id)} onClick={() => onSelect(Number(stream.id), Number(stream.deviceId))} type="button">
          <div className="flex items-center justify-between gap-2"><span className="truncate text-sm font-medium">{String(device?.name ?? `设备 #${stream.deviceId}`)}</span><Badge variant="outline">{String(stream.status)}</Badge></div>
          <p className="mt-1 truncate text-xs text-muted-foreground">{String(stream.streamKey ?? "默认视频频道")}</p>
        </button>;
      })}
    </div> : <p className="rounded-lg border border-dashed p-4 text-center text-sm text-muted-foreground">当前项目没有活跃直播，可先选择设备和视频频道启动。</p>}
  </section>;
}
