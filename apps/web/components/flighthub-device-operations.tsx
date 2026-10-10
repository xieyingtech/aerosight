"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { FlightHubFlightLaunch } from "@/components/flighthub-flight-launch";
import { projectPageHref } from "@/lib/page-routes";
import { apiFetch } from "@/lib/api-client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";

type Field = { key: string; label: string; type: string; required: boolean; hint?: string; options?: {value:string;label:string}[] };
type Operation = {key:string;label:string;group:string;fields:Field[];canRequest:boolean;available:boolean;blockers:string[]};
type Execution = {id:string;key:string;label:string;requestedBy:string;summary:Record<string,unknown>;receipt?:{id:string;status:string;error?:string|null}|null};
type ControlSession = {id:string;status:string;owned:boolean};
type Model = {operations:Operation[];executions:Execution[];controlSessions:ControlSession[]};
const statusLabels:Record<string,string>={pending:"待审批",approved:"已批准",rejected:"已拒绝",expired:"已过期",requested:"已请求",acquiring:"正在获取",active:"控制权已获取",releasing:"正在释放",succeeded:"执行完成",sent:"已下发，等待结果",queued:"已排队",failed:"执行失败",blocked:"执行被阻止",unknown:"结果待核实",dispatchable:"等待下发",acknowledged:"设备已确认",nacked:"设备拒绝",timed_out:"执行超时",canceled:"已取消",released:"控制权已释放"};

function errorMessage(body:Record<string,unknown>) {
 const value=body.error;
 const code=typeof value==="object"&&value?String((value as Record<string,unknown>).code):String(value??"操作失败");
 if(code==="DEVICE_OPERATION_STATE_CHANGED") return "设备或任务状态已变化，此操作当前不可执行，请刷新状态。";
 const labels:Record<string,string>={DEVICE_OPERATION_APPROVAL_DENIED:"无法审批：请检查权限、审批有效期及是否为申请人。",DEVICE_OPERATION_APPROVAL_REQUIRED:"请先取得有效的人工审批。",DEVICE_OPERATION_PARAMETERS_INVALID:"参数不符合司空接口要求，请检查后重试。",FLIGHTHUB_CONTROL_HEARTBEAT_REJECTED:"控制权已到期或尚未取得，请刷新状态。",FLIGHTHUB_COMMAND_NOT_FIELD_VERIFIED:"此型号与固件尚未完成现场操作验证。",FLIGHTHUB_COMMAND_STATE_STALE:"设备状态已过期，请刷新状态后重试。",FLIGHTHUB_COMMAND_FEATURE_DISABLED:"项目尚未启用此操作。",DEVICE_CAPABILITY_NOT_GRANTED:"当前账号没有此设备操作权限。",DEVICE_CAPABILITY_EXPLICITLY_DENIED:"当前账号的此设备能力被禁止。"};
 return labels[code]??code;
}

export function FlightHubDeviceOperations({projectId,deviceId,deviceName,onChanged,workflowActions=[],compact=false}:{projectId:number;deviceId:number;deviceName:string;onChanged:()=>Promise<void>;compact?:boolean;workflowActions:{key:string;label:string}[]}) {
 const endpoint=`/api/projects/${projectId}/devices/${deviceId}/flighthub-operations`;
 const [model,setModel]=useState<Model|null>(null),[error,setError]=useState(""),[pending,setPending]=useState(false),[selected,setSelected]=useState<Operation|null>(null),[parameters,setParameters]=useState<Record<string,string>>({}),[notice,setNotice]=useState(""),[keepControl,setKeepControl]=useState(false),[executionKey,setExecutionKey]=useState("");
 const load=useCallback(async()=>{try{const response=await apiFetch(endpoint,{cache:"no-store"});const data=await response.json();if(!response.ok)throw new Error(errorMessage(data));setModel(data);}catch(cause){setError(cause instanceof Error?cause.message:"操作状态读取失败");}},[endpoint]);
 useEffect(()=>{setModel(null);setSelected(null);setError("");setNotice("");setKeepControl(false);void load();const timer=setInterval(()=>void load(),10000);return()=>clearInterval(timer);},[load]);
 const ownedSession=model?.controlSessions.find(session=>session.owned&&session.status==="active");
 useEffect(()=>{if(!keepControl||!ownedSession)return;const timer=setInterval(()=>{void apiFetch(`/api/projects/${projectId}/devices/${deviceId}/flighthub-control-sessions/${ownedSession.id}`,{method:"PATCH",headers:{"content-type":"application/json"},body:JSON.stringify({action:"heartbeat"})}).then(async response=>{if(!response.ok){setKeepControl(false);setError(errorMessage(await response.json()));}}).catch(()=>{setKeepControl(false);setError("控制权续期失败，请刷新状态。");});},5000);return()=>clearInterval(timer);},[keepControl,ownedSession?.id,projectId,deviceId]);
 async function post(body:Record<string,unknown>){setPending(true);setError("");setNotice("");try{const response=await apiFetch(endpoint,{method:"POST",headers:{"content-type":"application/json"},body:JSON.stringify(body)});const data=await response.json();if(!response.ok)throw new Error(errorMessage(data));setNotice(`操作已受理：${statusLabels[data.status]??data.status??"等待后台处理"}。`);setSelected(null);setParameters({});await load();await onChanged();}catch(cause){setError(cause instanceof Error?cause.message:"操作失败");}finally{setPending(false);}}
 async function release(session:ControlSession){setPending(true);setKeepControl(false);try{const response=await apiFetch(`/api/projects/${projectId}/devices/${deviceId}/flighthub-control-sessions/${session.id}`,{method:"PATCH",headers:{"content-type":"application/json"},body:JSON.stringify({action:"release"})});if(!response.ok)throw new Error(errorMessage(await response.json()));setNotice("已请求释放控制权。");await load();}catch(cause){setError(cause instanceof Error?cause.message:"释放失败");}finally{setPending(false);}}
 const groups=[...new Set([...(workflowActions.length?["飞行控制"]:[]),...(model?.operations.map(operation=>operation.group)??[])])];
 return <div className="space-y-4">
  {!model&&!error&&<p className="text-sm text-muted-foreground">正在读取设备操作…</p>}
  <div className={compact?"flex flex-wrap gap-x-6 gap-y-3":"space-y-4"}>{groups.map(group=><div className="space-y-2" key={group}><h3 className="text-xs font-medium text-muted-foreground">{group}</h3><div className="flex flex-wrap gap-2">{group==="飞行控制"&&workflowActions.map(action=>action.key==="mission.create"?<FlightHubFlightLaunch key={`workflow:${action.key}`} projectId={projectId} deviceId={deviceId} deviceName={deviceName}/>:<Button asChild key={`workflow:${action.key}`} size="sm" variant="outline"><Link href={projectPageHref(projectId,"tasks")}>{action.label}</Link></Button>)}{model?.operations.filter(operation=>operation.group===group).map(operation=><Button key={operation.key} size="sm" variant="outline" disabled={pending||!operation.canRequest} onClick={()=>{setSelected(operation);setParameters({});setExecutionKey(crypto.randomUUID());setError("");}}>{operation.label}</Button>)}</div></div>)}</div>
  {model?.controlSessions.map(session=><div key={session.id} className="space-y-2 border-t pt-3 text-sm"><p>{statusLabels[session.status]??session.status}<span className="inline-block whitespace-pre-line">{!session.owned&&"\n由其他成员持有"}</span></p>{session.owned&&<div className="flex flex-wrap items-center gap-3">{session.status==="active"&&<label className="flex items-center gap-2"><input type="checkbox" checked={keepControl} onChange={event=>setKeepControl(event.target.checked)}/>在此页面保持控制权</label>}<Button size="sm" variant="outline" disabled={pending||session.status==="releasing"} onClick={()=>void release(session)}>释放控制权</Button></div>}</div>)}
  {Boolean(model?.executions.length)&&<details className="border-t pt-3"><summary className="cursor-pointer text-sm font-medium">操作记录</summary><div className="mt-3 divide-y">{model?.executions.map(item=><div key={item.id} className="py-2 text-sm"><p><span className="inline-flex max-w-full flex-wrap items-center gap-x-3 gap-y-1"><span>{item.label}</span><span>{item.requestedBy}</span></span></p><p className="text-xs text-muted-foreground">{item.receipt ? statusLabels[item.receipt.status]??item.receipt.status : "未下发"}<span className="inline-block whitespace-pre-line">{item.receipt?.error&&`\n${item.receipt.error}`}</span></p></div>)}</div></details>}
  {notice&&<p role="status" className="text-sm text-muted-foreground">{notice}</p>}{error&&<p role="alert" className="text-sm text-destructive">{error}</p>}
  <Dialog open={selected!==null} onOpenChange={open=>{if(!open&&!pending){setSelected(null);setParameters({});}}}><DialogContent className="max-h-[85vh] overflow-y-auto"><DialogHeader><DialogTitle>{selected?.label}</DialogTitle><DialogDescription>对 {deviceName} 执行此操作。{selected?.key==="active-project-update"&&"修改项目绑定会影响设备所属项目。"}</DialogDescription></DialogHeader><form className="space-y-4" onSubmit={event=>{event.preventDefault();if(!selected)return;const values=Object.fromEntries(selected.fields.filter(field=>(parameters[field.key]??"").trim()!=="").map(field=>[field.key,field.type==="number"?Number(parameters[field.key]):parameters[field.key]]));void post({action:"execute",operation:selected.key,parameters:values,idempotencyKey:executionKey});}}>
   {selected?.fields.map(field=><label className="grid gap-1 text-sm" key={field.key}>{field.label}{field.type==="select"?<select className="h-9 rounded-md border bg-background px-3" required={field.required} value={parameters[field.key]??""} onChange={event=>{setParameters(current=>({...current,[field.key]:event.target.value}));setExecutionKey(crypto.randomUUID());}}><option value="">请选择</option>{field.options?.map(option=><option value={option.value} key={option.value}>{option.label}</option>)}</select>:<Input type={field.type} required={field.required} value={parameters[field.key]??""} onChange={event=>{setParameters(current=>({...current,[field.key]:event.target.value}));setExecutionKey(crypto.randomUUID());}} disabled={pending}/>} {field.hint&&<span className="text-xs text-muted-foreground">{field.hint}</span>}</label>)}
   {selected?.key==="camera.change_lens"&&<p className="text-xs text-muted-foreground">切换镜头前，请先取得该相机的负载控制权。</p>}
   {Boolean(selected?.blockers.length)&&<div className="space-y-1 text-sm text-amber-700"><p>当前无法执行：</p><ul className="list-disc space-y-1 pl-5">{selected?.blockers.map(blocker=><li key={blocker}>{blocker}</li>)}</ul></div>}
   {error&&<p role="alert" className="text-sm text-destructive">{error}</p>}<DialogFooter><Button type="button" variant="outline" disabled={pending} onClick={()=>{setSelected(null);setParameters({});}}>取消</Button><Button type="submit" disabled={pending||!selected?.available}>执行</Button></DialogFooter>
  </form></DialogContent></Dialog>
 </div>;
}

