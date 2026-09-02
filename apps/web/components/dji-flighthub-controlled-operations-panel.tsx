"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { RefreshCwIcon, ShieldAlertIcon } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

type Action = { capabilityCode: string; label: string; domain: string; risk: "high" | "critical"; featureFlag: string;
  prerequisites: string[]; approval: string; resultEvidence: string; href: string; available: boolean; missing: string[];
  connectorReady: boolean; permissionReady: boolean; featureEnabled: boolean; capabilityVerified: boolean };
type Job = { id: string; action: string; status: string; lastErrorCode: string | null; completedAt: string | null; updatedAt: string };
type Payload = { actions: Action[]; jobs: Job[] };

function date(value: string | null) { return value ? new Date(value).toLocaleString("zh-CN") : "—"; }

export function DjiFlightHubControlledOperationsPanel({ projectId, connectorId }: { projectId: number; connectorId: string }) {
  const [payload,setPayload]=useState<Payload|null>(null),[loading,setLoading]=useState(false),[error,setError]=useState<string|null>(null);
  const load=async()=>{setLoading(true);setError(null);try{const response=await fetch(`/api/projects/${projectId}/connectors/dji-flighthub/${connectorId}/controlled-operations`,{cache:"no-store"});
    const data=await response.json() as Payload&{error?:string};if(!response.ok)throw new Error(data.error??"受控操作读取失败");setPayload(data);
  }catch(cause){setError(cause instanceof Error?cause.message:"受控操作读取失败");}finally{setLoading(false);}};
  useEffect(()=>{void load();},[projectId,connectorId]); // eslint-disable-line react-hooks/exhaustive-deps
  return <section className="space-y-3 rounded-lg border p-3">
    <div className="flex flex-wrap items-center justify-between gap-2"><div><h3 className="flex items-center gap-2 text-sm font-medium"><ShieldAlertIcon className="size-4"/>受控操作</h3>
      <p className="mt-1 text-xs text-muted-foreground">可用性由服务端对官方契约、连接状态、权限、功能开关与 field-write 证据求交集；页面状态不能替代 API 门禁。</p></div>
      <Button disabled={loading} onClick={()=>void load()} size="sm" type="button" variant="ghost"><RefreshCwIcon className={loading?"animate-spin":""}/>刷新</Button></div>
    {error&&<p className="rounded-md bg-destructive/5 p-2 text-xs text-destructive" role="alert">{error}</p>}
    {payload&&<><div className="grid gap-2 lg:grid-cols-2">{payload.actions.map(action=><article className="space-y-2 rounded-md border p-3" key={action.capabilityCode}>
      <div className="flex flex-wrap items-center justify-between gap-2"><div><p className="text-sm font-medium">{action.label}</p><code className="text-[11px] text-muted-foreground">{action.capabilityCode}</code></div>
        <div className="flex gap-1"><Badge variant={action.risk==="critical"?"destructive":"secondary"}>风险 · {action.risk}</Badge><Badge variant={action.available?"default":"outline"}>{action.available?"可用":"关闭"}</Badge></div></div>
      <dl className="grid gap-1 text-xs sm:grid-cols-2"><div><dt className="text-muted-foreground">前置条件</dt><dd>{action.prerequisites.join("、")}</dd></div>
        <div><dt className="text-muted-foreground">审批</dt><dd>{action.approval}</dd></div><div><dt className="text-muted-foreground">功能开关</dt><dd><code>{action.featureFlag}</code> · {action.featureEnabled?"已开启":"未开启"}</dd></div>
        <div><dt className="text-muted-foreground">最终结果</dt><dd>{action.resultEvidence}</dd></div></dl>
      {action.missing.length>0&&<p className="rounded bg-muted/50 p-2 text-xs text-muted-foreground">缺失：{action.missing.join("；")}</p>}
      <Button asChild={action.available} disabled={!action.available} size="sm" type="button" variant="outline">{action.available?<Link href={action.href}>前往操作入口</Link>:<span>操作不可用</span>}</Button>
    </article>)}</div>
      <details className="rounded-md border p-2"><summary className="cursor-pointer text-sm font-medium">最近最终结果 / 待对账 · {payload.jobs.length}</summary>
        <div className="mt-2 overflow-x-auto"><Table><TableHeader><TableRow><TableHead>操作</TableHead><TableHead>状态</TableHead><TableHead>安全结果</TableHead><TableHead>更新时间</TableHead></TableRow></TableHeader>
          <TableBody>{payload.jobs.map(job=><TableRow key={job.id}><TableCell>{job.action}</TableCell><TableCell><Badge variant={job.status==="failed"||job.status==="blocked"?"destructive":"outline"}>{job.status}</Badge></TableCell>
            <TableCell>{job.lastErrorCode??(job.completedAt?"已完成":"等待远端证据")}</TableCell><TableCell>{date(job.completedAt??job.updatedAt)}</TableCell></TableRow>)}
            {payload.jobs.length===0&&<TableRow><TableCell className="text-center text-muted-foreground" colSpan={4}>暂无受控操作记录。</TableCell></TableRow>}</TableBody></Table></div></details></>}
  </section>;
}
