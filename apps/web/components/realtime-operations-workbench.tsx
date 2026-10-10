"use client";

import { apiFetch } from "@/lib/api-client";

import { InfoIcon, MapPinOffIcon, RefreshCwIcon, WrenchIcon, PanelRightCloseIcon, PanelRightOpenIcon, PlusIcon, VideoIcon } from "lucide-react";
import { projectPageHref, scopedPageQuery } from "@/lib/page-routes";
import { useCallback, useEffect, useRef, useState } from "react";

import { MonitorBoard } from "@/components/monitor-board";
import { projectMonitorViews, videoViewId, type MonitorView } from "@/lib/monitor-layout";
import { Button } from "@/components/ui/button";
import { DeviceActionPanel } from "@/components/device-action-panel";
import { FlightHubDeviceOperations } from "@/components/flighthub-device-operations";
import { LiveDeviceWindow } from "@/components/live-device-window";
import { OperationDiagnostics } from "@/components/operation-diagnostics";
import { ProjectMap } from "@/components/project-map";
import { ProjectTimeline } from "@/components/project-timeline";
import { Badge } from "@/components/ui/badge";
import { InputSelect } from "@/components/ui/input-select";
import type { ProjectSituationSnapshot } from "@/lib/project-snapshot-core";
import {
  findProjectDevice, liveStreamPollDecision, realtimeDeviceModules, resolveWorkbenchSelection,
  type RealtimeWorkbenchSelection
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

export function RealtimeOperationsWorkbench({ initialSnapshot, initialDeviceId, initialStreamId, autoLive = false }: {
  initialSnapshot: ProjectSituationSnapshot;
  initialDeviceId?: string | null;
  initialStreamId?: string | null;
  autoLive?: boolean;
}) {
  const [snapshot, setSnapshot] = useState(initialSnapshot);
  const [selection, setSelection] = useState<RealtimeWorkbenchSelection>(() => resolveWorkbenchSelection(initialSnapshot, {
    deviceId: initialDeviceId, streamId: initialStreamId
  }));
  const [addedViews, setAddedViews] = useState<string[]>([]);
  const [viewsLoaded, setViewsLoaded] = useState(false);
  const watchedKey = `aerosight:monitor-views:${initialSnapshot.project.id}`;
  useEffect(() => {
    try {
      const saved: unknown = JSON.parse(localStorage.getItem(watchedKey) ?? "[]");
      if (Array.isArray(saved) && saved.every(id => typeof id === "string")) setAddedViews(saved);
    } catch { /* Monitoring remains available without browser storage. */ }
    setViewsLoaded(true);
  }, [watchedKey]);
  useEffect(() => {
    if (viewsLoaded) try { localStorage.setItem(watchedKey, JSON.stringify(addedViews)); } catch { /* Optional persistence. */ }
  }, [addedViews, viewsLoaded, watchedKey]);
  const [focusedViewId, setFocusedViewId] = useState<string | null>(null);
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [liveControls, setLiveControls] = useState<HTMLDivElement | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [transitionTimeout, setTransitionTimeout] = useState(false);
  const pollCount = useRef(0);
  const autoLiveDeviceId = useRef(selection.deviceId);

  const syncSelection = useCallback((next: RealtimeWorkbenchSelection, replace = true) => {
    setSelection(next);
    const query = new URLSearchParams(typeof window !== "undefined" ? window.location.search : "");
    for (const key of ["projectId", "deviceId", "streamId"]) query.delete(key);
    if (next.deviceId) query.set("deviceId", String(next.deviceId));
    if (next.streamId) query.set("streamId", String(next.streamId));
    const href = projectPageHref(snapshot.project.id, "realtime", Object.fromEntries(query));
    if (replace && typeof window !== "undefined" && `${window.location.pathname}${window.location.search}` !== href) {
      window.history.replaceState(window.history.state, "", href);
    }
  }, [snapshot.project.id]);

  const refresh = useCallback(async (selectStreamId?: number) => {
    setRefreshing(true);
    try {
      const response = await apiFetch(`/api/projects/${snapshot.project.id}/snapshot`, { cache: "no-store" });
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
    const pollingSnapshot = snapshot;
    const decision = liveStreamPollDecision(pollingSnapshot, pollCount.current);
    if (decision === "stable") { pollCount.current = 0; setTransitionTimeout(false); }
    if (decision === "timeout") setTransitionTimeout(true);
    const timer = window.setInterval(async () => {
      pollCount.current += 1;
      if (liveStreamPollDecision(pollingSnapshot, pollCount.current) === "timeout") {
        setTransitionTimeout(true);
      }
      await refresh();
    }, decision === "poll" ? 2000 : 5000);
    return () => window.clearInterval(timer);
  }, [refresh, snapshot, selection.deviceId]);

  useEffect(() => {
    const onPopState = () => {
      const params = scopedPageQuery(window.location.pathname, window.location.search);
      setSelection(resolveWorkbenchSelection(snapshot, { deviceId: params.get("deviceId"), streamId: params.get("streamId") }));
    };
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, [snapshot]);

  const selectedDevice = findProjectDevice(snapshot, selection.deviceId);
  const modules = realtimeDeviceModules(selectedDevice);
  const mapSelection: SituationSelection | null = selectedDevice ? {
    lane: "device-generic",
    entityId: String(selectedDevice.id), label: String(selectedDevice.name ?? `设备 #${selectedDevice.id}`)
  } : null;
  const deviceOptions = snapshot.devices.map((device) => ({
    value: String(device.id),
    label: String(device.name ?? `设备 #${device.id}`),
    description: `${String(device.typeName ?? device.category ?? "未分类")}（${String(device.status ?? "unknown")}）`,
    keywords: [String(device.typeKey ?? ""), String(device.driverKey ?? ""), String(device.category ?? "")]
  }));
  const deviceSnapshot = selectedDevice && selection.deviceId ? scopedTimelineSnapshot(snapshot, selection.deviceId) : null;
  const views = projectMonitorViews(snapshot, addedViews);
  const focusedView = views.find(view => view.id === focusedViewId)
    ?? views.find(view => view.deviceId === selection.deviceId && view.streamId === selection.streamId)
    ?? views.find(view => view.deviceId === selection.deviceId);
  const hasActiveStreams = snapshot.liveStreams.length > 0;
  const isFlightHub = selectedDevice?.connectorKey === "dji.flighthub2";
  const actions = (selectedDevice?.capabilities ?? []).flatMap((capability) => capability.actions).filter((action) => action.kind !== "live" && (!isFlightHub || action.kind === "workflow"));
  const diagnostics = selectedDevice && selection.deviceId
    ? (snapshot.diagnostics ?? []).filter((item) => item.deviceId == null || Number(item.deviceId) === selection.deviceId) : [];

  const selectDevice = (deviceId: number) => {
    const next = resolveWorkbenchSelection(snapshot, { deviceId });
    setFocusedViewId(null);
    syncSelection(next);
  };

  const handleStreamStarted = (session: Record<string, unknown> & { id: number; status: string }, deviceId = Number(selectedDevice?.id)) => {
    const optimisticSession = { ...session, deviceId, status: session.status || "requested" };
    setSnapshot((current) => ({
      ...current,
      liveStreams: [optimisticSession, ...current.liveStreams.filter((stream) => Number(stream.id) !== session.id)]
    }));

    window.setTimeout(() => { void refresh(); }, 500);
  };

  const addView = (deviceId: number, channelKey: string) => {
    const id = videoViewId(deviceId, channelKey);
    setAddedViews(current => current.includes(id) ? current : [...current, id]);
    setFocusedViewId(id);
    syncSelection(resolveWorkbenchSelection(snapshot, { deviceId }));
  };
  const focusView = (view: MonitorView) => {
    setFocusedViewId(view.id);
    if (view.deviceId) syncSelection(resolveWorkbenchSelection(snapshot, { deviceId: view.deviceId, streamId: view.streamId }));
  };
  return <div className="flex h-full min-h-0 overflow-hidden bg-background">
    <main className="min-h-0 min-w-0 flex-1">
      <MonitorBoard projectId={snapshot.project.id} views={views} focusedId={focusedView?.id ?? null} onFocus={focusView} renderView={view => {
        if (view.kind === "map") return <ProjectMap compactControls className="h-full min-h-0 rounded-none border-0" onSelect={value => { if (value.lane.startsWith("device-")) selectDevice(Number(value.entityId)); }} selection={mapSelection} snapshot={snapshot} />;
        const device = findProjectDevice(snapshot, view.deviceId ?? null);
        if (!device) return null;
        return <LiveDeviceWindow immersive videoChannelKey={view.channelKey} controlsTarget={liveControls} controlsVisible={view.id === focusedView?.id} snapshot={snapshot} device={device} selectedStreamId={view.streamId} autoStart={autoLive && Number(device.id) === autoLiveDeviceId.current} onStarted={session => handleStreamStarted(session, Number(device.id))} onChanged={async () => { await refresh(); }} />;
      }} />
    </main>
    <aside aria-label="设备与操作" className={sidebarOpen ? "flex h-full w-80 shrink-0 flex-col border-l bg-background" : "flex h-full w-10 shrink-0 flex-col border-l bg-background"}>
      <div className={sidebarOpen ? "flex h-12 shrink-0 items-center justify-between border-b px-4" : "flex h-12 shrink-0 items-center justify-center"}>
        {sidebarOpen && <h2 className="text-sm font-semibold">设备与操作</h2>}
        <Button size="icon-sm" variant="ghost" aria-label={sidebarOpen ? "收起设备与操作" : "展开设备与操作"} aria-expanded={sidebarOpen} onClick={() => setSidebarOpen(open => !open)}>{sidebarOpen ? <PanelRightCloseIcon /> : <PanelRightOpenIcon />}</Button>
      </div>
      <div hidden={!sidebarOpen} className="min-h-0 flex-1 space-y-3 overflow-y-auto p-3">
      <section className="rounded-xl border bg-card p-4">
        <div className="flex items-center justify-between gap-3">
          <div><h2 className="font-medium">作业设备</h2><p className="mt-1 text-xs text-muted-foreground">搜索并选择设备</p></div>
          <Button aria-label="刷新状态" variant="outline" size="icon-sm" disabled={refreshing} onClick={() => refresh()} type="button"><RefreshCwIcon className={`size-3.5 ${refreshing ? "animate-spin" : ""}`} /></Button>
        </div>
        <div className="mt-3 grid items-start gap-3">
          <InputSelect onValueChange={(value) => selectDevice(Number(value))} options={deviceOptions} placeholder="按名称、类型或驱动搜索" value={selection.deviceId ? String(selection.deviceId) : null} />
          {selectedDevice && <div className="space-y-2">
            <div className="flex flex-wrap items-center gap-2 text-sm"><span>{String(selectedDevice.typeName ?? selectedDevice.category ?? "设备")}</span><Badge variant="outline">{String(selectedDevice.status ?? "unknown")}</Badge><span className="text-xs text-muted-foreground">{String(selectedDevice.driverKey ?? "未绑定驱动")}@{String(selectedDevice.driverVersion ?? "-")}</span></div>
            {!hasPosition(selectedDevice) && <p className="flex items-center gap-2 text-xs text-amber-800"><MapPinOffIcon className="size-4" />该设备暂无位置，操作与实时数据仍可使用。</p>}
            <details className="text-xs text-muted-foreground"><summary className="cursor-pointer">设备能力</summary><div className="mt-2 flex flex-wrap gap-1.5">{(selectedDevice.capabilities ?? []).map(capability => <Badge key={capability.code} variant="secondary">{capability.code}</Badge>)}</div></details>
          </div>}
        </div>
        {selectedDevice && <div className="mt-4">
          {(actions.length > 0 || isFlightHub) && <section className="space-y-3 border-t pt-4">
            <h2 className="flex items-center gap-2 font-medium"><WrenchIcon className="size-4" />设备操作</h2>
            {!isFlightHub && actions.length > 0 && <DeviceActionPanel deviceName={selectedDevice.name} actions={actions} deviceId={Number(selectedDevice.id)} onChanged={async () => { await refresh(); }} projectId={snapshot.project.id} />}
            {isFlightHub && <FlightHubDeviceOperations compact workflowActions={actions} key={String(selectedDevice.id)} projectId={snapshot.project.id} deviceId={Number(selectedDevice.id)} deviceName={String(selectedDevice.name)} onChanged={async()=>{await refresh();}} />}
          </section>}</div>}
      </section>
      {selectedDevice && <section className="space-y-2 rounded-xl border bg-card p-3"><h3 className="flex items-center gap-2 text-sm font-medium"><VideoIcon className="size-4" />视频视角</h3>
        {(selectedDevice.channels ?? []).filter(channel => channel.dataType === "video").map(channel => {
          const id = videoViewId(Number(selectedDevice.id), channel.channelKey);
          const present = views.some(view => view.id === id);
          return <Button key={channel.stableChannelId} className="w-full justify-between" variant={present ? "secondary" : "outline"} size="sm" onClick={() => addView(Number(selectedDevice.id), channel.channelKey)}>{channel.displayName}{present ? <span className="text-xs text-muted-foreground">查看操作</span> : <PlusIcon className="size-3.5" />}</Button>;
        })}
        {!modules.live && <p className="text-xs text-muted-foreground">当前设备没有视频能力。</p>}
      </section>}
      <div ref={setLiveControls} className="space-y-3" />
      {hasActiveStreams && transitionTimeout && <p className="rounded-lg border bg-muted p-3 text-xs text-muted-foreground">直播状态长时间未收敛，请检查设备连接后手动刷新。</p>}
      {diagnostics.length ? <OperationDiagnostics compact items={diagnostics} /> : selectedDevice ? <p className="flex items-center gap-2 px-1 text-xs text-muted-foreground"><InfoIcon className="size-3.5" />当前设备没有待处理诊断</p> : null}
      {modules.timeline && deviceSnapshot && <ProjectTimeline snapshot={deviceSnapshot} />}
      </div>
    </aside>
  </div>;
}
