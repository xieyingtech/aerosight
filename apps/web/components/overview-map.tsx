"use client";

import Link from "next/link";
import { ArrowRightIcon, CrosshairIcon, RadioTowerIcon } from "lucide-react";
import { useState } from "react";

import { ProjectMap } from "@/components/project-map";
import { overviewSelectionHref } from "@/lib/overview-map-core";
import type { ProjectSituationSnapshot } from "@/lib/project-snapshot-core";
import type { SituationSelection } from "@/lib/situation-state";

export function OverviewMap({ snapshot }: { snapshot: ProjectSituationSnapshot }) {
  const [selection, setSelection] = useState<SituationSelection | null>(null);
  const selectionHref = overviewSelectionHref(snapshot.project.id, selection);
  const metrics = [
    ["设备", snapshot.devices.length],
    ["活动任务", snapshot.activeTasks.length],
    ["媒体", snapshot.mediaPoints.length],
    ["开放案件", snapshot.openIssues.length],
  ];
  return <section className="grid min-h-[600px] flex-1 gap-3 xl:min-h-0 xl:grid-cols-[minmax(0,1fr)_320px]" aria-label="项目态势地图">
    <ProjectMap className="h-[65svh] min-h-[520px] xl:h-full xl:min-h-0" onSelect={setSelection} selection={selection} snapshot={snapshot} />
    <aside className="flex min-h-0 flex-col gap-3 xl:overflow-y-auto xl:pr-1">
      <section className="rounded-xl border bg-card p-4">
        <div className="flex items-center justify-between gap-3">
          <div><h2 className="font-medium">项目态势</h2><p className="mt-1 text-xs text-muted-foreground">设备、任务、媒体与案件</p></div>
          <span className="inline-flex items-center gap-1.5 rounded-full border px-2 py-1 text-xs text-muted-foreground"><RadioTowerIcon className="size-3" />{snapshot.freshness.isRealtime ? "实时" : "等待数据"}</span>
        </div>
        <dl className="mt-4 grid grid-cols-2 gap-2">
          {metrics.map(([label, value]) => <div className="rounded-lg bg-muted/45 p-3" key={String(label)}><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1 text-2xl font-semibold tabular-nums">{value}</dd></div>)}
        </dl>
      </section>
      <section className="flex min-h-56 flex-1 flex-col rounded-xl border bg-card p-4">
        {selection ? <>
          <p className="text-xs text-muted-foreground">已选择 · {selection.lane}</p>
          <h3 className="mt-1 font-medium">{selection.label}</h3>
          {selection.timestamp && <p className="mt-1 text-xs text-muted-foreground">{new Date(selection.timestamp).toLocaleString("zh-CN")}</p>}
          {selectionHref ? <Link className="mt-4 inline-flex items-center justify-center gap-2 rounded-md bg-primary px-3 py-2 text-sm text-primary-foreground" href={selectionHref.href}>
            {selectionHref.label}<ArrowRightIcon className="size-4" />
          </Link> : <p className="mt-4 text-xs text-muted-foreground">该要素仅供态势查看。</p>}
        </> : <div className="flex flex-1 flex-col items-center justify-center text-center">
          <CrosshairIcon className="mb-3 size-8 text-muted-foreground" />
          <p className="text-sm font-medium">选择地图要素</p>
          <p className="mt-1 text-xs text-muted-foreground">在地图上选择设备、任务或案件查看详情。</p>
        </div>}
      </section>
    </aside>
  </section>;
}
