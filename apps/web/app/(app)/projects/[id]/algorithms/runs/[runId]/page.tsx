import { AlgorithmRunRetryButton } from "@/components/algorithm-run-retry-button";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { readAlgorithmRun } from "@/lib/algorithm-runs";

export default async function AlgorithmRunDetailPage({ params }: { params: Promise<{ id: string; runId: string }> }) {
  const { id, runId } = await params;
  const projectId = Number(id);
  const { run, attempts, view } = await readAlgorithmRun(projectId, runId);
  return <Page title={run.definitionName} description={`算法运行 ${run.id}`} actions={view.retryAllowed ? <AlgorithmRunRetryButton projectId={projectId} runId={run.id} /> : undefined}>
    <div className="space-y-4">
      <div className="grid gap-4 md:grid-cols-3">
        <Card><CardHeader><CardDescription>状态</CardDescription><CardTitle><Badge variant={run.status === "failed" || run.status === "timed_out" ? "destructive" : "outline"}>{run.status}</Badge></CardTitle></CardHeader><CardContent className="text-sm text-muted-foreground">{run.errorCode ? `${run.errorCode}：${run.errorMessage ?? "无错误详情"}` : "运行未报告错误"}</CardContent></Card>
        <Card><CardHeader><CardDescription>耗时</CardDescription><CardTitle>{view.durationMs === null ? "尚未开始" : `${(view.durationMs / 1000).toFixed(2)} 秒`}</CardTitle></CardHeader><CardContent className="text-xs text-muted-foreground">开始 {run.startedAt?.toLocaleString("zh-CN") ?? "-"}<br />结束 {run.finishedAt?.toLocaleString("zh-CN") ?? "-"}</CardContent></Card>
        <Card><CardHeader><CardDescription>版本</CardDescription><CardTitle>定义 v{run.definitionVersion}</CardTitle></CardHeader><CardContent className="text-xs text-muted-foreground">{view.version.providerType} · {view.version.modelOrProcess}<br />mapping {view.version.mappingVersion ?? "-"}</CardContent></Card>
      </div>
      <div className="grid gap-4 lg:grid-cols-2">
        <Card><CardHeader><CardTitle>输入</CardTitle><CardDescription>短时签名地址和 callback token 不展示</CardDescription></CardHeader><CardContent className="space-y-3 text-sm"><p>资产 #{view.input.assetId ?? run.inputAssetId} · v{view.input.assetVersion ?? "-"} · {view.input.mimeType ?? "未知类型"}</p><pre className="overflow-auto rounded-lg bg-muted p-3 text-xs">{JSON.stringify({ parameters: view.input.parameters, context: view.input.context }, null, 2)}</pre></CardContent></Card>
        <Card><CardHeader><CardTitle>原始结果引用</CardTitle><CardDescription>原始响应保存在受控对象存储，不写入数据库正文</CardDescription></CardHeader><CardContent className="space-y-2 text-xs">{view.rawResult ? <><p className="break-all font-mono">{view.rawResult.objectKey}</p><p className="break-all text-muted-foreground">SHA-256 {view.rawResult.checksumSha256}</p></> : <p className="text-muted-foreground">尚无原始结果</p>}</CardContent></Card>
      </div>
      <Card><CardHeader><CardTitle>Mapping 诊断</CardTitle></CardHeader><CardContent>{view.diagnostics.length ? <ul className="space-y-2">{view.diagnostics.map((diagnostic) => <li className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm" key={diagnostic}>{diagnostic}</li>)}</ul> : <p className="text-sm text-muted-foreground">无 mapping 诊断</p>}</CardContent></Card>
      <Card><CardHeader><CardTitle>调用尝试</CardTitle></CardHeader><CardContent className="space-y-2">{attempts.length ? attempts.map((attempt) => <div className="grid gap-2 rounded-lg border p-3 text-sm md:grid-cols-[70px_120px_120px_1fr]" key={String(attempt.attempt)}><span>#{String(attempt.attempt)}</span><Badge variant="outline">{String(attempt.status)}</Badge><span>{String(attempt.durationMs ?? "-")} ms</span><span className="text-muted-foreground">HTTP {String(attempt.responseStatus ?? "-")} · {String(attempt.errorCategory ?? "无错误")}</span></div>) : <p className="text-sm text-muted-foreground">尚无 attempt 记录</p>}</CardContent></Card>
      {!view.retryAllowed && ["failed", "timed_out"].includes(run.status) ? <p className="text-sm text-muted-foreground">当前账号无权重试此失败运行。</p> : null}
    </div>
  </Page>;
}
