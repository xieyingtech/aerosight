import Link from "next/link";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { MissionControlButtons } from "@/components/mission-control-buttons";
import type { MissionAction } from "@/lib/mission-workbench-core";

export function MissionRunWorkbench({ projectId, model, onChanged }: { onChanged: () => void; projectId: number; model: {
  run: Record<string, unknown>; steps: Array<Record<string, unknown>>; audit: Array<Record<string,unknown>>; actions: MissionAction[];
} }) {
  const checks = Array.isArray((model.run.preflight as { checks?: unknown[] } | undefined)?.checks)
    ? (model.run.preflight as { checks: Array<Record<string, unknown>> }).checks : [];
  return <div className="space-y-4">
 {model.steps.some(step=>String(step.uses).startsWith("inspection."))&&<Link className="text-sm underline" href={`/projects/inspection/summary/?projectId=${projectId}&runId=${model.run.id}`}>查看巡检进展与待复核摘要</Link>}
    <div className="grid gap-4 md:grid-cols-3">
      <Card><CardHeader><CardDescription>运行状态</CardDescription><CardTitle>{String(model.run.taskName)}</CardTitle></CardHeader><CardContent className="space-y-2"><Badge>{String(model.run.status)}</Badge><p className="text-xs text-muted-foreground">版本 {String(model.run.taskVersion ?? "-")} · 状态版本 {String(model.run.stateVersion)}</p><p className="text-xs text-muted-foreground">原因：{String(model.run.stateReason ?? "—")}</p></CardContent></Card>
      <Card><CardHeader><CardDescription>执行设备</CardDescription><CardTitle>{String(model.run.deviceName ?? "尚未分配")}</CardTitle></CardHeader><CardContent><p className="text-sm">{String(model.run.deviceStatus ?? "unknown")}</p></CardContent></Card>
      <Card><CardHeader><CardDescription>安全闸门</CardDescription><CardTitle>策略 v{String(model.run.safetyPolicyVersion ?? "-")}</CardTitle></CardHeader><CardContent><p className="text-sm">审批：{String(model.run.approvalStatus ?? "未要求")}</p></CardContent></Card>
    </div>
    <Card><CardHeader><CardTitle>触发与输入快照</CardTitle><CardDescription>运行固定记录实际触发主体、幂等标识和类型化参数。</CardDescription></CardHeader><CardContent className="space-y-2"><p className="text-sm">{String(model.run.triggerSource)} · {String(model.run.triggerKey ?? "无触发键")}</p><pre className="max-h-56 overflow-auto rounded bg-muted p-3 text-xs">{JSON.stringify(model.run.inputSnapshot,null,2)}</pre></CardContent></Card>
    <Card><CardHeader><CardTitle>预检</CardTitle><CardDescription>硬失败不可绕过，警告保留在运行快照中</CardDescription></CardHeader><CardContent className="space-y-2">{checks.length ? checks.map((item, index) => <div className="flex items-center justify-between rounded-lg border px-3 py-2" key={String(item.code ?? index)}><span>{String(item.message ?? item.code)}</span><Badge variant={item.severity === "hard_failure" ? "destructive" : "outline"}>{String(item.severity)}</Badge></div>) : <p className="text-sm text-muted-foreground">尚无预检快照</p>}</CardContent></Card>
    <Card><CardHeader><CardTitle>步骤、条件与输出</CardTitle><CardDescription>输入输出、条件判断、跳过/失败原因和执行幂等键均来自运行快照。</CardDescription></CardHeader><CardContent className="space-y-2">{model.steps.length ? model.steps.map((step) => <details id={`step-${step.id}`} className="rounded-lg border p-3" key={String(step.id)} open={step.status === "failed" || step.status === "paused"}><summary className="cursor-pointer"><span className="mr-2 text-muted-foreground">#{String(step.position)}</span><strong>{String(step.name)}</strong><Badge className="ml-2" variant="outline">{String(step.uses)}</Badge><Badge className="ml-2" variant={step.status === "failed" ? "destructive" : "outline"}>{String(step.status)}</Badge></summary><InspectionStepLinks projectId={projectId} output={step.outputSnapshot}/><div className="mt-3 grid gap-3 md:grid-cols-2"><div className="space-y-1 text-xs text-muted-foreground"><p>key={String(step.key)} · action={String(step.action)}</p><p>dependsOn={JSON.stringify(step.dependsOn)} · attempts={String(step.attemptCount)}</p><p>onFailure={String(step.onFailure)} · executionKey={String(step.executionKey ?? "—")}</p><p>命令={String(step.commandStatus ?? "未创建")}</p></div><pre className="max-h-52 overflow-auto rounded bg-muted p-2 text-xs">条件 {JSON.stringify(step.condition)}{"\n"}判断 {JSON.stringify(step.conditionResult)}{"\n"}输入 {JSON.stringify(step.inputSnapshot)}{"\n"}输出 {JSON.stringify(step.outputSnapshot)}{"\n"}结果 {JSON.stringify(step.result)}</pre></div></details>) : <p className="text-sm text-muted-foreground">尚未生成运行步骤</p>}</CardContent></Card>
    <Card><CardHeader><CardTitle>审计链</CardTitle><CardDescription>任务、版本和运行相关的授权与写操作按时间排列。</CardDescription></CardHeader><CardContent className="space-y-2">{model.audit.length ? model.audit.map((item) => <div className="flex flex-wrap items-center gap-2 rounded border p-2 text-sm" key={String(item.id)}><Badge variant="outline">{String(item.status)}</Badge><span className="font-medium">{String(item.action)}</span><span className="text-xs text-muted-foreground">actor={item.actorAgentId ? `agent:${String(item.actorAgentId)}` : `user:${String(item.actorUserId)}`} · request={String(item.requestId)}</span></div>) : <p className="text-sm text-muted-foreground">暂无关联审计记录</p>}</CardContent></Card>
    {model.actions.length ? <MissionControlButtons onChanged={onChanged} actions={model.actions} projectId={projectId} stateVersion={Number(model.run.stateVersion)} taskRunId={Number(model.run.id)} /> : <p className="text-sm text-muted-foreground">当前账号仅可查看，不能控制或审批任务。</p>}
  </div>;
}

function InspectionStepLinks({projectId,output}:{projectId:number;output:unknown}){
 if(!output||typeof output!=="object")return null;
 const values=output as Record<string,unknown>;
 const id=values.assessmentId;
 return <div className="mt-3 flex flex-wrap gap-3 text-sm">
  {typeof id==="string"&&/^[0-9a-f-]{36}$/i.test(id)&&<Link className="underline" href={`/projects/inspection/assessment/?projectId=${projectId}&assessmentId=${id}`}>查看研判与人工复核</Link>}
  {typeof values.observationId==="string"&&<a className="underline" href={`/projects/inspection/observation/?projectId=${projectId}&observationId=${encodeURIComponent(values.observationId)}`} target="_blank" rel="noreferrer">观察范围与原图引用</a>}
  {typeof values.evidenceSetId==="string"&&<a className="underline" href={`/projects/inspection/evidence/?projectId=${projectId}&evidenceSetId=${encodeURIComponent(values.evidenceSetId)}`} target="_blank" rel="noreferrer">识别证据</a>}
  {typeof values.reportId==="string"&&/^[0-9a-f-]{36}$/i.test(values.reportId)&&<Link className="underline" href={`/projects/reports/detail/?projectId=${projectId}&reportId=${values.reportId}`}>查看巡检报告</Link>}
  {Array.isArray(values.issueIds)&&values.issueIds.filter((v):v is number=>typeof v==="number"&&Number.isSafeInteger(v)&&v>0).map(issueId=><Link key={issueId} className="underline" href={`/projects/issues/detail/?projectId=${projectId}&issueId=${issueId}`}>案件 #{issueId}</Link>)}
 </div>;
}
