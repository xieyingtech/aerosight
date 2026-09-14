"use client";

import Link from "next/link";
import { useRef, useState } from "react";
import { useAPI } from "@/lib/use-api";
import { apiFetch } from "@/lib/api-client";
import { APIStateView } from "@/components/api-state";
import { Button } from "@/components/ui/button";

type HeldAlert = {resourceId:number;ownership:"pending"|"task";taskRunId:number|null;status:string;summary:{label?:string;capturedAt?:string;taskRunId?:number}};
type Policy = {taskManagedAlerts:boolean;heldAlerts:HeldAlert[];truncated:boolean;canConfigure:boolean};

export function InspectionAlertPolicy({projectId,connectorId}:{projectId:number;connectorId:string}){
 const path=`/api/projects/${projectId}/inspection/connectors/${connectorId}/alert-policy`;
 const state=useAPI<Policy>(path);
 const [pending,setPending]=useState(false),[error,setError]=useState("");
 const keys=useRef(new Map<string,string>());
 async function change(body:Record<string,unknown>){
  setPending(true);setError("");const actionKey=JSON.stringify(body);let key=keys.current.get(actionKey);if(!key){key=crypto.randomUUID();keys.current.set(actionKey,key);}
  try{const response=await apiFetch(path,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({...body,idempotencyKey:key})});const result=await response.json();if(!response.ok)throw new Error(String(result.error??"更新失败"));keys.current.delete(actionKey);state.reload();}
  catch(e){setError(e instanceof Error?e.message:"更新失败");}finally{setPending(false);}
 }
 return <section className="space-y-3 rounded-lg border p-3" aria-label="巡检告警处理策略">
  <h3 className="text-sm font-medium">巡检告警处理策略</h3>
  <p className="text-xs text-muted-foreground">开启后，未确认归属的新告警先保存为证据，由 Task 研判后处理案件。关闭开关也会保留已接管及待确认架次的保护。</p>
  <APIStateView state={state}>{policy=><>
   <div className="flex flex-wrap items-center gap-3"><span className="text-sm">{policy.taskManagedAlerts?"Task 管理已开启":"新架次默认沿用旧建案规则"}</span>
    {policy.canConfigure&&<Button size="sm" variant="outline" disabled={pending} onClick={()=>change({action:"set-policy",taskManagedAlerts:!policy.taskManagedAlerts})}>{policy.taskManagedAlerts?"关闭新架次接管":"开启 Task 告警管理"}</Button>}
   </div>
   {policy.heldAlerts.length>0&&<p className="text-xs text-muted-foreground">确认沿用旧规则会释放该架次的所有告警，后续同步可创建或修改案件。已有案件不会自动归入 Task。</p>}
   <ul className="space-y-2">{policy.heldAlerts.map(alert=><li key={alert.resourceId} className="space-y-1 rounded border p-2 text-xs">
    <div>证据 #{alert.resourceId} · {alert.summary.label??"司空告警"} · {alert.ownership==="task"?"Task 已接管":"归属待确认"}{alert.status==="missing"?" · 上游已缺失":""}</div>
    {alert.summary.capturedAt&&<div className="text-muted-foreground">观测时间 {alert.summary.capturedAt}</div>}
    {(alert.taskRunId??alert.summary.taskRunId)&&<Link className="underline" href={`/projects/tasks/runs/detail/?projectId=${projectId}&runId=${alert.taskRunId??alert.summary.taskRunId}`}>查看关联运行</Link>}
    {policy.canConfigure&&alert.ownership==="pending"&&<Button size="sm" variant="outline" disabled={pending} onClick={()=>change({action:"confirm-legacy",resourceId:alert.resourceId})}>确认该架次沿用旧建案规则</Button>}
   </li>)}</ul>
   {policy.heldAlerts.length===0&&<p className="text-xs text-muted-foreground">暂无待确认或已接管的告警证据。</p>}
   {policy.truncated&&<p className="text-xs text-muted-foreground">当前仅显示前 100 条证据，仍有其他记录；本列表不代表完整观测范围。</p>}
  </>}</APIStateView>
  {error&&<p role="alert" className="text-sm text-destructive">{error}</p>}
 </section>;
}
