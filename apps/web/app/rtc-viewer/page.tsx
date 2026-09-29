"use client";

import { useEffect, useState } from "react";
import { VolcRTCPlayer } from "@/components/volc-rtc-player";

// Each supplier player owns its own SDK realm. Credentials are delivered only
// by the same-origin parent; they never enter URLs, history, or storage.
export default function RTCViewerPage() {
  const [credential, setCredential] = useState<string | null>(null);
  useEffect(() => {
    if (window.parent === window) return;
    const receive = (event: MessageEvent) => {
      if (event.origin !== window.location.origin || event.source !== window.parent || event.data?.type !== "aerosight.rtc.credential") return;
      const value = event.data.credential;
      if (typeof value === "string" && value.length > 0 && value.length <= 16_384) setCredential(value);
    };
    window.addEventListener("message", receive);
    window.parent.postMessage({ type: "aerosight.rtc.ready" }, window.location.origin);
    return () => window.removeEventListener("message", receive);
  }, []);
  return <main className="h-dvh w-full bg-slate-950 text-slate-200">{credential
    ? <VolcRTCPlayer credential={credential} />
    : <div className="flex h-full items-center justify-center text-xs">正在连接播放器…</div>}</main>;
}
