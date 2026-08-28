"use client";

import { CrosshairIcon, InfoIcon, MapPinOffIcon, RefreshCwIcon, SearchIcon, WrenchIcon } from "lucide-react";
import { usePathname } from "next/navigation";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { ActiveStreamSwitcher } from "@/components/active-stream-switcher";
import { DeviceActionPanel } from "@/components/device-action-panel";
import { LiveChannelControls } from "@/components/live-channel-controls";
import { LiveStreamPanel } from "@/components/live-stream-panel";
import { OperationDiagnostics } from "@/components/operation-diagnostics";
import { ProjectMap } from "@/components/project-map";
import { ProjectTimeline } from "@/components/project-timeline";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import type { ProjectSituationSnapshot } from "@/lib/project-snapshot-core";
import {
  findProjectDevice, liveStreamPollDecision, resolveWorkbenchSelection,
  type RealtimeWorkbenchSelection, workbenchQuery
} from "@/lib/realtime-workbench-core";
import type { SituationSelection } from "@/lib/situation-state";

function hasPosition(device: Record<string, unknown> | null) {
  const pose = device?.pose as Record<string, unknown> | null | undefined;
  return Number.isFinite(Number(pose?.longitude)) && Number.isFinite(Number(pose?.latitude));
}

function scopedTimelineSnapshot(snapshot: ProjectSituationSnapshot, deviceId: number): ProjectSituationSnapshot {
  return {
    ...snapshot,
    devices: snapshot.devices.filter((item) => Number(item.id) === deviceId),
    tracks: snapshot.tracks.filter((item) => Number(item.deviceId) === deviceId),
    mediaPoints: snapshot.mediaPoints.filter((item) => Number(item.deviceId) === deviceId),
    liveStreams: snapshot.liveStreams.filter((item) => Number(item.deviceId) === deviceId),
    realtimeChannels: (snapshot.realtimeChannels ?? []).filter((item) => Number(item.deviceId) === deviceId),
    diagnostics: (snapshot.diagnostics ?? []).filter((item) => item.deviceId == null || Number(item.deviceId) === deviceId)
  };
}

export function RealtimeOperationsWorkbench({ initialSnapshot, initialDeviceId, initialStreamId }: {
  initialSnapshot: ProjectSituationSnapshot;
  initialDeviceId?: string | null;
  initialStreamId?: string | null;
}) {
  const pathname = usePathname();
  const [snapshot, setSnapshot] = useState(initialSnapshot);
  const [selection, setSelection] = useState<RealtimeWorkbenchSelection>(() => resolveWorkbenchSelection(initialSnapshot, {
    deviceId: initialDeviceId, streamId: initialStreamId
  }));
  const [query, setQuery] = useState("");
  const [refreshing, setRefreshing] = useState(false);
  const [transitionTimeout, setTransitionTimeout] = useState(false);
  const pollCount = useRef(0);

  const syncSelection = useCallback((next: RealtimeWorkbenchSelection, replace = true) => {
    setSelection(next);
    const encoded = workbenchQuery(next);
    const href = encoded ? `${pathname}?${encoded}` : pathname;
    if (replace && typeof window !== "undefined" && `${window.location.pathname}${window.location.search}` !== href) {
      window.history.replaceState(window.history.state, "", href);
    }
  }, [pathname]);

  const refresh = useCallback(async (selectStreamId?: number) => {
    setRefreshing(true);
    try {
      const response = await fetch(`/api/projects/${snapshot.project.id}/snapshot`, { cache: "no-store" });
      if (!response.ok) return null;
      const next = await response.json() as ProjectSituationSnapshot;
      setSnapshot(next);
      const resolved = resolveWorkbenchSelection(next, {
        deviceId: selection.deviceId,
        streamId: selectStreamId ?? selection.streamId
      });
      syncSelection(resolved);
      return next;
    } finally {
      setRefreshing(false);
    }
  }, [selection.deviceId, selection.streamId, snapshot.project.id, syncSelection]);

  useEffect(() => {
    const decision = liveStreamPollDecision(snapshot, pollCount.current);
    if (decision === "stable") { pollCount.current = 0; setTransitionTimeout(false); return; }
    if (decision === "timeout") { setTransitionTimeout(true); return; }
    const timer = window.setInterval(async () => {
      pollCount.current += 1;
      if (liveStreamPollDecision(snapshot, pollCount.current) === "timeout") {
        window.clearInterval(timer);
        setTransitionTimeout(true);
        return;
      }
      await refresh();
    }, 2000);
    return () => window.clearInterval(timer);
  }, [refresh, snapshot]);

  useEffect(() => {
    const onPopState = () => {
      const params = new URLSearchParams(window.location.search);
      setSelection(resolveWorkbenchSelection(snapshot, { deviceId: params.get("deviceId"), streamId: params.get("streamId") }));
    };
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, [snapshot]);

  const selectedDevice = findProjectDevice(snapshot, selection.deviceId);
  const mapSelection: SituationSelection | null = selectedDevice ? {
    lane: `device-${String(selectedDevice.category ?? selectedDevice.type ?? "ground")}`,
    entityId: String(selectedDevice.id), label: String(selectedDevice.name ?? `设备 #${selectedDevice.id}`)
  } : null;
  const filteredDevices = useMemo(() => snapshot.devices.filter((device) => {
    const needle = query.trim().toLowerCase();
    return !needle || [device.name, device.typeName, device.typeKey, device.category, device.driverKey]
      .some((value) => String(value ?? "").toLowerCase().includes(needle));
  }), [query, snapshot.devices]);
  const timelineSnapshot = selectedDevice && selection.deviceId ? scopedTimelineSnapshot(snapshot, selection.deviceId) : null;
  const actions = (selectedDevice?.capabilities ?? []).flatMap((capability) => capability.actions).filter((action) => action.kind !== "live");
  const diagnostics = selectedDevice && selection.deviceId
    ? (snapshot.diagnostics ?? []).filter((item) => item.deviceId == null || Number(item.deviceId) === selection.deviceId) : [];

  const selectDevice = (deviceId: number) => {
    const next = resolveWorkbenchSelection(snapshot, { deviceId });
    syncSelection(next);
  };
  const selectStream = (streamId: number, deviceId: number) => syncSelection({ deviceId, streamId });

  return <div className="space-y-4">
    <ActiveStreamSwitcher onSelect={selectStream} selectedStreamId={selection.streamId} snapshot={snapshot} />
    <section className="rounded-xl border bg-card p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div><h2 className="font-medium">作业设备</h2><p className="mt-1 text-xs text-muted-foreground">搜索所有设备，包括没有地图坐标的设备。</p></div>
        <button className="inline-flex items-center gap-2 rounded-md border px-3 py-2 text-xs disabled:opacity-50" disabled={refreshing} onClick={() => refresh()} type="button"><RefreshCwIcon className={`size-3.5 ${refreshing ? "animate-spin" : ""}`} />刷新状态</button>
      </div>
      <div className="relative mt-3"><SearchIcon className="absolute left-3 top-2.5 size-4 text-muted-foreground" /><Input className="pl-9" onChange={(event) => setQuery(event.target.value)} placeholder="按设备名称、类型或驱动搜索" value={query} /></div>
      <div className="mt-2 flex max-h-40 flex-wrap gap-2 overflow-y-auto">
        {filteredDevices.map((device) => <button className={`rounded-md border px-3 py-2 text-left text-sm ${Number(device.id) === selection.deviceId ? "border-primary bg-primary/5" : "hover:bg-muted/50"}`} key={String(device.id)} onClick={() => selectDevice(Number(device.id))} type="button">
          <span className="font-medium">{String(device.name ?? `设备 #${device.id}`)}</span><span className="ml-2 text-xs text-muted-foreground">{String(device.typeName ?? device.category ?? "未分类")}</span>
        </button>)}
        {!filteredDevices.length && <p className="py-3 text-sm text-muted-foreground">没有匹配的设备</p>}
      </div>
    </section>

    <div className="grid gap-4 xl:grid-cols-[minmax(0,1.35fr)_minmax(340px,.65fr)]">
      <ProjectMap className="h-[620px]" onSelect={(value) => { if (value.lane.startsWith("device-")) selectDevice(Number(value.entityId)); }} selection={mapSelection} snapshot={snapshot} />
      <aside className="space-y-4">
        {selectedDevice ? <>
          <section className="rounded-xl border bg-card p-4">
            <div className="flex items-start justify-between gap-3"><div><p className="text-xs text-muted-foreground">{String(selectedDevice.typeName ?? selectedDevice.category ?? "设备")}</p><h2 className="text-lg font-medium">{String(selectedDevice.name)}</h2><p className="mt-1 text-xs text-muted-foreground">{String(selectedDevice.driverKey ?? "未绑定驱动")}@{String(selectedDevice.driverVersion ?? "-")}</p></div><Badge variant="outline">{String(selectedDevice.status ?? "unknown")}</Badge></div>
            {!hasPosition(selectedDevice) && <p className="mt-3 flex items-center gap-2 rounded-md bg-amber-50 p-2 text-xs text-amber-800"><MapPinOffIcon className="size-4" />该设备暂无位置，操作与实时数据仍可使用。</p>}
            <div className="mt-3 flex flex-wrap gap-1.5">{(selectedDevice.capabilities ?? []).map((capability) => <Badge key={capability.code} variant="secondary">{capability.code}</Badge>)}</div>
          </section>
          {actions.length ? <section className="rounded-xl border bg-card p-4"><h2 className="flex items-center gap-2 font-medium"><WrenchIcon className="size-4" />设备操作</h2><DeviceActionPanel actions={actions} deviceId={Number(selectedDevice.id)} onChanged={async () => { await refresh(); }} projectId={snapshot.project.id} /></section> : null}
          <LiveChannelControls device={selectedDevice} onStarted={async (streamId) => { await refresh(streamId); }} projectId={snapshot.project.id} />
          <section className="overflow-hidden rounded-xl border bg-card"><LiveStreamPanel cursor={null} mode="live" onStreamChanged={async () => { await refresh(); }} selectedStreamId={selection.streamId} selection={mapSelection} snapshot={snapshot} /></section>
        </> : <section className="flex min-h-96 flex-col items-center justify-center rounded-xl border border-dashed bg-card p-8 text-center"><CrosshairIcon className="mb-3 size-9 text-muted-foreground" /><h2 className="font-medium">选择一台设备开始作业</h2><p className="mt-1 text-sm text-muted-foreground">操作、直播与实时数据会按设备能力显示在这里。</p></section>}
      </aside>
    </div>
    {transitionTimeout && <p className="rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm text-amber-800">直播状态长时间未收敛，请检查设备连接后手动刷新。</p>}
    {diagnostics.length ? <OperationDiagnostics items={diagnostics} /> : selectedDevice ? <section className="rounded-xl border bg-card p-4 text-sm text-muted-foreground"><span className="flex items-center gap-2"><InfoIcon className="size-4" />当前设备没有待处理诊断</span></section> : null}
    {timelineSnapshot && <ProjectTimeline snapshot={timelineSnapshot} />}
  </div>;
}
