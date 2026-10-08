"use client";

import { useEffect, useRef, useState } from "react";
import { RefreshCwIcon, VideoOffIcon } from "lucide-react";

import { parseVolcRTCPlaybackCredential } from "@/lib/volc-rtc-player-core";
import { captureVideoFrame } from "@/lib/video-frame";

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
    const receive = (event: MessageEvent) => {
      if (event.origin !== window.location.origin || event.source !== window.parent || event.data?.type !== "aerosight.rtc.capture" || typeof event.data.requestId !== "string") return;
      const requestId = event.data.requestId;
      const video = containerRef.current?.querySelector("video");
      const reply = (value: unknown) => window.parent.postMessage(value, window.location.origin);
      if (!video) { reply({ type: "aerosight.rtc.frame", requestId, error: "直播尚未出现可抽取的视频画面" }); return; }
      void captureVideoFrame(video).then(frame => reply({ type: "aerosight.rtc.frame", requestId, frame }), error => reply({ type: "aerosight.rtc.frame", requestId, error: error.message }));
    };
    window.addEventListener("message", receive);
    return () => window.removeEventListener("message", receive);
  }, []);

  useEffect(() => {
    let disposed = false;
    let cleanup: (() => Promise<void>) | null = null;
    let rtcError: string | null = null;
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
            rtcError = code;
            setErrorCode(code);
            setStatus("error");
          }
        });
        let joinTimeout: ReturnType<typeof setTimeout> | null = null;
        // Keep the default visible viewer presence. DJI must be able to observe
        // join/leave events to maintain its live viewing session. No local media
        // is captured or published by this receive-only engine.
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
          setErrorCode(rtcError ?? (error instanceof Error && error.message === "VOLC_RTC_JOIN_TIMEOUT"
            ? "JOIN_TIMEOUT" : "CLIENT_INIT_FAILED"));
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

  const viewerError = errorCode === "KICKED_OUT"
    ? "RTC 服务端已结束当前观看会话。"
    : errorCode === "JOIN_TIMEOUT"
    ? "直播已启动，但当前浏览器未建立 RTC 观看连接。"
    : "直播已启动，但当前浏览器的 RTC 观看连接失败。";

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
        : <div><RefreshCwIcon className="mx-auto mb-2 size-7 animate-spin" />{status === "joining" ? `正在加入 RTC 房间…${connectionState ? `（${connectionState}）` : ""}` : "已加入，等待设备视频流…"}</div>}
    </div>}
  </div>;
}
