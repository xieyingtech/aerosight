"use client";

import { useEffect, useMemo, useState } from "react";
import { DownloadIcon, HistoryIcon, RadioTowerIcon, RefreshCwIcon, VideoOffIcon } from "lucide-react";

import { createLiveStreamPanelModel } from "@/lib/live-stream-panel-model";
import type { ProjectSituationSnapshot } from "@/lib/project-snapshot-core";
import type { SituationSelection } from "@/lib/situation-state";

type PlaybackState =
  | { status: "idle" | "loading" | "error"; candidates?: never; index?: never }
  | { status: "ready"; candidates: { protocol: "webrtc" | "hls" | "simulator"; url: string }[]; index: number };

function HistoricalMedia({ projectId, media }: { projectId: number; media: Record<string, unknown> }) {
  const [accessUrl, setAccessUrl] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  const mimeType = String(media.mimeType ?? "application/octet-stream");
  const action = mimeType.startsWith("video/") ? "play" : "preview";
  useEffect(() => {
    const controller = new AbortController();
    setFailed(false);
    fetch(`/api/projects/${projectId}/assets/${String(media.id)}/access?action=${action}`, {
      signal: controller.signal, cache: "no-store"
    }).then(async (response) => {
      if (!response.ok) throw new Error("media access failed");
      const result = await response.json() as { url: string };
      setAccessUrl(result.url);
    }).catch((error) => { if (error?.name !== "AbortError") setFailed(true); });
    return () => controller.abort();
  }, [action, media.id, projectId]);
  const download = async () => {
    const response = await fetch(`/api/projects/${projectId}/assets/${String(media.id)}/access?action=download`, { cache: "no-store" });
    if (!response.ok) { setFailed(true); return; }
    const result = await response.json() as { url: string };
    window.location.assign(result.url);
  };
  return <div className="space-y-2">
    <div className="flex aspect-video items-center justify-center overflow-hidden rounded-lg border bg-muted/40 text-center text-xs text-muted-foreground">
      {failed ? <div><VideoOffIcon className="mx-auto mb-2 size-7" />媒体不可用或无访问权限</div>
        : !accessUrl ? <RefreshCwIcon className="size-5 animate-spin" />
          : mimeType.startsWith("image/") ? <img alt="历史巡检媒体" className="h-full w-full object-contain" src={accessUrl} />
            : mimeType.startsWith("video/") ? <video className="h-full w-full object-contain" controls src={accessUrl} />
              : <div>媒体 #{String(media.id)} · {mimeType}</div>}
    </div>
    <button className="inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1.5 text-xs" onClick={download} type="button">
      <DownloadIcon className="size-3.5" />下载
    </button>
  </div>;
}

export function LiveStreamPanel({ snapshot, selection, mode, cursor }: {
  snapshot: ProjectSituationSnapshot;
  selection: SituationSelection | null;
  mode: "live" | "history";
  cursor: string | null;
}) {
  const model = useMemo(() => createLiveStreamPanelModel({ snapshot, selection, mode, cursor }), [snapshot, selection, mode, cursor]);
  const [playback, setPlayback] = useState<PlaybackState>({ status: "idle" });
  const streamId = model.mode === "live" ? Number(model.stream?.id) || null : null;
  const selectedDeviceId = selection?.lane.includes("device") ? Number(selection.entityId)
    : Number(model.stream?.deviceId) || null;
  const channels = useMemo(() => (snapshot.realtimeChannels ?? [])
    .filter((channel) => Number(channel.deviceId) === selectedDeviceId), [selectedDeviceId, snapshot.realtimeChannels]);
  const dataChannels = channels.filter((channel) => channel.dataType !== "video" && channel.dataType !== "audio");
  const [activeChannelId, setActiveChannelId] = useState<string | null>(null);
  const activeChannel = dataChannels.find((channel) => String(channel.stableChannelId) === activeChannelId)
    ?? dataChannels[0] ?? null;
  const [sample, setSample] = useState<Record<string, unknown> | null>(null);

  useEffect(() => {
    if (!activeChannel) { setActiveChannelId(null); setSample(null); return; }
    const channelId = String(activeChannel.stableChannelId);
    setActiveChannelId(channelId);
    setSample(activeChannel.latestPayload && typeof activeChannel.latestPayload === "object"
      ? activeChannel.latestPayload as Record<string, unknown> : null);
    const source = new EventSource(`/api/projects/${snapshot.project.id}/realtime-channels/${encodeURIComponent(channelId)}/events`);
    source.addEventListener("channel.sample", (event) => {
      const value = JSON.parse((event as MessageEvent).data) as { payload?: Record<string, unknown> };
      if (value.payload) setSample(value.payload);
    });
    source.addEventListener("access.revoked", () => source.close());
    source.addEventListener("stream.closed", () => source.close());
    return () => source.close();
  }, [activeChannel?.stableChannelId, snapshot.project.id]);

  useEffect(() => {
    if (!streamId) { setPlayback({ status: "idle" }); return; }
    const controller = new AbortController();
    setPlayback({ status: "loading" });
    fetch(`/api/projects/${snapshot.project.id}/live-streams/${streamId}/playback`, {
      signal: controller.signal, cache: "no-store"
    }).then(async (response) => {
      if (!response.ok) throw new Error("playback request failed");
      const result = await response.json() as {
        available: boolean;
        locator?: { url: string; expiresAt: string };
        playback?: { candidates: { protocol: "webrtc" | "hls"; url: string }[]; expiresAt: string };
      };
      const candidates = result.playback?.candidates
        ?? (result.locator ? [{ protocol: "simulator" as const, url: result.locator.url }] : []);
      if (!result.available || candidates.length === 0) throw new Error("playback unavailable");
      setPlayback({ status: "ready", candidates, index: 0 });
    }).catch((error) => { if (error?.name !== "AbortError") setPlayback({ status: "error" }); });
    return () => controller.abort();
  }, [snapshot.project.id, streamId]);

  if (model.mode === "history") {
    return <div className="space-y-3 p-4">
      <div className="flex items-center gap-2 text-sm font-medium"><HistoryIcon className="size-4" />历史媒体</div>
      {model.media ? <HistoricalMedia media={model.media} projectId={snapshot.project.id} />
        : <div className="flex aspect-video items-center justify-center rounded-lg border bg-muted/40 text-center text-xs text-muted-foreground">
          <div><VideoOffIcon className="mx-auto mb-2 size-7" />当前时间点没有可用媒体</div>
        </div>}
    </div>;
  }

  const realtimeData = activeChannel ? <section className="space-y-2 border-t pt-3">
    <div className="flex flex-wrap gap-2">
      {dataChannels.map((channel) => <button className={`rounded-md border px-2.5 py-1 text-xs ${String(channel.stableChannelId) === String(activeChannel.stableChannelId) ? "bg-primary text-primary-foreground" : ""}`} key={String(channel.stableChannelId)} onClick={() => setActiveChannelId(String(channel.stableChannelId))} type="button">
        {String(channel.displayName)} · {String(channel.dataType)}
      </button>)}
    </div>
    <div className="rounded-lg border bg-muted/20 p-3">
      <div className="mb-2 flex items-center justify-between text-xs"><span>{String(activeChannel.displayName)}</span><span className="text-muted-foreground">{String(activeChannel.unit ?? "无统一单位")} · {String(activeChannel.availability)}</span></div>
      <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-all text-xs text-muted-foreground">{sample ? JSON.stringify(sample, null, 2) : "等待实时数据…"}</pre>
    </div>
  </section> : null;

  if (!model.stream) return <div className="flex flex-1 flex-col items-center justify-center p-8 text-center">
    <VideoOffIcon className="mb-3 size-8 text-muted-foreground" />
    <p className="text-sm font-medium">没有可用直播</p>
    <p className="mt-1 text-xs text-muted-foreground">选择具有活动直播的在线设备。</p>
    {realtimeData}
  </div>;

  const sourceType = String(model.stream.sourceType ?? "unknown");
  const status = String(model.stream.status);
  const lastActive = model.stream.lastActiveAt ? Date.parse(String(model.stream.lastActiveAt)) : NaN;
  const latencySeconds = Number.isFinite(lastActive) ? Math.max(0, Math.round((Date.now() - lastActive) / 1000)) : null;
  return <div className="space-y-3 p-4">
    <div className="flex items-center justify-between text-sm font-medium">
      <span className="flex items-center gap-2"><RadioTowerIcon className="size-4" />设备 #{String(model.stream.deviceId)}</span>
      <span className={status === "degraded" ? "text-amber-600" : "text-emerald-600"}>{status}</span>
    </div>
    <div className="flex aspect-video items-center justify-center overflow-hidden rounded-lg border bg-slate-950 text-slate-200">
      {playback.status === "loading" ? <RefreshCwIcon className="size-6 animate-spin" />
        : playback.status === "error" ? <div className="text-center text-xs"><VideoOffIcon className="mx-auto mb-2 size-7" />直播连接失败或 locator 已过期</div>
          : playback.status === "ready" && playback.candidates[playback.index]?.protocol === "webrtc"
            ? <iframe allow="autoplay; fullscreen" className="h-full w-full border-0" src={playback.candidates[playback.index].url} title="WebRTC 直播" />
          : playback.status === "ready" && playback.candidates[playback.index]?.protocol === "hls"
            ? <video autoPlay className="h-full w-full object-contain" controls muted onError={() => setPlayback((current) => current.status === "ready" && current.index + 1 < current.candidates.length ? { ...current, index: current.index + 1 } : { status: "error" })} src={playback.candidates[playback.index].url} />
            : <div className="text-center text-xs"><RadioTowerIcon className="mx-auto mb-2 size-8 animate-pulse" />Simulator 直播信号<br />{String(model.stream.streamKey)}</div>}
    </div>
    <p className="text-xs text-muted-foreground">{latencySeconds === null ? "等待首帧时间" : `最后活动约 ${latencySeconds} 秒前`} · {sourceType}</p>
    {playback.status === "ready" && playback.index + 1 < playback.candidates.length && <button className="rounded-md border px-2.5 py-1 text-xs" onClick={() => setPlayback({ ...playback, index: playback.index + 1 })} type="button">切换备用协议</button>}
    {realtimeData}
  </div>;
}
