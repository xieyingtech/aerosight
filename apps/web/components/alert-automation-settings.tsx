"use client";

import { useState } from "react";

import type { AlertAutomationMode } from "@/lib/alert-automation-policy-core";

export function AlertAutomationSettings({
  projectId,
  currentMode
}: {
  projectId: number;
  currentMode: AlertAutomationMode;
}) {
  const [mode, setMode] = useState(currentMode);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function saveMode(nextMode: AlertAutomationMode) {
    const previousMode = mode;
    setMode(nextMode);
    setSaving(true);
    setError(null);
    try {
      const response = await fetch(`/api/projects/${projectId}/alert-automation`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ action: "save", mode: nextMode })
      });
      if (!response.ok) {
        setMode(previousMode);
        setError((await response.json()).error ?? "保存失败");
      }
    } catch {
      setMode(previousMode);
      setError("保存失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="rounded-xl border p-4">
      <div className="flex items-center justify-between gap-4">
        <h2 className="font-medium">自动 AI</h2>
        <select
          aria-label="自动 AI 模式"
          className="h-8 w-64 max-w-full rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none transition-colors focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:opacity-50"
          disabled={saving}
          onChange={(event) => saveMode(event.target.value as AlertAutomationMode)}
          value={mode}
        >
          <option value="manual">仅人工处理</option>
          <option value="agent-on-demand">按需生成 AI 草案</option>
          <option value="agent-auto-draft">自动生成 AI 草案</option>
          <option value="follow-up-draft">生成后续处置草案</option>
        </select>
      </div>
      {error ? <p className="mt-2 text-sm text-destructive">{error}</p> : null}
    </section>
  );
}
