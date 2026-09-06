"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { apiJSON, APIError } from "@/lib/api-client";
import { Button } from "@/components/ui/button";

export function AlgorithmRunRetryButton({ projectId, runId }: { projectId: number; runId: string }) {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  async function retry() {
    setPending(true); setError(null);
    try {
      const result = await apiJSON<{runId: string}>(`/api/projects/${projectId}/algorithm-runs/${runId}/retry`, { method: "POST" });
      if (!result.runId) { setError("重试失败"); return; }
      router.push(`/projects/algorithms/runs/detail/?projectId=${projectId}&runId=${encodeURIComponent(result.runId)}`);
    } catch (error) { setError(error instanceof APIError ? error.code : "重试失败，请稍后重试。"); }
    finally { setPending(false); }
  }
  return <div className="space-y-1"><Button disabled={pending} onClick={retry}>{pending ? "正在创建…" : "重试运行"}</Button>{error ? <p className="text-xs text-destructive">{error}</p> : null}</div>;
}
