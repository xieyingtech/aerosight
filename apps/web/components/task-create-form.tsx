"use client";
import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { apiFetch } from "@/lib/api-client";
import { inspectionTaskTemplate } from "@/lib/inspection-task-templates";
import { InspectionReadiness } from "@/components/inspection-readiness";
import { TaskSourceValidation } from "@/components/task-source-validation";
import { TaskSourceEditor } from "@/components/task-source-editor";
import { InspectionFlightPlanPicker } from "@/components/inspection-flight-plan-picker";
import { editTaskField, readTaskSource } from "@/lib/task-source-editor";
import { Button } from "@/components/ui/button";

export function TaskCreateForm({projectId}:{projectId:number}){
 const router=useRouter();
 const [template,setTemplate]=useState<"assets"|"existing-flight"|"flighthub-flight">("assets");
 const [value,setValue]=useState(()=>inspectionTaskTemplate("assets"));
 const [pending,setPending]=useState(false),[message,setMessage]=useState("");
 const key=useRef<string|null>(null);
 async function submit(){
  setPending(true);setMessage("");key.current??=crypto.randomUUID();
  try{
   const response=await apiFetch(`/api/projects/${projectId}/tasks`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({sourceFormat:value.format,source:value.source,idempotencyKey:key.current})});
   const result=await response.json();if(!response.ok)throw new Error(String(result.error??"创建失败"));
   router.push(`/projects/tasks/detail/?projectId=${projectId}&taskId=${String(result.taskId)}`);
  }catch(e){setMessage(e instanceof Error?e.message:"创建失败");}finally{setPending(false);}
 }
 return <section className="space-y-3 rounded-lg border p-4">
  <h2 className="font-medium">新建巡检任务</h2>
  <p className="text-sm text-muted-foreground">选择当前项目的已有飞行或航拍图片，创建停用状态的草稿；发布前校验所需资源和执行能力。</p>
  <select aria-label="巡检模板" className="rounded border bg-background p-2" disabled={pending} onChange={e=>{key.current=null;const mode=e.target.value as typeof template;setTemplate(mode);setValue(inspectionTaskTemplate(mode));}}>
   <option value="assets">已有航拍图片</option><option value="existing-flight">司空已完成飞行</option><option value="flighthub-flight">司空新飞行（仅草稿）</option>
  </select>
  <InspectionReadiness projectId={projectId}/>
  {template==="flighthub-flight"&&<fieldset disabled={pending}><InspectionFlightPlanPicker projectId={projectId} onApply={plan=>{try{const steps=readTaskSource(value).steps as Array<{uses?:string;with?:{mode?:string}}>;const index=steps?.findIndex(step=>step.uses==="inspection.observe"&&step.with?.mode==="flighthub-flight")??-1;if(index<0)throw new Error("当前草稿没有司空新飞行观察步骤");let next=value;for(const [field,selected] of Object.entries({connectorId:plan.connectorId,deviceId:plan.deviceId,waylineResourceId:plan.waylineResourceId,waylineVersion:plan.waylineVersion}))next=editTaskField(next,["steps",index,"with",field],selected);setValue(next);key.current=null;setMessage("航线目录版本已写入草稿，保存后仍需发布校验。");}catch(e){setMessage(e instanceof Error?e.message:"草稿无法解析");}}}/></fieldset>}
  <TaskSourceValidation projectId={projectId} value={value} disabled={pending}/>
      <TaskSourceEditor value={value} onChange={next=>{key.current=null;setValue(next);}} readOnly={pending}/>
  <Button disabled={pending} onClick={submit}>创建任务草稿</Button>
  {message&&<p role="alert" className="text-sm text-destructive">{message}</p>}
 </section>;
}
