"use client";

import { useState } from "react";
import { apiJSON, APIError } from "@/lib/api-client";
import { Button } from "@/components/ui/button";
import type { MissionAction } from "@/lib/mission-workbench-core";

const labels: Record<MissionAction, string> = {
  pause: "暂停", resume: "恢复", cancel: "取消并返航", emergency_stop: "紧急停止", approve: "批准"
};

export function MissionControlButtons({ projectId, taskRunId, stateVersion, actions, onChanged }: {
  projectId: number; taskRunId: number; stateVersion: number; actions: MissionAction[]; onChanged: () => void;
}) {
  const [pending, setPending] = useState<MissionAction | null>(null);
  const [error, setError] = useState<string | null>(null);
  async function invoke(action: MissionAction) {
    setPending(action); setError(null);
    try { await apiJSON(`/api/projects/${projectId}/task-runs/${taskRunId}/control`, {
      method: "POST", headers: { "content-type": "application/json" },
      body: JSON.stringify({ action, expectedVersion: stateVersion, reason: `operator_${action}` })
    });
    onChanged();
    } catch (error) { setError(error instanceof APIError ? error.code : "操作失败，请稍后重试。"); }
    finally { setPending(null); }
  }
  return <div className="space-y-2">
    <div className="flex flex-wrap gap-2">{actions.map((action) => <Button key={action} disabled={pending !== null}
      onClick={() => invoke(action)} variant={action === "emergency_stop" ? "destructive" : "outline"}>{labels[action]}</Button>)}</div>
    {error ? <p className="text-sm text-destructive">{error}</p> : null}
  </div>;
}
