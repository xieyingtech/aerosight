"use client";

import { useEffect, useRef, useState } from "react";
import { RefreshCwIcon, VideoOffIcon } from "lucide-react";

import { parseVolcRTCPlaybackCredential } from "@/lib/volc-rtc-player-core";

let volcRTCCleanupTail = Promise.resolve();

function enqueueVolcRTCCleanup(release: () => Promise<void>) {
  const run = async () => { try { await release(); } catch { /* cleanup is best effort */ } };
  volcRTCCleanupTail = volcRTCCleanupTail.then(run, run);
  return volcRTCCleanupTail;
}

export function VolcRTCPlayer({ credential }: { credential: string }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [status, setStatus] = useState<"joining" | "waiting" | "playing" | "error">("joining");
  const [errorCode, setErrorCode] = useState<string | null>(null);
  const [connectionState, setConnectionState] = useState<string | null>(null);
  const [viewerAttempt, setViewerAttempt] = useState(0);

  useEffect(() => {
    let disposed = false;
    let cleanup: (() => Promise<void>) | null = null;
    setStatus("joining");
    setErrorCode(null);
    setConnectionState(null);
    void (async () => {
      try {
        await volcRTCCleanupTail;
        if (disposed) return;
        const parsed = parseVolcRTCPlaybackCredential(credential);
        const rtc = await import("@volcengine/rtc");
        if (disposed || !containerRef.current) return;
        rtc.default.setLogConfig({ logLevel: "error" });
        const engine = rtc.default.createEngine(parsed.appId);
        cleanup = async () => {
          try { await engine.leaveRoom(); } catch { /* already left */ }
          rtc.default.destroyEngine(engine);
        };
        engine.on(rtc.default.events.onVideoFirstFrameDecoded, () => {
          if (!disposed) setStatus("playing");
        });
        engine.on(rtc.default.events.onConnectionStateChanged, ({ state }) => {
          if (!disposed) setConnectionState(rtc.ConnectionState[state] ?? "CONNECTION_STATE_UNKNOWN");
        });
        engine.on(rtc.default.events.onUserPublishStream, ({ userId, mediaType }) => {
          if (disposed || !containerRef.current) return;
          if ((mediaType & rtc.MediaType.VIDEO) !== rtc.MediaType.VIDEO) return;
          engine.setRemoteVideoPlayer(rtc.StreamIndex.STREAM_INDEX_MAIN, {
            userId, renderDom: containerRef.current
          });
          void engine.subscribeStream(userId, rtc.MediaType.VIDEO).catch(() => {
            if (!disposed) {
              setErrorCode("VIDEO_SUBSCRIBE_FAILED");
              setStatus("error");
            }
          });
          setStatus("waiting");
        });
        engine.on(rtc.default.events.onError, ({ errorCode: code }) => {
          if (!disposed) {
            setErrorCode(code);
            setStatus("error");
          }
        });
        let joinTimeout: ReturnType<typeof setTimeout> | null = null;
        await engine.setUserVisibility(false);
        const joined = engine.joinRoom(parsed.token, parsed.roomId, { userId: parsed.userId }, {
          isAutoPublish: false, isAutoSubscribeAudio: false, isAutoSubscribeVideo: false
        });
        await Promise.race([joined, new Promise<never>((_, reject) => {
          joinTimeout = setTimeout(() => reject(new Error("VOLC_RTC_JOIN_TIMEOUT")), 15_000);
        })]).finally(() => { if (joinTimeout) clearTimeout(joinTimeout); });
        if (!disposed) setStatus("waiting");
      } catch (error) {
        const release = cleanup;
        cleanup = null;
        if (release) await enqueueVolcRTCCleanup(release);
        if (!disposed) {
          setErrorCode(error instanceof Error && error.message === "VOLC_RTC_JOIN_TIMEOUT"
            ? "JOIN_TIMEOUT" : "CLIENT_INIT_FAILED");
          setStatus("error");
        }
      }
    })();
    return () => {
      disposed = true;
      const release = cleanup;
      cleanup = null;
      if (release) void enqueueVolcRTCCleanup(release);
    };
  }, [credential, viewerAttempt]);

  const viewerError = errorCode === "JOIN_TIMEOUT"
    ? "司空直播已启动，但当前浏览器未建立 RTC 观看连接。"
    : "司空直播已启动，但当前浏览器的 RTC 观看连接失败。";

  return <div className="relative h-full w-full">
    <div className="h-full w-full" ref={containerRef} />
    {status !== "playing" && <div className="absolute inset-0 flex items-center justify-center bg-slate-950 text-center text-xs text-slate-200">
      {status === "error" ? <div className="max-w-sm px-4">
        <VideoOffIcon className="mx-auto mb-2 size-7" />
        <p>{viewerError}{errorCode ? `（${errorCode}）` : ""}</p>
        <button className="mt-3 inline-flex items-center gap-1.5 rounded-md border border-slate-500 px-2.5 py-1.5 text-xs hover:bg-slate-800" onClick={() => setViewerAttempt((current) => current + 1)} type="button">
          <RefreshCwIcon className="size-3.5" />重试观看
        </button>
      </div>
        : <div><RefreshCwIcon className="mx-auto mb-2 size-7 animate-spin" />{status === "joining" ? `正在加入 RTC 房间…${connectionState ? `（${connectionState}）` : ""}` : "已加入，等待 Dock 视频流…"}</div>}
    </div>}
  </div>;
}
