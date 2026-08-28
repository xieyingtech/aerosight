import Link from "next/link";
import { EvidenceImage } from "@/components/evidence-image";
import { IssueCollaborationPanel } from "@/components/issue-collaboration-panel";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { issueEvidenceSummary } from "@/lib/issue-view-core";
import { readIssue } from "@/lib/issues";

function displayDate(value: unknown) {
  if (!value) return "—";
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "medium" }).format(new Date(String(value)));
}

const activityLabels: Record<string, string> = {
  "copilot.requested": "已请求 Copilot",
  "copilot.accepted": "Copilot 已接收",
  "copilot.progress": "Copilot 正在整理证据",
  "copilot.completed": "Copilot 已生成草案",
  "copilot.failed": "Copilot 处理失败",
  "comment.created": "添加评论",
  "assignee.added": "添加负责人",
  "assignee.removed": "移除负责人",
  "status.changed": "变更状态",
  "labels.changed": "更新标签"
};

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
        {model.events.length ? <ol className="space-y-3">{model.events.map((event) => <li className="border-l-2 pl-4 text-sm" key={String(event.id)}><div className="flex flex-wrap justify-between gap-2"><span className="font-medium">{activityLabels[String(event.eventType)] ?? String(event.eventType)}</span><time className="text-muted-foreground">{displayDate(event.createdAt)}</time></div><p className="text-muted-foreground">{String(event.actorName)}{event.body ? ` · ${String(event.body)}` : ""}</p></li>)}</ol> : <p className="text-sm text-muted-foreground">暂无活动记录。</p>}
      </CardContent></Card>
      {model.drafts.length ? <Card><CardHeader><CardTitle>Copilot 草案</CardTitle><CardDescription>草案只提供建议，不会自动执行任务、算法或设备命令。</CardDescription></CardHeader><CardContent className="space-y-4">
        {model.drafts.map((draft) => { const payload = (draft.payload ?? {}) as Record<string, unknown>; return <article className="space-y-2 rounded-lg border p-4" key={String(draft.id)}><div className="flex flex-wrap items-center justify-between gap-2"><h3 className="font-medium">{String(draft.title)}</h3><Badge variant="outline">待人工确认</Badge></div><p className="whitespace-pre-wrap text-sm">{String(payload.analysis ?? "草案内容不可用")}</p><p className="text-xs text-muted-foreground">模型 {String(draft.modelId)} · 提示模板 {String(draft.promptTemplateVersion)} · 证据快照 {String(draft.evidenceVersionHash).slice(0, 12)}</p></article>; })}
      </CardContent></Card> : null}
      <Card><CardHeader><CardTitle>协作处置</CardTitle><CardDescription>评论、标签、状态以及成员/智能体指派受项目权限和乐观并发保护。</CardDescription></CardHeader><CardContent>
        <IssueCollaborationPanel agents={model.agents} assignees={model.assignees} canAssign={model.canAssign} canHandle={model.canHandle} canUseAgent={model.canUseAgent}
          issueId={Number(issue.id)} labels={labels.map(String)} members={model.members} projectId={projectId}
          stateVersion={Number(issue.stateVersion)} status={String(issue.status)} />
      </CardContent></Card>
    </div>
  </Page>;
}
