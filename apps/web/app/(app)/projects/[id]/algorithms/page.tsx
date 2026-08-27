import Link from "next/link";
import { AlgorithmProviderForm } from "@/components/algorithm-provider-form";
import { AlgorithmCatalog } from "@/components/algorithm-catalog";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { listAlgorithmProviders } from "@/lib/algorithm-providers";
import { listAlgorithmCatalog } from "@/lib/algorithm-catalog";
import { listAlgorithmRuns } from "@/lib/algorithm-runs";
import { requireCurrentProjectPermission } from "@/lib/data";

export default async function AlgorithmsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const projectId = Number(id);
  const { access } = await requireCurrentProjectPermission(projectId, "project:view");
  const canManage = access.permissions.has("algorithm:manage");
  const [runs, providers, catalog] = await Promise.all([listAlgorithmRuns(projectId), canManage ? listAlgorithmProviders(projectId) : Promise.resolve([]), listAlgorithmCatalog(projectId)]);
  return <Page title="算法运行" description="跟踪外部算法输入、版本、耗时、重试与原始结果证据"><div className="space-y-6">
    <Card><CardHeader><CardTitle>最近运行</CardTitle></CardHeader><CardContent className="space-y-2">{runs.length ? runs.map((run) => <Link className="grid gap-2 rounded-lg border p-3 transition-colors hover:bg-muted/40 md:grid-cols-[1fr_160px_140px]" href={`/projects/${projectId}/algorithms/runs/${run.id}`} key={run.id}><div><p className="font-medium">{run.definitionName}</p><p className="text-xs text-muted-foreground">{run.providerName} · 定义 v{run.definitionVersion} · 资产 #{run.inputAssetId}</p></div><Badge className="w-fit" variant={run.status === "failed" || run.status === "timed_out" ? "destructive" : "outline"}>{run.status}</Badge><time className="text-xs text-muted-foreground">{run.createdAt.toLocaleString("zh-CN")}</time></Link>) : <p className="text-sm text-muted-foreground">尚无算法运行</p>}</CardContent></Card>
    <div><h2 className="text-lg font-semibold">算法目录</h2><p className="text-sm text-muted-foreground">定义、参数与结果类型均来自已发布 schema</p></div><AlgorithmCatalog canRun={canManage} entries={catalog} projectId={projectId} />
    {canManage ? <><div><h2 className="text-lg font-semibold">算法服务配置</h2><p className="text-sm text-muted-foreground">仅保存秘密引用，连接凭据不会回显</p></div><AlgorithmProviderForm projectId={projectId} /><div className="grid gap-3 md:grid-cols-2">{providers.length ? providers.map((provider) => <Card key={String(provider.id)}><CardHeader><CardTitle>{provider.name}</CardTitle></CardHeader><CardContent className="space-y-2 text-sm"><div className="flex gap-2"><Badge variant="outline">{provider.providerType}</Badge><Badge variant="outline">{provider.status}</Badge></div><p className="truncate text-muted-foreground">{provider.baseUrl}</p><p>{provider.secretConfigured ? "秘密引用已配置" : "未配置认证秘密"}</p></CardContent></Card>) : <p className="text-sm text-muted-foreground">尚未配置算法服务</p>}</div></> : <p className="text-sm text-muted-foreground">当前账号可查看运行，但不能配置服务或重试失败运行。</p>}
  </div></Page>;
}
