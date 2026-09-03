"use client";

import { useEffect, useRef, useState } from "react";
import { RefreshCwIcon, VideoOffIcon } from "lucide-react";

import { parseVolcRTCPlaybackCredential } from "@/lib/volc-rtc-player-core";

export function VolcRTCPlayer({ credential }: { credential: string }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [status, setStatus] = useState<"joining" | "waiting" | "playing" | "error">("joining");

  useEffect(() => {
    let disposed = false;
    let cleanup: (() => Promise<void>) | null = null;
    setStatus("joining");
    void (async () => {
      try {
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
        engine.on(rtc.default.events.onUserPublishStream, ({ userId, mediaType }) => {
          if (disposed || !containerRef.current) return;
          engine.setRemoteVideoPlayer(rtc.StreamIndex.STREAM_INDEX_MAIN, {
            userId, renderDom: containerRef.current
          });
          void engine.subscribeStream(userId, mediaType).catch(() => { if (!disposed) setStatus("error"); });
          setStatus("waiting");
        });
        engine.on(rtc.default.events.onError, () => { if (!disposed) setStatus("error"); });
        await engine.setUserVisibility(false);
        await engine.joinRoom(parsed.token, parsed.roomId, { userId: parsed.userId }, {
          isAutoPublish: false, isAutoSubscribeAudio: false, isAutoSubscribeVideo: false
        });
        if (!disposed) setStatus("waiting");
      } catch {
        if (!disposed) setStatus("error");
      }
    })();
    return () => {
      disposed = true;
      const release = cleanup;
      cleanup = null;
      if (release) void release();
    };
  }, [credential]);

  return <div className="relative h-full w-full">
    <div className="h-full w-full" ref={containerRef} />
    {status !== "playing" && <div className="absolute inset-0 flex items-center justify-center bg-slate-950 text-center text-xs text-slate-200">
      {status === "error" ? <div><VideoOffIcon className="mx-auto mb-2 size-7" />RTC 直播连接失败</div>
        : <div><RefreshCwIcon className="mx-auto mb-2 size-7 animate-spin" />{status === "joining" ? "正在加入 RTC 房间…" : "已加入，等待 Dock 视频流…"}</div>}
    </div>}
  </div>;
}
