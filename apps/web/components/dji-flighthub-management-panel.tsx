"use client";

import { useEffect, useMemo, useState } from "react";
import { RefreshCwIcon, SearchIcon, ShieldCheckIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

type Resource={id:string;connectorId:string;kind:string;status:string;summary:Record<string,string|number|boolean|null>;lastSeenAt:string;missingAt:string|null};
type Payload={connector:{id:string;name:string;status:string};managementCapabilityVerified:boolean;syncState:{status?:string;lastSucceededAt?:string;lastErrorCode?:string|null}|null;resources:Resource[]};
type JoinResult={projectName:string;organizationName:string;userInOrganization:boolean;recommendedUserCallsign:string;recommendedDroneCallsign:string|null};

const kindLabels:Record<string,string>={organization:"组织", "organization-user":"组织用户 / 当前身份", "organization-role":"组织角色",
  "organization-permission":"组织权限", "project-user":"项目用户", "project-member":"在线成员"};

function text(summary:Resource["summary"],...keys:string[]){
  for(const key of keys){const value=summary[key];if(typeof value==="string"&&value.trim())return value;}
  return "—";
}

export function DjiFlightHubManagementPanel({projectId,connectorId}:{projectId:number;connectorId:string}){
  const [payload,setPayload]=useState<Payload|null>(null),[loading,setLoading]=useState(false),[error,setError]=useState<string|null>(null);
  const [projectCode,setProjectCode]=useState(""),[joinCode,setJoinCode]=useState(""),[droneSN,setDroneSN]=useState("");
  const [joinResult,setJoinResult]=useState<JoinResult|null>(null),[joinLoading,setJoinLoading]=useState(false);
  const load=async()=>{setLoading(true);setError(null);try{const response=await fetch(`/api/projects/${projectId}/connectors/dji-flighthub/${connectorId}/management`,{cache:"no-store"});
    const data=await response.json() as Payload&{error?:string};if(!response.ok)throw new Error(data.error??"读取失败");setPayload(data);}catch(cause){setError(cause instanceof Error?cause.message:"读取失败");}finally{setLoading(false);}};
  useEffect(()=>{void load();},[projectId,connectorId]); // eslint-disable-line react-hooks/exhaustive-deps
  const groups=useMemo(()=>Object.entries((payload?.resources??[]).reduce<Record<string,Resource[]>>((result,item)=>{
    (result[item.kind]??=[]).push(item);return result;
  },{})),[payload]);
  const lookup=async()=>{if(!projectCode.trim()||!joinCode.trim())return;setJoinLoading(true);setJoinResult(null);setError(null);
    try{const response=await fetch(`/api/projects/${projectId}/connectors/dji-flighthub/${connectorId}/management`,{method:"POST",headers:{"content-type":"application/json"},cache:"no-store",
      body:JSON.stringify({projectCode:projectCode.trim(),fastJoinCode:joinCode.trim(),...(droneSN.trim()?{associationDroneSN:droneSN.trim()}:{})})});
      const data=await response.json() as JoinResult&{error?:string};if(!response.ok)throw new Error(data.error??"加入码查询失败");setJoinResult(data);
    }catch(cause){setError(cause instanceof Error?cause.message:"加入码查询失败");}finally{setJoinCode("");setDroneSN("");setJoinLoading(false);}};
  return <section className="space-y-3 rounded-lg border p-3">
    <div className="flex flex-wrap items-center justify-between gap-2"><div><div className="flex items-center gap-2"><ShieldCheckIcon className="size-4"/><h3 className="text-sm font-medium">组织与项目管理（只读）</h3>
      {payload?.managementCapabilityVerified&&<Badge variant="outline">管理域已验证</Badge>}</div><p className="mt-1 text-xs text-muted-foreground">组织管理权限独立于设备操作；不会展示远端标识、离线坐标、控制设备 SN 或认证详情。</p></div>
      <Button disabled={loading} onClick={()=>void load()} size="sm" type="button" variant="ghost"><RefreshCwIcon className={loading?"animate-spin":""}/>刷新</Button></div>
    {error&&<p className="rounded-md bg-destructive/5 p-2 text-xs text-destructive" role="alert">{error}</p>}
    {payload&&<><div className="grid gap-2 sm:grid-cols-3"><div className="rounded-md bg-muted/35 p-2 text-xs"><span className="text-muted-foreground">同步状态</span><p className="mt-1 font-medium">{payload.syncState?.status??"等待同步"}</p></div>
      <div className="rounded-md bg-muted/35 p-2 text-xs"><span className="text-muted-foreground">最近成功</span><p className="mt-1 font-medium">{payload.syncState?.lastSucceededAt?new Date(payload.syncState.lastSucceededAt).toLocaleString("zh-CN"):"—"}</p></div>
      <div className="rounded-md bg-muted/35 p-2 text-xs"><span className="text-muted-foreground">投影记录</span><p className="mt-1 font-medium">{payload.resources.length}</p></div></div>
      {groups.map(([kind,items])=><details className="rounded-md border p-2" key={kind} open={kind==="organization"||kind==="project-member"}><summary className="cursor-pointer text-sm font-medium">{kindLabels[kind]??kind} · {items?.length??0}</summary>
        <div className="mt-2 overflow-x-auto"><Table><TableHeader><TableRow><TableHead>名称 / 账号</TableHead><TableHead>角色 / 类型</TableHead><TableHead>状态</TableHead><TableHead>最近同步</TableHead></TableRow></TableHeader>
          <TableBody>{(items??[]).map(item=><TableRow key={item.id}><TableCell>{text(item.summary,"name","nickname","projectCallsign","account")}</TableCell>
            <TableCell>{text(item.summary,"projectRole","role","roleType","permissionType","organizationRole")}</TableCell><TableCell><Badge variant="outline">{typeof item.summary.online==="boolean"?(item.summary.online?"在线":"离线"):item.status}</Badge></TableCell>
            <TableCell className="text-xs">{new Date(item.lastSeenAt).toLocaleString("zh-CN")}</TableCell></TableRow>)}</TableBody></Table></div></details>)}</>}
    <details className="rounded-md border p-2"><summary className="cursor-pointer text-sm font-medium">按加入码核验当前司空项目</summary><div className="mt-3 grid gap-2 md:grid-cols-[1fr_1fr_1fr_auto]">
      <Input onChange={event=>setProjectCode(event.target.value)} placeholder="司空项目编号" value={projectCode}/><Input autoComplete="off" onChange={event=>setJoinCode(event.target.value)} placeholder="快速加入码" type="password" value={joinCode}/>
      <Input autoComplete="off" onChange={event=>setDroneSN(event.target.value)} placeholder="关联飞机 SN（可选）" type="password" value={droneSN}/><Button disabled={joinLoading||!projectCode.trim()||!joinCode.trim()} onClick={()=>void lookup()} type="button"><SearchIcon/>核验</Button></div>
      {joinResult&&<div className="mt-2 rounded-md bg-muted/35 p-2 text-xs"><p className="font-medium">{joinResult.organizationName} / {joinResult.projectName}</p><p className="mt-1 text-muted-foreground">当前用户{joinResult.userInOrganization?"已在":"不在"}组织 · 建议称呼 {joinResult.recommendedUserCallsign||"—"} · 飞机称呼 {joinResult.recommendedDroneCallsign||"—"}</p></div>}</details>
  </section>;
}
