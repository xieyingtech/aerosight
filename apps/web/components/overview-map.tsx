"use client";

import Link from "next/link";
import { ArrowRightIcon, CrosshairIcon, MapIcon } from "lucide-react";
import { useState } from "react";

import { ProjectMap } from "@/components/project-map";
import { overviewSelectionHref } from "@/lib/overview-map-core";
import type { ProjectSituationSnapshot } from "@/lib/project-snapshot-core";
import type { SituationSelection } from "@/lib/situation-state";

export function OverviewMap({ snapshot }: { snapshot: ProjectSituationSnapshot }) {
  const [selection, setSelection] = useState<SituationSelection | null>(null);
  const selectionHref = overviewSelectionHref(snapshot.project.id, selection);
  return <section className="space-y-3" aria-label="项目态势地图">
    <div className="flex items-center justify-between">
      <div>
        <h2 className="flex items-center gap-2 font-medium"><MapIcon className="size-4" />态势地图</h2>
        <p className="mt-1 text-xs text-muted-foreground">只读查看设备、任务、媒体与案件位置</p>
      </div>
      <span className="text-xs text-muted-foreground">{snapshot.freshness.isRealtime ? "实时快照" : "等待最新数据"}</span>
    </div>
    <div className="grid gap-3 xl:grid-cols-[minmax(0,1fr)_280px]">
      <ProjectMap onSelect={setSelection} selection={selection} snapshot={snapshot} />
      <aside className="flex min-h-56 flex-col rounded-xl border bg-card p-4">
        {selection ? <>
          <p className="text-xs text-muted-foreground">{selection.lane}</p>
          <h3 className="mt-1 font-medium">{selection.label}</h3>
          {selection.timestamp && <p className="mt-1 text-xs text-muted-foreground">{new Date(selection.timestamp).toLocaleString("zh-CN")}</p>}
          {selectionHref ? <Link className="mt-4 inline-flex items-center justify-center gap-2 rounded-md bg-primary px-3 py-2 text-sm text-primary-foreground" href={selectionHref.href}>
            {selectionHref.label}<ArrowRightIcon className="size-4" />
          </Link> : <p className="mt-4 text-xs text-muted-foreground">该要素仅供态势查看。</p>}
        </> : <div className="flex flex-1 flex-col items-center justify-center text-center">
          <CrosshairIcon className="mb-3 size-8 text-muted-foreground" />
          <p className="text-sm font-medium">选择地图要素</p>
          <p className="mt-1 text-xs text-muted-foreground">选择设备后可进入对应实时作业。</p>
        </div>}
      </aside>
    </div>
  </section>;
}
