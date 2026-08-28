"use client";

import { useMemo, useState } from "react";
import { ChevronDownIcon, Clock3Icon } from "lucide-react";
import { buildTimelineModel, timelinePosition } from "@/lib/timeline-model";
import type { ProjectSituationSnapshot } from "@/lib/project-snapshot-core";
import { cn } from "@/lib/utils";
import { interpolateTimeline, type SituationSelection } from "@/lib/situation-state";

const laneColors = {
  devices: "bg-sky-500", tasks: "bg-violet-500", media: "bg-fuchsia-500",
  algorithms: "bg-amber-500", detections: "bg-orange-500", alerts: "bg-red-500"
};

export function ProjectTimeline({ snapshot, cursor, range, onSelect, onCursorChange, onRangeChange, onReturnLive }: {
  snapshot: ProjectSituationSnapshot;
  cursor?: string | null;
  range?: { from: string; to: string } | null;
  onSelect?: (selection: SituationSelection) => void;
  onCursorChange?: (timestamp: string) => void;
  onRangeChange?: (from: string, to: string) => void;
  onReturnLive?: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const model = useMemo(() => buildTimelineModel(snapshot), [snapshot]);
  return (
    <section className="overflow-hidden rounded-xl border bg-card">
      <button className="flex w-full items-center justify-between px-4 py-3 text-left" onClick={() => setExpanded((value) => !value)} type="button">
        <span className="flex items-center gap-2 text-sm font-medium"><Clock3Icon className="size-4" />时空时间线</span>
        <span className="flex items-center gap-2 text-xs text-muted-foreground">{cursor ? new Date(cursor).toLocaleTimeString("zh-CN") : "实时"}<ChevronDownIcon className={cn("size-4 transition-transform", expanded && "rotate-180")} /></span>
      </button>
      {expanded && <div className="border-t px-4 py-3">
        <div className="space-y-2">
          {model.lanes.map((lane) => (
            <div className="grid grid-cols-[72px_minmax(0,1fr)] items-center gap-3" key={lane.key}>
              <div className="truncate text-xs text-muted-foreground">{lane.label}</div>
              <div className="relative h-8 overflow-hidden rounded bg-muted/50">
                <div className="absolute inset-x-0 top-1/2 h-px bg-border" />
                {range && <div className="absolute inset-y-0 bg-primary/10" style={{ left: `${timelinePosition(range.from, model.from, model.to)}%`, right: `${100 - timelinePosition(range.to, model.from, model.to)}%` }} />}
                {cursor && <div className="absolute inset-y-0 z-10 w-px bg-foreground/70" style={{ left: `${timelinePosition(cursor, model.from, model.to)}%` }} />}
                {lane.items.map((item) => (
                  <button className={cn("absolute top-1/2 h-3.5 min-w-3.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-background shadow-sm", laneColors[lane.key])}
                    key={item.id} onClick={() => onSelect?.({ lane: item.lane, entityId: item.entityId, label: item.label, timestamp: item.timestamp })}
                    style={{ left: `${timelinePosition(item.timestamp, model.from, model.to)}%`, width: item.count > 1 ? `${Math.min(32, 12 + item.count * 3)}px` : undefined }} title={`${item.label} · ${new Date(item.timestamp).toLocaleString("zh-CN")}${item.count > 1 ? ` · ${item.count} 项` : ""}`} type="button" />
                ))}
                {!lane.items.length && <span className="absolute inset-0 flex items-center justify-center text-[10px] text-muted-foreground/70">暂无数据</span>}
              </div>
            </div>
          ))}
        </div>
        <div className="mt-3 grid gap-2 border-t pt-3 md:grid-cols-[1fr_1fr_auto] md:items-end">
          <label className="grid gap-1 text-[11px] text-muted-foreground">时间游标
            <input aria-label="时间游标" max="1000" min="0" onChange={(event) => onCursorChange?.(interpolateTimeline(model.from, model.to, Number(event.target.value)))} type="range" value={Math.round(timelinePosition(cursor ?? model.to, model.from, model.to) * 10)} />
          </label>
          <div className="grid gap-1 text-[11px] text-muted-foreground"><span>时间范围</span><div className="grid grid-cols-2 gap-2">
            <input aria-label="范围开始" max="1000" min="0" onChange={(event) => onRangeChange?.(interpolateTimeline(model.from, model.to, Number(event.target.value)), range?.to ?? model.to)} type="range" value={Math.round(timelinePosition(range?.from ?? model.from, model.from, model.to) * 10)} />
            <input aria-label="范围结束" max="1000" min="0" onChange={(event) => onRangeChange?.(range?.from ?? model.from, interpolateTimeline(model.from, model.to, Number(event.target.value)))} type="range" value={Math.round(timelinePosition(range?.to ?? model.to, model.from, model.to) * 10)} />
          </div></div>
          <button className="rounded-md border bg-background px-3 py-1.5 text-xs hover:bg-muted" onClick={onReturnLive} type="button">返回实时</button>
        </div>
      </div>}
    </section>
  );
}
