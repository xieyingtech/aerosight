"use client";

import { useEffect, useMemo, useState } from "react";
import { DownloadIcon, HistoryIcon, RadioTowerIcon, RefreshCwIcon, VideoOffIcon } from "lucide-react";

import { createLiveStreamPanelModel } from "@/lib/live-stream-panel-model";
import type { ProjectSituationSnapshot } from "@/lib/project-snapshot-core";
import type { SituationSelection } from "@/lib/situation-state";

type PlaybackState =
  | { status: "idle" | "loading" | "error"; locator?: never }
  | { status: "ready"; locator: { url: string; expiresAt: string } };

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

  useEffect(() => {
    if (!streamId) { setPlayback({ status: "idle" }); return; }
    const controller = new AbortController();
    setPlayback({ status: "loading" });
    fetch(`/api/projects/${snapshot.project.id}/live-streams/${streamId}/playback`, {
      signal: controller.signal, cache: "no-store"
    }).then(async (response) => {
      if (!response.ok) throw new Error("playback request failed");
      const result = await response.json() as { available: boolean; locator?: { url: string; expiresAt: string } };
      if (!result.available || !result.locator) throw new Error("playback unavailable");
      setPlayback({ status: "ready", locator: result.locator });
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

  if (!model.stream) return <div className="flex flex-1 flex-col items-center justify-center p-8 text-center">
    <VideoOffIcon className="mb-3 size-8 text-muted-foreground" />
    <p className="text-sm font-medium">没有可用直播</p>
    <p className="mt-1 text-xs text-muted-foreground">选择具有活动直播的在线设备。</p>
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
          : playback.status === "ready" && sourceType !== "simulator"
            ? <video autoPlay className="h-full w-full object-contain" controls muted src={playback.locator.url} />
            : <div className="text-center text-xs"><RadioTowerIcon className="mx-auto mb-2 size-8 animate-pulse" />Simulator 直播信号<br />{String(model.stream.streamKey)}</div>}
    </div>
    <p className="text-xs text-muted-foreground">{latencySeconds === null ? "等待首帧时间" : `最后活动约 ${latencySeconds} 秒前`} · {sourceType}</p>
  </div>;
}
