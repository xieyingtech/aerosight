"use client";
import { canonicalPageHref } from "@/lib/page-routes";

import Link from "next/link";
import { EvidenceImage } from "@/components/evidence-image";
import { IssueCollaborationPanel } from "@/components/issue-collaboration-panel";
import { IssueFeedbackPanel } from "@/components/issue-feedback-panel";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
import { CircleDotIcon, CircleCheckIcon, ArrowLeftIcon, MessageSquareIcon, GitBranchIcon, PaperclipIcon } from "lucide-react";
import { issueEvidenceSummary, issuePriorityLabel } from "@/lib/issue-view-core";
import type { IssueDetail } from "@/lib/web-api-types";

function displayDate(value: unknown) {
  if (!value) return "—";
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "medium" }).format(new Date(String(value)));
}

const activityLabels: Record<string, string> = {
  "issue.created": "创建案件",
  "issue.updated": "更新案件",
  "copilot.requested": "已请求 Copilot",
  "copilot.accepted": "Copilot 已接收",
  "copilot.progress": "Copilot 正在整理证据",
  "copilot.completed": "Copilot 已生成草案",
  "copilot.failed": "Copilot 处理失败",
  "comment.created": "添加评论",
  "assignee.added": "添加负责人",
  "assignee.removed": "移除负责人",
  "status.changed": "变更状态",
  "labels.changed": "更新标签",
  "feedback.confirm": "确认检测",
  "feedback.false_positive": "标记误报",
  "feedback.category_correction": "修正类别",
  "feedback.disposition": "记录处置结果"
};

import { positiveParam, StaticAPIPage } from "@/components/static-api-page";
type Model = IssueDetail;
export default function DetailPage() {
 return <StaticAPIPage<Model> endpoint={(query)=>{const pid=positiveParam(query);const id=positiveParam(query,"issueId");return pid&&id?`/api/projects/${pid}/issues/${id}`:null;}}>
 {(model,query,reload)=>{const projectId=positiveParam(query)!;
  const issue = model.issue;
  const summary = issueEvidenceSummary({ detections: model.detections, assets: model.assets });
  const labels = Array.isArray(issue.labels) ? issue.labels : [];
  const taskRunIds = [...new Set([issue.taskRunId, ...model.links.filter(link => link.linkType === "task_run").map(link => link.targetId)].map(Number).filter(id => Number.isSafeInteger(id) && id > 0))];
  const inspectionLinks = model.links.flatMap(link => {
    const target = encodeURIComponent(String(link.targetId));
    const routes: Record<string, [string, string]> = {
      inspection_observation: ["观察范围与原图", canonicalPageHref(`/projects/inspection/observation/?projectId=${projectId}&observationId=${target}`)],
      inspection_evidence_set: ["巡检检测证据", canonicalPageHref(`/projects/inspection/evidence/?projectId=${projectId}&evidenceSetId=${target}`)],
      inspection_assessment: ["模型研判与复核", canonicalPageHref(`/projects/inspection/assessment/?projectId=${projectId}&assessmentId=${target}`)],
      algorithm_run: ["算法运行与模型版本", canonicalPageHref(`/projects/algorithms/runs/detail/?projectId=${projectId}&runId=${target}`)]
    };
    const route = routes[String(link.linkType)];
    return route ? [{ label: route[0], href: route[1] }] : [];
  });
  const collaboration = {onChanged: reload, agents: model.agents, assignees: model.assignees, canAssign: model.canAssign, canHandle: model.canHandle, canUseAgent: model.canUseAgent,
    issueId: Number(issue.id), labels: labels.map(String), members: model.members, projectId, stateVersion: Number(issue.stateVersion), status: String(issue.status)};
  const closed = issue.status === "closed";
  const taskSource = issue.sourceType === "task" || Boolean(issue.taskRunId);
  return <Page title={String(issue.title)} actions={<span className="text-2xl font-light text-muted-foreground">#{String(issue.number)}</span>}>
    <div className="-mt-3 flex flex-wrap items-center gap-3 border-b pb-5 text-sm">
      <span className={`inline-flex items-center gap-1.5 rounded-full px-3 py-1 font-medium ${closed ? "bg-purple-100 text-purple-800 dark:bg-purple-950 dark:text-purple-300" : "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300"}`}>{closed ? <CircleCheckIcon className="size-4" /> : <CircleDotIcon className="size-4" />}{closed ? "已关闭" : "开放中"}</span>
      <span className="text-muted-foreground">创建于 {displayDate(issue.createdAt)} · {model.events.filter(event => event.eventType === "comment.created").length} 条评论</span>
      <Link className="ml-auto inline-flex items-center gap-1.5 text-muted-foreground hover:text-foreground" href={canonicalPageHref(`/projects/issues/?projectId=${projectId}`)}><ArrowLeftIcon className="size-3.5" />全部案件</Link>
    </div>
    <div className="grid items-start gap-8 lg:grid-cols-[minmax(0,1fr)_280px]">
      <div className="min-w-0 space-y-7">
        <article className="overflow-hidden rounded-md border">
          <header className="flex items-center justify-between gap-3 border-b bg-muted/40 px-4 py-3 text-sm"><h2 className="font-medium">案件说明</h2><span className="text-xs text-muted-foreground">创建于 {displayDate(issue.createdAt)}</span></header>
          <div className="min-h-28 whitespace-pre-wrap px-5 py-5 text-sm leading-7">{String(issue.description || "暂无补充说明")}</div>
        </article>
        <section aria-label="活动时间线"><h2 className="mb-5 flex items-center gap-2 text-sm font-semibold"><GitBranchIcon className="size-4" />活动时间线</h2>
          {model.events.length ? <ol className="ml-4 border-l pl-7">{model.events.map(event => {
            const comment = event.eventType === "comment.created";
            const metadata = (event.metadata ?? {}) as Record<string, unknown>;
            const taskRunId = Number(metadata.taskRunId);
            return <li className="relative pb-6 last:pb-1" key={String(event.id)}>
              <span className="absolute -left-[39px] top-1 flex size-6 items-center justify-center rounded-full border bg-background text-muted-foreground">{comment ? <MessageSquareIcon className="size-3" /> : <GitBranchIcon className="size-3" />}</span>
              {comment ? <article className="overflow-hidden rounded-md border"><header className="flex flex-wrap items-center gap-2 border-b bg-muted/40 px-4 py-2.5 text-xs"><strong className="text-foreground">{String(event.actorName || "系统")}</strong><span className="text-muted-foreground">评论于 {displayDate(event.createdAt)}</span></header><p className="whitespace-pre-wrap break-words px-4 py-4 text-sm leading-7">{String(event.body || "—")}</p></article>
                : <div className="py-1 text-sm"><span className="font-medium">{String(event.actorName || "系统")}</span> <span>{activityLabels[String(event.eventType)] ?? String(event.eventType)}</span>{Number.isSafeInteger(taskRunId) && taskRunId > 0 && <Link className="ml-2 text-primary hover:underline" href={canonicalPageHref(`/projects/tasks/runs/detail/?projectId=${projectId}&runId=${taskRunId}`)}>来自任务运行 #{taskRunId}</Link>}<time className="ml-2 text-xs text-muted-foreground">{displayDate(event.createdAt)}</time>{Boolean(event.body) && <p className="mt-2 whitespace-pre-wrap text-muted-foreground">{String(event.body)}</p>}</div>}
            </li>;
          })}</ol> : <p className="py-4 text-sm text-muted-foreground">暂无活动记录。</p>}
        </section>
        {model.drafts.length > 0 && <section className="space-y-4 border-t pt-5"><h2 className="font-semibold">Copilot 草案</h2>{model.drafts.map(draft => {
          const payload = (draft.payload ?? {}) as Record<string, unknown>;
          return <article className="space-y-2 border-l-2 border-primary/30 pl-4" key={String(draft.id)}><div className="flex flex-wrap items-center gap-2"><h3 className="text-sm font-medium">{String(draft.title)}</h3><Badge variant="outline">待人工确认</Badge></div><p className="whitespace-pre-wrap text-sm leading-7">{String(payload.analysis ?? "草案内容不可用")}</p><p className="text-xs text-muted-foreground">模型 {String(draft.modelId)} · 提示模板 {String(draft.promptTemplateVersion)} · 证据快照 {String(draft.evidenceVersionHash).slice(0,12)}</p></article>;
        })}</section>}
        <section className="border-t pt-6"><IssueCollaborationPanel {...collaboration} section="conversation" /></section>
        <section id="issue-evidence" className="scroll-mt-6 space-y-4 border-t pt-5">
          <details className="group" open={summary.hasEvidence}>
            <summary className="cursor-pointer text-sm font-semibold">检测与模型证据 <span className="ml-1 font-normal text-muted-foreground">{summary.detectionCount} 条检测 · {summary.assetCount} 个媒体</span></summary>
            <div className="mt-5 space-y-6">{model.detections.length ? model.detections.map(detection => <article className="grid gap-4 border-b pb-5 last:border-0 md:grid-cols-2" key={String(detection.id)}>
              <EvidenceImage assetId={Number(detection.inputAssetId)} projectId={projectId} />
              <div className="min-w-0 space-y-2 text-sm"><div className="flex flex-wrap gap-2"><Badge>{String(detection.label)}</Badge><Badge variant="outline">置信度 {Number(detection.confidence).toFixed(2)}</Badge></div>
                <p>算法：{String(detection.algorithmName)} · {String(detection.modelOrProcess)} · 配置 v{String(detection.algorithmVersion)}</p>
                <p>位置质量：{String(detection.locationQuality)} · 投影 {String(detection.projectionMethod)} · mapping {String(detection.mappingVersion ?? "未提供")}</p>
                <p className="break-all text-xs text-muted-foreground">原始资产：#{String(detection.inputAssetId)} v{String(detection.assetVersion)} · 校验 {String(detection.assetChecksumSha256 || "未记录")}</p>
                <pre className="overflow-auto rounded bg-muted p-3 text-xs">像素标注 {JSON.stringify(detection.pixelGeometry, null, 2)}</pre>
              </div></article>) : <p className="text-sm text-muted-foreground">此案件尚未关联检测记录。</p>}
              {model.assets.filter(asset => !model.detections.some(detection => Number(detection.inputAssetId) === Number(asset.id))).map(asset => <article className="space-y-2 border-t pt-4" key={String(asset.id)}><p className="text-sm font-medium">关联媒体 #{String(asset.id)} · v{String(asset.version)}</p>{String(asset.mimeType ?? "").startsWith("image/") ? <EvidenceImage assetId={Number(asset.id)} projectId={projectId} /> : <Link className="text-sm text-primary hover:underline" href={canonicalPageHref(`/projects/assets/?projectId=${projectId}`)}>在数据资产中查看</Link>}</article>)}
            </div>
          </details>
          <details className="border-t pt-4"><summary className="cursor-pointer text-sm font-semibold">人工反馈与质量统计 <span className="ml-1 font-normal text-muted-foreground">{model.feedback.length} 条反馈</span></summary><div className="mt-4 space-y-4">
            {model.canHandle && model.detections.length > 0 ? <IssueFeedbackPanel onChanged={reload} detections={model.detections} issueId={Number(issue.id)} projectId={projectId} stateVersion={Number(issue.stateVersion)} /> : <p className="text-sm text-muted-foreground">无可反馈检测，或当前账号没有案件处置权限。</p>}
            {model.feedback.map(item => <div className="border-b pb-3 text-sm" key={String(item.id)}><strong>{String(item.action)}</strong> · 检测 #{String(item.detectionId)} · 模型版本 #{String(item.algorithmDefinitionVersionId)} · 任务版本 #{String(item.taskVersionId ?? "—")}<p className="text-muted-foreground">{String(item.reason)}{item.correctedLabel ? ` · 修正为 ${String(item.correctedLabel)}` : ""}{item.disposition ? ` · ${String(item.disposition)}` : ""}</p></div>)}
            {model.qualityStats.map(item => <p className="text-xs leading-6 text-muted-foreground" key={`${String(item.algorithmDefinitionVersionId)}:${String(item.taskVersionId)}`}>模型版本 #{String(item.algorithmDefinitionVersionId)} · 任务版本 #{String(item.taskVersionId ?? "—")} · 样本 {String(item.total)} · 确认 {String(item.confirmed)} · 误报 {String(item.falsePositives)} · 类别修正 {String(item.corrections)} · 误报率 {String(item.falsePositiveRate ?? "—")}</p>)}
          </div></details>
        </section>
      </div>
      <aside className="space-y-5 text-sm lg:border-l lg:pl-6" aria-label="案件属性">
        <IssueCollaborationPanel {...collaboration} section="properties" />
        <section className="space-y-2 border-t pt-5"><h2 className="font-medium">优先级</h2><Badge variant="outline">{issuePriorityLabel(String(issue.priority))}</Badge></section>
        <section className="space-y-2 border-t pt-5"><h2 className="font-medium">关联任务</h2>{taskRunIds.length ? taskRunIds.map(id => <Link key={id} className="block text-primary hover:underline" href={canonicalPageHref(`/projects/tasks/runs/detail/?projectId=${projectId}&runId=${id}`)}>{id === Number(issue.taskRunId) ? String(issue.taskName || "任务") : "任务"} · Run #{id}</Link>) : <p className="text-muted-foreground">暂无关联任务</p>}{Boolean(issue.taskVersionId) && <p className="text-xs text-muted-foreground">任务版本 v{String(issue.taskVersion || "—")} · 快照 #{String(issue.taskVersionId)} · 条件范围 {String(issue.conditionScopeKey || "—")}</p>}</section>
        <section className="space-y-2 border-t pt-5"><h2 className="font-medium">案件来源</h2><p>{taskSource ? "任务创建" : issue.sourceType === "manual" ? "手动创建" : "其他来源"}</p></section>
        <section className="space-y-2 border-t pt-5"><h2 className="font-medium">位置</h2><p className="text-muted-foreground">{summary.hasMapLocation ? "已关联地理位置" : "暂无位置信息"}</p>{summary.hasMapLocation && <p className="text-xs text-muted-foreground">{summary.locationLabel}</p>}</section>
        <section className="space-y-3 border-t pt-5"><h2 className="flex items-center gap-2 font-medium"><PaperclipIcon className="size-3.5" />关联证据</h2><p className="text-muted-foreground">{summary.evidenceLabel}</p>{summary.hasEvidence && <a className="block text-primary hover:underline" href="#issue-evidence">查看检测与媒体</a>}{inspectionLinks.map(link => <Link className="block text-primary hover:underline" key={link.href} href={link.href}>{link.label}</Link>)}</section>
      </aside>
    </div>
  </Page>;
 }}</StaticAPIPage>;
}
