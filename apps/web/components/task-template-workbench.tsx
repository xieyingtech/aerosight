"use client";

import { InspectionReadiness } from "@/components/inspection-readiness";
import { TaskSourceValidation } from "@/components/task-source-validation";
import { TaskSourceEditor } from "@/components/task-source-editor";
import { InspectionFlightPlanPicker } from "@/components/inspection-flight-plan-picker";
import { editTaskField, readTaskSource, type TaskSource } from "@/lib/task-source-editor";
import { useRef, useState } from "react";
import { apiFetch } from "@/lib/api-client";
import { useRouter } from "next/navigation";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export function TaskTemplateWorkbench({ projectId,taskId,model,onChanged }: { projectId: number; taskId: number; onChanged:()=>void; model: {
  versions: Array<Record<string,unknown>>; selectedVersion: Record<string,unknown> | null; steps: Array<Record<string,unknown>>;
  definition: Record<string,unknown>; canEdit: boolean; task?:Record<string,unknown>;
} }) {
  const router = useRouter();
  const [definition,setDefinition] = useState<TaskSource>({format:model.selectedVersion?.sourceFormat === "yaml" ? "yaml":"json",source:typeof model.selectedVersion?.source === "string" ? model.selectedVersion.source : JSON.stringify(model.definition,null,2)});
  const [runInputs,setRunInputs] = useState("{}");
  const manualAttempt=useRef<{signature:string;idempotencyKey:string;occurredAt:string}|null>(null);
  const [pending,setPending] = useState(false);
  const [message,setMessage] = useState("");
  const selected = model.selectedVersion;
 const savedSource=typeof selected?.source === "string" ? selected.source : JSON.stringify(model.definition,null,2);
 const dirty=definition.source!==savedSource;
 let flightStepIndex=-1;
 try { const steps=readTaskSource(definition).steps as Array<{uses?:string;with?:{mode?:string}}>;
 flightStepIndex=steps?.findIndex(step=>step.uses==="inspection.observe"&&step.with?.mode==="flighthub-flight")??-1;
 } catch { /* The source editor displays parse errors. */ }

  async function act(action: "create"|"save"|"publish") {
    setPending(true); setMessage("");
    try {
      const payload: Record<string,unknown> = { action,versionId: selected?.id };
      if (action === "save") {payload.source=definition.source;payload.sourceFormat=definition.format;}
      if (selected?.revision !== undefined) payload.expectedRevision=selected.revision;
      const response = await apiFetch(`/api/projects/${projectId}/tasks/${taskId}/versions`, {
        method: "POST",headers: { "content-type": "application/json" },body: JSON.stringify(payload)
      });
      const result = await response.json();
      if (!response.ok) throw new Error(String(result.error || "TASK_VERSION_UPDATE_FAILED"));
      setMessage(action === "publish" ? "版本已发布" : action === "save" ? "草稿已保存" : "草稿已创建");
      onChanged();
    } catch (error) { setMessage(error instanceof Error ? error.message : "操作失败"); }
    finally { setPending(false); }
  }
  async function updateState(){
    setPending(true);setMessage("");
    try{const response=await apiFetch(`/api/projects/${projectId}/tasks/${taskId}`,{method:"PATCH",headers:{"Content-Type":"application/json"},body:JSON.stringify({status:model.task?.status==="active"?"disabled":"active"})});const result=await response.json();if(!response.ok)throw new Error(String(result.error??"更新失败"));onChanged();}
    catch(e){setMessage(e instanceof Error?e.message:"更新失败");}finally{setPending(false);}
  }
  async function triggerManual() {
    setPending(true); setMessage("");
    try {
      const inputs = JSON.parse(runInputs);
      const signature=JSON.stringify({versionId:selected?.id,inputs});
      if(manualAttempt.current?.signature!==signature)manualAttempt.current={signature,idempotencyKey:crypto.randomUUID(),occurredAt:new Date().toISOString()};
      const attempt=manualAttempt.current;
      const response = await apiFetch(`/api/projects/${projectId}/tasks/${taskId}/runs`, { method: "POST",
        headers: { "content-type": "application/json" }, body: JSON.stringify({ type: "manual",idempotencyKey: attempt.idempotencyKey,
          occurredAt: attempt.occurredAt,inputs }) });
      const result = await response.json();
      if (!response.ok) throw new Error(String(result.error || "TASK_TRIGGER_FAILED"));
      manualAttempt.current=null;
      router.push(`/projects/tasks/runs/detail/?projectId=${projectId}&runId=${String(result.taskRunId)}`);
    } catch (error) { setMessage(error instanceof Error ? error.message : "触发失败"); }
    finally { setPending(false); }
  }
  return <div className="space-y-4">
    <Card><CardHeader><CardTitle>版本与触发器</CardTitle><CardDescription>已发布版本不可变；编辑会落到独立草稿。</CardDescription></CardHeader><CardContent className="flex flex-wrap items-center gap-2">
      {model.versions.map((version) => <Badge key={String(version.id)} variant={version.status === "published" ? "default" : "outline"}>v{String(version.version)} · {String(version.status)}</Badge>)}
      {model.canEdit && selected?.status !== "draft" ? <Button disabled={pending} onClick={() => act("create")}>创建草稿</Button> : null}
      {model.canEdit && selected?.status === "draft" ? <><Button disabled={pending} onClick={() => act("save")}>保存草稿</Button><Button disabled={pending||dirty} variant="outline" onClick={() => act("publish")}>发布版本</Button></> : null}
      {selected?.status === "published" && ["manual", "schedule"].includes((model.definition.trigger as { type?: string } | undefined)?.type ?? "") && model.canEdit
        ? <Button disabled={pending} onClick={triggerManual}>手动运行</Button> : null}
      {model.canEdit&&model.task&&<Button disabled={pending} variant="outline" onClick={updateState}>{model.task.status==="active"?"停用任务":"启用任务"}</Button>}
      {model.task&&<p className="w-full text-sm text-muted-foreground">当前执行委托者：{model.task.authorizedByUserId?`${String(model.task.authorizedByName??"用户")}（#${String(model.task.authorizedByUserId)}）`:"尚未绑定"}。启用任务会绑定当前账号，定时运行前重新检查其权限。</p>}
      {dirty&&<span className="text-sm text-muted-foreground">有未保存的修改，保存后可发布。</span>}
      {message ? <span className="text-sm text-muted-foreground">{message}</span> : null}
    </CardContent></Card>
    {selected?.status === "published" && ["manual", "schedule"].includes((model.definition.trigger as { type?: string } | undefined)?.type ?? "") && model.canEdit
      ? <Card><CardHeader><CardTitle>手动运行参数</CardTitle><CardDescription>按当前发布版本的 inputSchema 填写 JSON；项目范围由服务端会话绑定。</CardDescription></CardHeader><CardContent><textarea aria-label="任务运行输入 JSON" className="min-h-28 w-full rounded-md border bg-background p-3 font-mono text-xs" value={runInputs} onChange={(event) => setRunInputs(event.target.value)} /></CardContent></Card> : null}
    <Card><CardHeader><CardTitle>类型化任务配置</CardTitle><CardDescription>包含输入 schema、触发器、并发限制、资源能力、条件、依赖、超时、重试与失败策略。</CardDescription></CardHeader><CardContent>
      <InspectionReadiness projectId={projectId}/>
      {model.canEdit && selected?.status==="draft" && flightStepIndex>=0 && <fieldset disabled={pending}><InspectionFlightPlanPicker projectId={projectId} onApply={plan=>{
        try { let next=definition; for(const [field,value] of Object.entries({connectorId:plan.connectorId,deviceId:plan.deviceId,waylineResourceId:plan.waylineResourceId,waylineVersion:plan.waylineVersion})) next=editTaskField(next,["steps",flightStepIndex,"with",field],value);
          setDefinition(next);setMessage("航线目录版本已写入编辑器，请保存草稿。");
        } catch(error) { setMessage(error instanceof Error?error.message:"草稿无法解析"); }
      }}/></fieldset>}
      <TaskSourceValidation projectId={projectId} value={definition} disabled={pending || !model.canEdit}/>
      <TaskSourceEditor value={definition} onChange={setDefinition} readOnly={pending || !model.canEdit || selected?.status !== "draft"}/>
    </CardContent></Card>
    <Card><CardHeader><CardTitle>步骤配置</CardTitle></CardHeader><CardContent className="space-y-2">{model.steps.map((step) => <div className="rounded-lg border p-3" key={String(step.id)}>
      <div className="flex flex-wrap items-center gap-2"><Badge variant="outline">#{String(step.position)}</Badge><strong>{String(step.name)}</strong><Badge>{String(step.uses)}</Badge></div>
      <p className="mt-2 text-xs text-muted-foreground">key={String(step.key)} · requires={String(step.capabilityCode)} · dependsOn={JSON.stringify(step.dependsOn)} · timeout={String(step.timeoutSeconds)}s · retry={JSON.stringify(step.retry)} · onFailure={String(step.onFailure)}</p>
      <pre className="mt-2 overflow-auto rounded bg-muted p-2 text-xs">条件 {JSON.stringify(step.condition ?? null)}{"\n"}输入 {JSON.stringify(step.inputSchema)}{"\n"}输出 {JSON.stringify(step.outputSchema)}</pre>
    </div>)}</CardContent></Card>
  </div>;
}
