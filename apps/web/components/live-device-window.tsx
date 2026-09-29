"use client";

import { useEffect, useRef, useState } from "react";
import { PlayIcon, RefreshCwIcon } from "lucide-react";
import { apiFetch } from "@/lib/api-client";
import { activeProjectStreams, deviceHasLiveSignal } from "@/lib/realtime-workbench-core";
import type { ProjectSituationSnapshot, ProjectSnapshotDevice } from "@/lib/project-snapshot-core";
import { LiveStreamPanel } from "@/components/live-stream-panel";
import { Button } from "@/components/ui/button";

export function LiveDeviceWindow({ snapshot, device, selectedStreamId, autoStart = false, onStarted, onChanged }: {
 snapshot: ProjectSituationSnapshot; device: ProjectSnapshotDevice; selectedStreamId?: number | null; autoStart?: boolean;
 onStarted: (session: Record<string, unknown> & { id: number; status: string }) => void;
 onChanged: () => Promise<void>;
}) {
 const channels = (device.channels ?? []).filter(channel => channel.dataType === "video");
 const streams = activeProjectStreams(snapshot).filter(stream => Number(stream.deviceId) === Number(device.id));
 const preferred = streams.find(stream => Number(stream.id) === selectedStreamId) ?? streams[0];
 const [channelKey, setChannelKey] = useState("");
 const channel = channels.find(item => item.channelKey === channelKey)
  ?? channels.find(item => item.channelKey === preferred?.streamKey)
  ?? channels.find(item => item.availability === "available") ?? channels[0];
 const stream = streams.find(item => item.streamKey === channel?.channelKey) ?? (!channelKey ? preferred : null);
 const action = device.capabilities?.find(capability => capability.code === "stream.video.control")?.actions.find(action => action.kind === "live");
 const [pending, setPending] = useState(false), [error, setError] = useState("");
 const attempts = useRef(new Set<string>()), suppressed = useRef(false), starting = useRef(false);
 const autoStartDeadline = useRef(Date.now() + 5 * 60_000);
 const signal = deviceHasLiveSignal(snapshot, Number(device.id));
 const enabled = Boolean(action?.enabled && channel?.availability === "available");
 async function start(automatic = false) {
  if (!channel || !enabled || starting.current || stream) return;
  starting.current = true; setPending(true); setError("");
  try {
   const response = await apiFetch(`/api/projects/${snapshot.project.id}/devices/${device.id}/live-streams`, {
    method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ streamKey: channel.channelKey })
   });
   const result = await response.json();
   if (!response.ok || !result.session) throw new Error(typeof result.error === "string" ? result.error : "连接直播失败");
   onStarted(result.session);
  } catch (cause) { setError(`${automatic ? "自动连接失败" : "启动失败"}：${cause instanceof Error ? cause.message : "请重试"}`); }
  finally { starting.current = false; setPending(false); }
 }
 useEffect(() => {
  if (stream || suppressed.current || !enabled || !channel) return;
  if (!signal && !(autoStart && Date.now() < autoStartDeadline.current)) return;
  if (attempts.current.has(channel.channelKey)) return;
  attempts.current.add(channel.channelKey);
  void start(true);
 }, [stream?.id, channel?.channelKey, enabled, signal, autoStart]);
 return <div className="overflow-hidden rounded-xl border bg-card">
  <div className="flex flex-wrap items-center justify-between gap-2 border-b px-4 py-2">
   <h2 className="text-sm font-medium">{String(device.name)} · {String(device.typeName ?? "直播")}</h2>
   {channels.length > 1 && <select aria-label={`${device.name} 视频通道`} title={streams.length ? "停止当前直播后可切换通道" : "选择视频通道"} className="max-w-full rounded border bg-background px-2 py-1 text-xs" value={channel?.channelKey ?? ""} disabled={pending || streams.length > 0} onChange={event => { setChannelKey(event.target.value); setError(""); suppressed.current = false; }}>
    {channels.map(item => <option key={item.stableChannelId} value={item.channelKey}>{item.displayName}</option>)}
   </select>}
  </div>
  {stream ? <LiveStreamPanel compact key={String(stream.id)} snapshot={snapshot} selectedStreamId={Number(stream.id)} selection={{ lane: `device-${String(device.category ?? "ground")}`, entityId: String(device.id), label: String(device.name) }} mode="live" cursor={null} onStreamChanged={async () => { suppressed.current = true; await onChanged(); }} />
   : <div className="flex aspect-video flex-col items-center justify-center gap-3 bg-muted/30 p-4 text-center text-sm">
    <p className="text-muted-foreground">{pending ? signal ? "正在连接直播…" : "正在启动直播…" : device.status !== "online" ? "设备尚未在线，等待设备连接" : "暂无直播信号"}</p>
    {channel && <Button size="sm" variant="outline" disabled={!enabled || pending} onClick={() => { suppressed.current = false; void start(); }}>{pending ? <RefreshCwIcon className="size-4 animate-spin"/> : <PlayIcon className="size-4"/>}{signal ? "连接直播" : "启动直播"}</Button>}
    {!enabled && device.status === "online" && <p className="text-xs text-muted-foreground">{action?.unavailableReason ?? channel?.availabilityReason ?? "当前没有可用的视频通道"}</p>}
    {error && <p role="alert" className="text-xs text-destructive">{error}</p>}
   </div>}
 </div>;
}
