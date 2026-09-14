"use client";
import {useState} from "react";
import {useAPI} from "@/lib/use-api";
import {apiJSON} from "@/lib/api-client";
import {APIStateView} from "@/components/api-state";
import {Button} from "@/components/ui/button";
export type FlightPlan={connectorId:number;deviceId:number;waylineResourceId:number;name:string;waylineVersion:{waylineId:string;updatedAt:number;sizeBytes:number;remoteVersion:string}};
type Options={waylines:Array<{id:string;connectorId:string;name:string}>;devices:Array<{id:number;connectorId:string;name:string}>;limit:number};
export function InspectionFlightPlanPicker({projectId,onApply}:{projectId:number;onApply:(plan:FlightPlan)=>void}){
 const state=useAPI<Options>(`/api/projects/${projectId}/inspection/flight-plan-options`);
 const [route,setRoute]=useState("");const [device,setDevice]=useState("");const [plan,setPlan]=useState<FlightPlan|null>(null);const [pending,setPending]=useState(false);const [error,setError]=useState("");
 return <section aria-label="司空航线预览" className="space-y-3 rounded border p-3">
 <h3 className="text-sm font-medium">选择司空预设航线与设备</h3>
 <p className="text-xs text-muted-foreground">航线在司空编辑，预览只读目录。当前新飞行步骤尚未开放执行，可保存草稿；版本与设备兼容性还需在执行前重新核验。</p>
 <APIStateView state={state}>{options=>{const selected=options.waylines.find(item=>item.id===route);return <div className="space-y-3">
 <select aria-label="司空预设航线" className="w-full rounded border bg-background p-2" disabled={pending} value={route} onChange={e=>{setRoute(e.target.value);setDevice("");setPlan(null);setError("");}}><option value="">选择航线</option>{options.waylines.map(item=><option key={item.id} value={item.id}>{item.name} · 连接器 #{item.connectorId}</option>)}</select>
 <select aria-label="司空执行设备" className="w-full rounded border bg-background p-2" disabled={pending||!selected} value={device} onChange={e=>{setDevice(e.target.value);setPlan(null);setError("");}}><option value="">选择同连接器设备</option>{options.devices.filter(item=>item.connectorId===selected?.connectorId).map(item=><option key={item.id} value={item.id}>{item.name}</option>)}</select>
 <Button type="button" variant="outline" disabled={pending||!selected||!device} onClick={async()=>{setPending(true);setPlan(null);setError("");try{setPlan(await apiJSON<FlightPlan>(`/api/projects/${projectId}/inspection/flight-plan?connectorId=${selected!.connectorId}&waylineResourceId=${route}&deviceId=${device}`));}catch(e){setError(e instanceof Error?e.message:"预览失败");}finally{setPending(false);}}}>预览航线版本</Button>
 <p className="text-xs text-muted-foreground">各类目录最多显示 {options.limit} 条；列表为空时请先在连接器中同步资源。</p>
 {plan&&<div className="space-y-2 rounded bg-muted p-3 text-sm"><p>{plan.name} · 司空来源 · 目录版本 {plan.waylineVersion.remoteVersion}</p><p>航线 ID：{plan.waylineVersion.waylineId}</p><Button type="button" onClick={()=>onApply(plan)}>将航线版本写入草稿</Button></div>}
 </div>;}}</APIStateView>
 {error&&<p role="alert" className="text-sm text-destructive">{error}</p>}
 </section>;
}
