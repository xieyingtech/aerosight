import Link from "next/link";
import { EvidenceImage } from "@/components/evidence-image";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { issueEvidenceSummary } from "@/lib/issue-view-core";
import { readIssue } from "@/lib/issues";

function displayDate(value: unknown) {
  if (!value) return "—";
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "medium" }).format(new Date(String(value)));
}

export default async function IssueDetailPage({ params }: { params: Promise<{ id: string; issueId: string }> }) {
  const { id, issueId } = await params;
  const projectId = Number(id);
  const model = await readIssue(projectId, Number(issueId));
  const issue = model.issue;
  const summary = issueEvidenceSummary({ detections: model.detections, assets: model.assets });
  const labels = Array.isArray(issue.labels) ? issue.labels : [];
  return <Page title={`案件 #${String(issue.number)} · ${String(issue.title)}`} description="案件是可协作处置的业务记录；算法结果和原始证据保持不可变。">
    <div className="space-y-4">
      <div className="grid gap-4 md:grid-cols-4">
        <Card><CardHeader><CardDescription>状态</CardDescription><CardTitle><Badge variant="outline">{String(issue.status)}</Badge></CardTitle></CardHeader><CardContent className="text-sm">优先级 {String(issue.priority)}</CardContent></Card>
        <Card><CardHeader><CardDescription>出现次数</CardDescription><CardTitle>{String(issue.occurrenceCount)}</CardTitle></CardHeader><CardContent className="text-sm text-muted-foreground">最近 {displayDate(issue.lastSeenAt)}</CardContent></Card>
        <Card><CardHeader><CardDescription>位置</CardDescription><CardTitle>{summary.hasMapLocation ? "可在地图展示" : "仅影像级"}</CardTitle></CardHeader><CardContent className="text-sm text-muted-foreground">{summary.locationLabel}</CardContent></Card>
        <Card><CardHeader><CardDescription>证据完整性</CardDescription><CardTitle>{summary.completeEvidence ? "已关联" : "待补充"}</CardTitle></CardHeader><CardContent className="text-sm text-muted-foreground">{summary.detectionCount} 条检测 · {summary.assetCount} 个媒体</CardContent></Card>
      </div>

      <Card><CardHeader><CardTitle>案件说明</CardTitle><CardDescription>{labels.length ? labels.map(String).join(" · ") : "暂无标签"}</CardDescription></CardHeader><CardContent className="space-y-2 text-sm">
        <p>{String(issue.description || "暂无补充说明")}</p>
        <p className="text-muted-foreground">任务：{issue.taskRunId ? <Link className="underline" href={`/projects/${projectId}/tasks/runs/${String(issue.taskRunId)}`}>{String(issue.taskName || "任务")} · Run #{String(issue.taskRunId)}</Link> : "手动案件"}</p>
        {issue.taskVersionId ? <p className="text-muted-foreground">任务版本：v{String(issue.taskVersion || "—")}（快照 #{String(issue.taskVersionId)}） · 条件范围 {String(issue.conditionScopeKey || "—")}</p> : null}
      </CardContent></Card>

      <Card><CardHeader><CardTitle>检测与模型证据</CardTitle><CardDescription>显示算法、模型配置快照、置信度、空间质量和原始资产版本。</CardDescription></CardHeader><CardContent className="space-y-5">
        {model.detections.length ? model.detections.map((detection) => <div className="grid gap-4 border-b pb-5 last:border-0 lg:grid-cols-2" key={String(detection.id)}>
          <EvidenceImage assetId={Number(detection.inputAssetId)} projectId={projectId} />
          <div className="space-y-2 text-sm">
            <div className="flex flex-wrap gap-2"><Badge>{String(detection.label)}</Badge><Badge variant="outline">置信度 {Number(detection.confidence).toFixed(2)}</Badge></div>
            <p>算法：{String(detection.algorithmName)} · {String(detection.modelOrProcess)} · 配置 v{String(detection.algorithmVersion)}</p>
            <p>位置质量：{String(detection.locationQuality)} · 投影 {String(detection.projectionMethod)} · mapping {String(detection.mappingVersion)}</p>
            <p>原始资产：#{String(detection.inputAssetId)} v{String(detection.assetVersion)} · 校验 {String(detection.assetChecksumSha256 || "未记录")}</p>
            <pre className="overflow-auto rounded-lg bg-muted p-3 text-xs">像素标注 {JSON.stringify(detection.pixelGeometry, null, 2)}</pre>
          </div>
        </div>) : <p className="text-sm text-muted-foreground">此案件尚未关联检测记录。</p>}
      </CardContent></Card>

      <Card><CardHeader><CardTitle>活动时间线</CardTitle><CardDescription>任务自动建案、合并以及后续评论和处置都记录在这里。</CardDescription></CardHeader><CardContent>
        {model.events.length ? <ol className="space-y-3">{model.events.map((event) => <li className="border-l-2 pl-4 text-sm" key={String(event.id)}><div className="flex flex-wrap justify-between gap-2"><span className="font-medium">{String(event.eventType)}</span><time className="text-muted-foreground">{displayDate(event.createdAt)}</time></div><p className="text-muted-foreground">{String(event.actorName)}{event.body ? ` · ${String(event.body)}` : ""}</p></li>)}</ol> : <p className="text-sm text-muted-foreground">暂无活动记录。</p>}
      </CardContent></Card>
    </div>
  </Page>;
}
