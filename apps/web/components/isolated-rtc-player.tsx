"use client";

import { useEffect, useRef } from "react";
import type { CapturedVideoFrame } from "@/lib/video-frame";

export function captureRTCFrame(frame: HTMLIFrameElement, signal: AbortSignal): Promise<CapturedVideoFrame> {
  signal.throwIfAborted();
  return new Promise((resolve, reject) => {
    const requestId = crypto.randomUUID();
    const cleanup = () => { clearTimeout(timer); window.removeEventListener("message", receive); signal.removeEventListener("abort", abort); };
    const abort = () => { cleanup(); reject(signal.reason); };
    const receive = (event: MessageEvent) => {
      if (event.origin !== window.location.origin || event.source !== frame.contentWindow || event.data?.type !== "aerosight.rtc.frame" || event.data.requestId !== requestId) return;
      cleanup();
      const result = event.data.frame;
      if (event.data.error || !(result?.blob instanceof Blob) || result.blob.size > 2 * 1024 * 1024) { reject(new Error(event.data.error ?? "抽帧响应无效")); return; }
      resolve(result);
    };
    const timer = setTimeout(() => { cleanup(); reject(new Error("直播抽帧超时")); }, 10_000);
    window.addEventListener("message", receive); signal.addEventListener("abort", abort, { once: true });
    frame.contentWindow?.postMessage({ type: "aerosight.rtc.capture", requestId }, window.location.origin);
  });
}

export function IsolatedRTCPlayer({ credential }: { credential: string }) {
  const frame = useRef<HTMLIFrameElement>(null);
  useEffect(() => {
    const send = () => frame.current?.contentWindow?.postMessage({ type: "aerosight.rtc.credential", credential }, window.location.origin);
    const ready = (event: MessageEvent) => {
      if (event.origin === window.location.origin && event.source === frame.current?.contentWindow && event.data?.type === "aerosight.rtc.ready") send();
    };
    window.addEventListener("message", ready);
    send();
    return () => window.removeEventListener("message", ready);
  }, [credential]);
  return <iframe ref={frame} src="/rtc-viewer/" title="RTC 视频播放" allow="autoplay; fullscreen" className="h-full w-full border-0" />;
}
