"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { apiFetch } from "@/lib/api-client";
import { missionLiveHref } from "@/lib/mission-live-core";

export function MissionLiveRedirect({ projectId, run, enabled, onChanged }: {
  projectId: number; run: Record<string, unknown>; enabled: boolean; onChanged: () => void;
}) {
  const router = useRouter();
  const [message, setMessage] = useState("");
  useEffect(() => {
    if (!enabled) return;
    const href = missionLiveHref(projectId, run);
    if (href) { router.replace(href); return; }
    if (["failed", "canceled", "succeeded"].includes(String(run.status))) { setMessage(""); return; }
    setMessage("任务正在准备，司空受理飞行任务后将自动进入实时作业。");
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const deadline = Date.now() + 5 * 60_000;
    const poll = async () => {
      try {
        const response = await apiFetch(`/api/projects/${projectId}/task-runs/${run.id}`, { cache: "no-store", signal: controller.signal });
        if (response.ok) {
          const model = await response.json() as { run: Record<string, unknown> };
          const next = missionLiveHref(projectId, model.run);
          if (next) { router.replace(next); return; }
          if (["failed", "canceled", "succeeded", "blocked"].includes(String(model.run.status))) { setMessage(""); onChanged(); return; }
        } else if ([401, 403, 404].includes(response.status)) { setMessage("无法读取飞行状态，请刷新任务页面。"); return; }
      } catch { if (controller.signal.aborted) return; }
      if (controller.signal.aborted) return;
      if (Date.now() >= deadline) { setMessage("尚未收到司空的任务受理结果，请查看任务状态。"); onChanged(); return; }
      timer = setTimeout(poll, 2000);
    };
    timer = setTimeout(poll, 2000);
    return () => { controller.abort(); clearTimeout(timer); };
  }, [enabled, projectId, run.id, run.status, run.realtimeFlight, router, onChanged]);
  return message ? <p role="status" className="text-sm text-muted-foreground">{message}</p> : null;
}
