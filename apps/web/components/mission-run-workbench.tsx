import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { MissionControlButtons } from "@/components/mission-control-buttons";
import type { MissionAction } from "@/lib/mission-workbench-core";

export function MissionRunWorkbench({ projectId, model }: { projectId: number; model: {
  run: Record<string, unknown>; steps: Array<Record<string, unknown>>; actions: MissionAction[];
} }) {
  const checks = Array.isArray((model.run.preflight as { checks?: unknown[] } | undefined)?.checks)
    ? (model.run.preflight as { checks: Array<Record<string, unknown>> }).checks : [];
  return <div className="space-y-4">
    <div className="grid gap-4 md:grid-cols-3">
      <Card><CardHeader><CardDescription>运行状态</CardDescription><CardTitle>{String(model.run.taskName)}</CardTitle></CardHeader><CardContent className="space-y-2"><Badge>{String(model.run.status)}</Badge><p className="text-xs text-muted-foreground">版本 {String(model.run.taskVersion ?? "-")} · 状态版本 {String(model.run.stateVersion)}</p></CardContent></Card>
      <Card><CardHeader><CardDescription>执行设备</CardDescription><CardTitle>{String(model.run.deviceName ?? "尚未分配")}</CardTitle></CardHeader><CardContent><p className="text-sm">{String(model.run.deviceStatus ?? "unknown")}</p></CardContent></Card>
      <Card><CardHeader><CardDescription>安全闸门</CardDescription><CardTitle>策略 v{String(model.run.safetyPolicyVersion ?? "-")}</CardTitle></CardHeader><CardContent><p className="text-sm">审批：{String(model.run.approvalStatus ?? "未要求")}</p></CardContent></Card>
    </div>
    <Card><CardHeader><CardTitle>预检</CardTitle><CardDescription>硬失败不可绕过，警告保留在运行快照中</CardDescription></CardHeader><CardContent className="space-y-2">{checks.length ? checks.map((item, index) => <div className="flex items-center justify-between rounded-lg border px-3 py-2" key={String(item.code ?? index)}><span>{String(item.message ?? item.code)}</span><Badge variant={item.severity === "hard_failure" ? "destructive" : "outline"}>{String(item.severity)}</Badge></div>) : <p className="text-sm text-muted-foreground">尚无预检快照</p>}</CardContent></Card>
    <Card><CardHeader><CardTitle>步骤与命令确认</CardTitle></CardHeader><CardContent className="space-y-2">{model.steps.length ? model.steps.map((step) => <div className="grid gap-2 rounded-lg border p-3 md:grid-cols-[50px_1fr_130px_180px]" key={String(step.position)}><span className="text-muted-foreground">#{String(step.position)}</span><div><p className="font-medium">{String(step.name)}</p><p className="text-xs text-muted-foreground">{String(step.action)}</p></div><Badge variant="outline">{String(step.status)}</Badge><p className="text-xs text-muted-foreground">命令：{String(step.commandStatus ?? "尚未创建")}</p></div>) : <p className="text-sm text-muted-foreground">尚未生成运行步骤</p>}</CardContent></Card>
    {model.actions.length ? <MissionControlButtons actions={model.actions} projectId={projectId} stateVersion={Number(model.run.stateVersion)} taskRunId={Number(model.run.id)} /> : <p className="text-sm text-muted-foreground">当前账号仅可查看，不能控制或审批任务。</p>}
  </div>;
}
