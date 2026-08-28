"use client";

import { useEffect, useMemo, useReducer, useState } from "react";
import { CrosshairIcon, ImageIcon, InfoIcon, RadioTowerIcon } from "lucide-react";
import { ProjectMap } from "@/components/project-map";
import { ProjectTimeline } from "@/components/project-timeline";
import type { ProjectSituationSnapshot } from "@/lib/project-snapshot-core";
import { initialSituationState, situationReducer } from "@/lib/situation-state";
import { applyReplayToSnapshot } from "@/lib/replay-model";
import type { ProjectReplay } from "@/lib/project-replay-core";
import { LiveStreamPanel } from "@/components/live-stream-panel";
import { OperationDiagnostics } from "@/components/operation-diagnostics";

function selectedRecord(snapshot: ProjectSituationSnapshot, lane: string, entityId: string) {
  const sources = lane.includes("device") || lane === "track" ? snapshot.devices
    : lane === "media" ? snapshot.mediaPoints
    : lane === "issue" || lane === "issues" ? snapshot.openIssues
    : lane === "alert" || lane === "alerts" ? snapshot.openAlerts
    : lane === "tasks" || lane === "mission-route" ? snapshot.activeTasks
    : lane === "detections" || lane === "suspected-construction" ? snapshot.suspectedConstruction
    : lane === "region" ? snapshot.regions : [];
  return sources.find((item) => String(item.id ?? item.deviceId) === entityId);
}

export function SituationExplorer({ snapshot, mapClassName }: { snapshot: ProjectSituationSnapshot; mapClassName?: string }) {
  const [state, dispatch] = useReducer(situationReducer, initialSituationState);
  const [replay, setReplay] = useState<ProjectReplay | null>(null);
  const [replayStatus, setReplayStatus] = useState<"idle" | "loading" | "error">("idle");
  useEffect(() => {
    if (!state.range) { setReplayStatus("idle"); return; }
    const controller = new AbortController();
    setReplayStatus("loading");
    const query = new URLSearchParams({ from: state.range.from, to: state.range.to });
    fetch(`/api/projects/${snapshot.project.id}/replay?${query}`, { signal: controller.signal, cache: "no-store" })
      .then(async (response) => { if (!response.ok) throw new Error("replay request failed"); return response.json() as Promise<ProjectReplay>; })
      .then((value) => { setReplay(value); setReplayStatus("idle"); })
      .catch((error) => { if (error?.name !== "AbortError") setReplayStatus("error"); });
    return () => controller.abort();
  }, [snapshot.project.id, state.range]);
  const viewSnapshot = useMemo(() => state.range && replay && replay.window.from === state.range.from && replay.window.to === state.range.to
    ? applyReplayToSnapshot(snapshot, replay) : snapshot, [snapshot, replay, state.range]);
  const detail = useMemo(() => state.selection
    ? selectedRecord(viewSnapshot, state.selection.lane, state.selection.entityId) : null, [viewSnapshot, state.selection]);
  return (
    <div className="space-y-3">
      <div className="grid gap-3 xl:grid-cols-[minmax(0,1fr)_300px]">
        <ProjectMap className={mapClassName} onSelect={(selection) => dispatch({ type: "select", selection })} range={state.range} selection={state.selection} snapshot={viewSnapshot} />
        <aside className="flex min-h-72 flex-col rounded-xl border bg-card">
          <div className="flex items-center justify-between border-b px-4 py-3"><span className="flex items-center gap-2 text-sm font-medium"><InfoIcon className="size-4" />要素详情</span><span className="text-xs text-muted-foreground">{state.mode === "live" ? "实时" : "历史"}</span></div>
          {state.selection ? <div className="space-y-4 p-4">
            <div><p className="text-xs text-muted-foreground">{state.selection.lane}</p><h3 className="font-medium">{state.selection.label}</h3>{state.selection.timestamp && <p className="mt-1 text-xs text-muted-foreground">{new Date(state.selection.timestamp).toLocaleString("zh-CN")}</p>}</div>
            {state.selection.lane === "media" && <div className="flex aspect-video items-center justify-center rounded-lg border bg-muted/40"><div className="text-center text-xs text-muted-foreground"><ImageIcon className="mx-auto mb-2 size-7" />媒体帧将在授权后加载</div></div>}
            {detail && <dl className="grid grid-cols-[80px_1fr] gap-x-3 gap-y-2 text-xs">
              {Object.entries(detail).filter(([key, value]) => !["metadata", "input", "pose", "geometry"].includes(key) && ["string", "number", "boolean"].includes(typeof value)).slice(0, 8).map(([key, value]) => <div className="contents" key={key}><dt className="truncate text-muted-foreground">{key}</dt><dd className="truncate">{String(value)}</dd></div>)}
            </dl>}
          </div> : <div className="flex flex-1 flex-col items-center justify-center p-8 text-center"><CrosshairIcon className="mb-3 size-8 text-muted-foreground" /><p className="text-sm font-medium">选择地图或时间线要素</p><p className="mt-1 text-xs text-muted-foreground">设备、媒体和告警会同步显示在这里。</p></div>}
          <div className="mt-auto border-t px-4 py-3 text-xs text-muted-foreground"><span className="flex items-center gap-2"><RadioTowerIcon className="size-3.5" />{replayStatus === "loading" ? "正在加载回放…" : replayStatus === "error" ? "回放加载失败" : state.range ? `${new Date(state.range.from).toLocaleTimeString("zh-CN")} — ${new Date(state.range.to).toLocaleTimeString("zh-CN")}` : "跟随最新态势"}</span></div>
        </aside>
      </div>
      <div className="rounded-xl border bg-card">
        <LiveStreamPanel cursor={state.cursor} mode={state.mode} selection={state.selection} snapshot={viewSnapshot} />
      </div>
      <OperationDiagnostics items={viewSnapshot.diagnostics ?? []} />
      <ProjectTimeline cursor={state.cursor} onCursorChange={(cursor) => dispatch({ type: "set-cursor", cursor })} onRangeChange={(from, to) => dispatch({ type: "set-range", from, to })} onReturnLive={() => dispatch({ type: "return-live" })} onSelect={(selection) => dispatch({ type: "select", selection })} range={state.range} snapshot={viewSnapshot} />
    </div>
  );
}
