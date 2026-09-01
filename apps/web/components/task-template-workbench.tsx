"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export function TaskTemplateWorkbench({ projectId,taskId,model }: { projectId: number; taskId: number; model: {
  versions: Array<Record<string,unknown>>; selectedVersion: Record<string,unknown> | null; steps: Array<Record<string,unknown>>;
  definition: Record<string,unknown>; canEdit: boolean;
} }) {
  const router = useRouter();
  const [definition,setDefinition] = useState(JSON.stringify(model.definition,null,2));
  const [runInputs,setRunInputs] = useState("{}");
  const [pending,setPending] = useState(false);
  const [message,setMessage] = useState("");
  const selected = model.selectedVersion;
  async function act(action: "create"|"save"|"publish") {
    setPending(true); setMessage("");
    try {
      const payload: Record<string,unknown> = { action,versionId: selected?.id };
      if (action === "save") payload.definition = JSON.parse(definition);
      const response = await fetch(`/api/projects/${projectId}/tasks/${taskId}/versions`, {
        method: "POST",headers: { "content-type": "application/json" },body: JSON.stringify(payload)
      });
      const result = await response.json();
      if (!response.ok) throw new Error(String(result.error || "TASK_VERSION_UPDATE_FAILED"));
      setMessage(action === "publish" ? "版本已发布" : action === "save" ? "草稿已保存" : "草稿已创建");
      router.refresh();
    } catch (error) { setMessage(error instanceof Error ? error.message : "操作失败"); }
    finally { setPending(false); }
  }
  async function triggerManual() {
    setPending(true); setMessage("");
    try {
      const inputs = JSON.parse(runInputs);
      const response = await fetch(`/api/projects/${projectId}/tasks/${taskId}/runs`, { method: "POST",
        headers: { "content-type": "application/json" }, body: JSON.stringify({ type: "manual",idempotencyKey: crypto.randomUUID(),
          occurredAt: new Date().toISOString(),inputs }) });
      const result = await response.json();
      if (!response.ok) throw new Error(String(result.error || "TASK_TRIGGER_FAILED"));
      router.push(`/projects/${projectId}/tasks/runs/${String(result.taskRunId)}`);
    } catch (error) { setMessage(error instanceof Error ? error.message : "触发失败"); }
    finally { setPending(false); }
  }
  return <div className="space-y-4">
    <Card><CardHeader><CardTitle>版本与触发器</CardTitle><CardDescription>已发布版本不可变；编辑会落到独立草稿。</CardDescription></CardHeader><CardContent className="flex flex-wrap items-center gap-2">
      {model.versions.map((version) => <Badge key={String(version.id)} variant={version.status === "published" ? "default" : "outline"}>v{String(version.version)} · {String(version.status)}</Badge>)}
      {model.canEdit && selected?.status !== "draft" ? <Button disabled={pending} onClick={() => act("create")}>创建草稿</Button> : null}
      {model.canEdit && selected?.status === "draft" ? <><Button disabled={pending} onClick={() => act("save")}>保存草稿</Button><Button disabled={pending} variant="outline" onClick={() => act("publish")}>发布版本</Button></> : null}
      {selected?.status === "published" && (model.definition.trigger as { type?: string } | undefined)?.type === "manual" && model.canEdit
        ? <Button disabled={pending} onClick={triggerManual}>手动运行</Button> : null}
      {message ? <span className="text-sm text-muted-foreground">{message}</span> : null}
    </CardContent></Card>
    {selected?.status === "published" && (model.definition.trigger as { type?: string } | undefined)?.type === "manual" && model.canEdit
      ? <Card><CardHeader><CardTitle>手动运行参数</CardTitle><CardDescription>按当前发布版本的 inputSchema 填写 JSON；项目范围由服务端会话绑定。</CardDescription></CardHeader><CardContent><textarea aria-label="任务运行输入 JSON" className="min-h-28 w-full rounded-md border bg-background p-3 font-mono text-xs" value={runInputs} onChange={(event) => setRunInputs(event.target.value)} /></CardContent></Card> : null}
    <Card><CardHeader><CardTitle>类型化任务配置</CardTitle><CardDescription>包含输入 schema、触发器、并发限制、资源能力、条件、依赖、超时、重试与失败策略。</CardDescription></CardHeader><CardContent>
      <textarea aria-label="任务定义 JSON" className="min-h-[420px] w-full rounded-md border bg-background p-3 font-mono text-xs" readOnly={!model.canEdit || selected?.status !== "draft"} value={definition} onChange={(event) => setDefinition(event.target.value)} />
    </CardContent></Card>
    <Card><CardHeader><CardTitle>步骤配置</CardTitle></CardHeader><CardContent className="space-y-2">{model.steps.map((step) => <div className="rounded-lg border p-3" key={String(step.id)}>
      <div className="flex flex-wrap items-center gap-2"><Badge variant="outline">#{String(step.position)}</Badge><strong>{String(step.name)}</strong><Badge>{String(step.uses)}</Badge></div>
      <p className="mt-2 text-xs text-muted-foreground">key={String(step.key)} · requires={String(step.capabilityCode)} · dependsOn={JSON.stringify(step.dependsOn)} · timeout={String(step.timeoutSeconds)}s · retry={JSON.stringify(step.retry)} · onFailure={String(step.onFailure)}</p>
      <pre className="mt-2 overflow-auto rounded bg-muted p-2 text-xs">条件 {JSON.stringify(step.condition ?? null)}{"\n"}输入 {JSON.stringify(step.inputSchema)}{"\n"}输出 {JSON.stringify(step.outputSchema)}</pre>
    </div>)}</CardContent></Card>
  </div>;
}
