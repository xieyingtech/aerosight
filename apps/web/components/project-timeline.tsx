"use client";

import { useMemo, useState } from "react";
import { ChevronDownIcon, Clock3Icon } from "lucide-react";
import { buildTimelineModel, timelinePosition } from "@/lib/timeline-model";
import type { ProjectSituationSnapshot } from "@/lib/project-snapshot-core";
import { cn } from "@/lib/utils";

const laneColors = {
  devices: "bg-sky-500", tasks: "bg-violet-500", media: "bg-fuchsia-500",
  algorithms: "bg-amber-500", detections: "bg-orange-500", alerts: "bg-red-500"
};

export function ProjectTimeline({ snapshot }: { snapshot: ProjectSituationSnapshot }) {
  const [expanded, setExpanded] = useState(true);
  const model = useMemo(() => buildTimelineModel(snapshot), [snapshot]);
  return (
    <section className="overflow-hidden rounded-xl border bg-card">
      <button className="flex w-full items-center justify-between px-4 py-3 text-left" onClick={() => setExpanded((value) => !value)} type="button">
        <span className="flex items-center gap-2 text-sm font-medium"><Clock3Icon className="size-4" />时空时间线</span>
        <span className="flex items-center gap-2 text-xs text-muted-foreground">{new Date(model.from).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })} — {new Date(model.to).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })}<ChevronDownIcon className={cn("size-4 transition-transform", expanded && "rotate-180")} /></span>
      </button>
      {expanded && <div className="border-t px-4 py-3">
        <div className="space-y-2">
          {model.lanes.map((lane) => (
            <div className="grid grid-cols-[72px_minmax(0,1fr)] items-center gap-3" key={lane.key}>
              <div className="truncate text-xs text-muted-foreground">{lane.label}</div>
              <div className="relative h-8 overflow-hidden rounded bg-muted/50">
                <div className="absolute inset-x-0 top-1/2 h-px bg-border" />
                {lane.items.map((item) => (
                  <div className={cn("absolute top-1/2 h-3.5 min-w-3.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-background shadow-sm", laneColors[lane.key])}
                    key={item.id} style={{ left: `${timelinePosition(item.timestamp, model.from, model.to)}%`, width: item.count > 1 ? `${Math.min(32, 12 + item.count * 3)}px` : undefined }} title={`${item.label} · ${new Date(item.timestamp).toLocaleString("zh-CN")}${item.count > 1 ? ` · ${item.count} 项` : ""}`} />
                ))}
                {!lane.items.length && <span className="absolute inset-0 flex items-center justify-center text-[10px] text-muted-foreground/70">暂无数据</span>}
              </div>
            </div>
          ))}
        </div>
      </div>}
    </section>
  );
}
