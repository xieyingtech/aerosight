"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";

export function AlgorithmRunRetryButton({ projectId, runId }: { projectId: number; runId: string }) {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  async function retry() {
    setPending(true); setError(null);
    const response = await fetch(`/api/projects/${projectId}/algorithm-runs/${runId}/retry`, { method: "POST" });
    const result = await response.json() as { runId?: string; error?: string };
    setPending(false);
    if (!response.ok || !result.runId) { setError(result.error ?? "重试失败"); return; }
    router.push(`/projects/${projectId}/algorithms/runs/${result.runId}`);
    router.refresh();
  }
  return <div className="space-y-1"><Button disabled={pending} onClick={retry}>{pending ? "正在创建…" : "重试运行"}</Button>{error ? <p className="text-xs text-destructive">{error}</p> : null}</div>;
}
