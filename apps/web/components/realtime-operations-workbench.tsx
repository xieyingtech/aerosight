"use client";

import { apiFetch } from "@/lib/api-client";

import { CrosshairIcon, InfoIcon, MapPinOffIcon, RefreshCwIcon, WrenchIcon } from "lucide-react";
import { projectPageHref, scopedPageQuery } from "@/lib/page-routes";
import { useCallback, useEffect, useRef, useState } from "react";


import { CanvasWorkspace } from "@/components/canvas-workspace";
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
  activeProjectStreams, findProjectDevice, liveStreamPollDecision, realtimeDeviceModules, relatedLiveDevices, resolveWorkbenchSelection,
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
  const [primaryDeviceId, setPrimaryDeviceId] = useState<number | null>(null);
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
    const device = findProjectDevice(snapshot, selection.deviceId);
    const pollingSnapshot = selection.deviceId && realtimeDeviceModules(device).live
      ? snapshot : { ...snapshot, liveStreams: [] };
    const decision = liveStreamPollDecision(pollingSnapshot, pollCount.current);
    if (!selection.deviceId) return;
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
    description: `${String(device.typeName ?? device.category ?? "未分类")} · ${String(device.status ?? "unknown")}`,
    keywords: [String(device.typeKey ?? ""), String(device.driverKey ?? ""), String(device.category ?? "")]
  }));
  const deviceSnapshot = selectedDevice && selection.deviceId ? scopedTimelineSnapshot(snapshot, selection.deviceId) : null;
  const liveDevices = relatedLiveDevices(snapshot, selection.deviceId);
  const primaryDevice = liveDevices.find(device => Number(device.id) === primaryDeviceId)
    ?? liveDevices.find(device => Number(device.id) === Number(snapshot.liveStreams.find(stream => Number(stream.id) === selection.streamId)?.deviceId))
    ?? liveDevices[0];
  const liveDeviceIds = new Set(liveDevices.map(device => Number(device.id)));
  const hasActiveStreams = activeProjectStreams(snapshot).some(stream => liveDeviceIds.has(Number(stream.deviceId)));
  const isFlightHub = selectedDevice?.connectorKey === "dji.flighthub2";
  const actions = (selectedDevice?.capabilities ?? []).flatMap((capability) => capability.actions).filter((action) => action.kind !== "live" && (!isFlightHub || action.kind === "workflow"));
  const diagnostics = selectedDevice && selection.deviceId
    ? (snapshot.diagnostics ?? []).filter((item) => item.deviceId == null || Number(item.deviceId) === selection.deviceId) : [];

  const selectDevice = (deviceId: number) => {
    const next = resolveWorkbenchSelection(snapshot, { deviceId });
    setPrimaryDeviceId(null);
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

  return <CanvasWorkspace title="实时作业" subtitle={primaryDevice ? `${snapshot.project.name} · ${String(primaryDevice.name)}` : snapshot.project.name} leftTitle="设备与操作" left={<div className="space-y-3">
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
      {liveDevices.length > 1 && <div className="space-y-2"><h3 className="text-sm font-medium">主画面设备</h3><div className="flex flex-wrap gap-2">{liveDevices.map(device => <Button key={String(device.id)} size="sm" variant={device.id === primaryDevice?.id ? "secondary" : "outline"} aria-pressed={device.id === primaryDevice?.id} onClick={() => setPrimaryDeviceId(Number(device.id))}>{String(device.name)}</Button>)}</div></div>}
      <div ref={setLiveControls} className="space-y-3" />
    </div>} rightTitle="地图与实时数据" right={<div className="space-y-3">
      <ProjectMap compactControls className="h-60 min-h-0" onSelect={(value) => { if (value.lane.startsWith("device-")) selectDevice(Number(value.entityId)); }} selection={mapSelection} snapshot={snapshot} />
      {!selectedDevice && <section className="flex min-h-96 flex-col items-center justify-center rounded-xl border border-dashed bg-card p-8 text-center"><CrosshairIcon className="mb-3 size-9 text-muted-foreground" /><h2 className="font-medium">选择一台设备开始作业</h2><p className="mt-1 text-sm text-muted-foreground">操作、直播与实时数据会按设备能力显示在这里。</p></section>}
      {modules.live && hasActiveStreams && transitionTimeout && <p className="rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm text-amber-800">直播状态长时间未收敛，请检查设备连接后手动刷新。</p>}
      {diagnostics.length ? <OperationDiagnostics items={diagnostics} /> : selectedDevice ? <section className="rounded-xl border bg-card p-4 text-sm text-muted-foreground"><span className="flex items-center gap-2"><InfoIcon className="size-4" />当前设备没有待处理诊断</span></section> : null}
      {modules.timeline && deviceSnapshot && <ProjectTimeline snapshot={deviceSnapshot} />}
    </div>}>
      <section aria-label="设备直播" className="h-full bg-slate-950">
        {liveDevices.map(device => <div key={String(device.id)} hidden={device.id !== primaryDevice?.id} className="h-full"><LiveDeviceWindow immersive controlsTarget={liveControls} controlsVisible={device.id === primaryDevice?.id} snapshot={snapshot} device={device} selectedStreamId={selection.streamId} autoStart={autoLive && selection.deviceId === autoLiveDeviceId.current} onStarted={session => handleStreamStarted(session, Number(device.id))} onChanged={async () => { await refresh(); }} /></div>)}
        {!liveDevices.length && <div className="flex h-full flex-col items-center justify-center gap-3 p-6 text-center text-slate-200"><CrosshairIcon className="size-10" /><h2 className="font-medium">{selectedDevice ? "当前设备没有可用视频通道" : "选择一台设备开始作业"}</h2><p className="text-sm text-slate-400">在左侧设备面板选择直播设备，地图和实时数据位于右侧。</p></div>}
      </section>
    </CanvasWorkspace>;
}
