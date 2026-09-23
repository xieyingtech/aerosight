"use client";
import Link from "next/link";
import { AlgorithmCatalog } from "@/components/algorithm-catalog";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import type { AlgorithmCatalogEntry } from "@/lib/web-api-types";
import type { AlgorithmRunView } from "@/lib/web-api-types";


import { positiveParam, StaticAPIPage } from "@/components/static-api-page";
import { APIStateView } from "@/components/api-state";
import { useAPI } from "@/lib/use-api";
import { effectiveProjectPermissions, type ProjectTeamRole } from "@/lib/project-permission-policy";
type Project={id:number;role:ProjectTeamRole;permissions:string[]};
export default function AlgorithmsPage(){
 return <StaticAPIPage<Project> endpoint={(query)=>{const id=positiveParam(query);return id?`/api/projects/${id}`:null;}}>
 {(project)=><Workspace key={project.id} project={project}/>}
 </StaticAPIPage>;
}
function Workspace({project}:{project:Project}){
 const projectId=project.id;
 const canManage=effectiveProjectPermissions(project.role,project.permissions).has("algorithm:manage");
 const runsState=useAPI<AlgorithmRunView[]>(`/api/projects/${projectId}/algorithm-runs`);
 const catalogState=useAPI<{definitions:AlgorithmCatalogEntry[]}>(`/api/projects/${projectId}/algorithm-definitions`);
 return <APIStateView state={runsState}>{runs=><APIStateView state={catalogState}>{catalog=>
  <Page title="算法运行" description="跟踪外部算法输入、Provider 模型来源、耗时、重试与原始结果证据"><div className="space-y-6">
    <Card><CardHeader><CardTitle>最近运行</CardTitle></CardHeader><CardContent className="space-y-2">{runs.length ? runs.map((run) => <Link className="grid gap-2 rounded-lg border p-3 transition-colors hover:bg-muted/40 md:grid-cols-[1fr_160px_140px]" href={`/projects/algorithms/runs/detail/?projectId=${projectId}&runId=${run.id}`} key={run.id}><div><p className="font-medium">{run.definitionName}</p><p className="text-xs text-muted-foreground">{run.providerName} · 资产 #{run.inputAssetId}</p></div><Badge className="w-fit" variant={run.status === "failed" || run.status === "timed_out" ? "destructive" : "outline"}>{run.status}</Badge><time className="text-xs text-muted-foreground">{new Date(run.createdAt).toLocaleString("zh-CN")}</time></Link>) : <p className="text-sm text-muted-foreground">尚无算法运行</p>}</CardContent></Card>
    <div><h2 className="text-lg font-semibold">算法目录</h2><p className="text-sm text-muted-foreground">定义、参数与结果类型来自当前保存的配置；模型版本由 Provider 管理</p></div><AlgorithmCatalog canRun={canManage} entries={catalog.definitions} projectId={projectId} />
    {!canManage ? <p className="text-sm text-muted-foreground">当前账号可查看运行，但不能管理项目算法定义或重试失败运行。</p> : null}
  </div></Page>
 }</APIStateView>}</APIStateView>;
}
