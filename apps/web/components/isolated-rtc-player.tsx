"use client";

import { useEffect, useRef } from "react";

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
