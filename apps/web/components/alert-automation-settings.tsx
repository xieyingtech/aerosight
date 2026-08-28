"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";

import { Button } from "@/components/ui/button";

export function AlertAutomationSettings({
  projectId,
  automaticAi,
  currentMode
}: {
  projectId: number;
  automaticAi: boolean;
  currentMode: string;
}) {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);

  async function call(body: unknown) {
    setError(null);
    const response = await fetch(`/api/projects/${projectId}/alert-automation`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(body)
    });
    if (!response.ok) setError((await response.json()).error ?? "保存失败");
    else router.refresh();
  }

  return (
    <section className="space-y-4 rounded-xl border p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="font-medium">自动 AI</h2>
          <p className="text-sm text-muted-foreground">
            当前：{automaticAi ? "启用" : "关闭"}；关闭后排队和运行中的后续草案都会停止。
          </p>
        </div>
        <Button
          onClick={() => call({ action: "toggle", enabled: !automaticAi })}
          size="sm"
          variant={automaticAi ? "destructive" : "default"}
        >
          {automaticAi ? "关闭自动 AI" : "启用自动 AI"}
        </Button>
      </div>

      <div>
        <h3 className="text-sm font-medium">执行策略</h3>
        <p className="mt-1 text-xs text-muted-foreground">
          策略可随时修改，保存后立即用于新触发的自动化；已有运行仍保留当时的配置快照。
        </p>
      </div>
      <form
        action={(form) => call({
          action: "save",
          mode: form.get("mode")
        })}
        className="flex flex-wrap items-center gap-2"
      >
        <select className="h-8 min-w-56 flex-1 rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none transition-colors focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50" defaultValue={currentMode} name="mode">
          <option value="manual">仅人工处理</option>
          <option value="agent-on-demand">按需生成 AI 草案</option>
          <option value="agent-auto-draft">自动生成 AI 草案</option>
          <option value="follow-up-draft">生成后续处置草案</option>
        </select>
        <Button type="submit">保存模式</Button>
      </form>
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
    </section>
  );
}
