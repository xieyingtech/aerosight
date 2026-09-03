import Link from "next/link";

import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { readFlightHubFlightOperations } from "@/lib/dji-flighthub-flight-operations";

function displayDate(value: string | null) {
  if (!value) return "—";
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

function displayBytes(value: number | null) {
  if (value === null) return "—";
  if (value < 1024) return `${value} B`;
  if (value < 1024 ** 2) return `${(value / 1024).toFixed(1)} KB`;
  return `${(value / 1024 ** 2).toFixed(1)} MB`;
}

function statusBadge(status: string | null) {
  if (!status) return <span>—</span>;
  const destructive = ["failed", "blocked", "critical"].includes(status);
  return <Badge variant={destructive ? "destructive" : "outline"}>{status}</Badge>;
}

function Section({ title, description, children }: { title: string; description: string; children: React.ReactNode }) {
  return <section className="space-y-2">
    <div><h2 className="font-medium">{title}</h2><p className="text-sm text-muted-foreground">{description}</p></div>
    {children}
  </section>;
}

export default async function FlightOperationsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const projectId = Number(id);
  const operations = await readFlightHubFlightOperations(projectId);
  return <Page
    title="航线与飞行任务"
    description="查看连接器同步的航线、飞行任务、轨迹、产物与告警。"
    actions={operations.access.canOperate ? <Button asChild variant="outline"><Link href={`/projects/${projectId}/tasks`}>进入受控任务操作</Link></Button> : undefined}
  >
    <div className="space-y-8">
      <Card size="sm">
        <CardHeader>
          <CardTitle>{operations.access.canOperate ? "操作者视图" : "只读视图"}</CardTitle>
          <CardDescription>{operations.access.canOperate
            ? "可查看连接器的现场验收与功能开关状态；所有写入仍需通过任务预检、审批和持久作业。"
            : "可查看全部飞行运营投影与最终结果，但不提供任何写操作入口。"}</CardDescription>
          <CardAction><Badge variant={operations.access.canOperate ? "default" : "secondary"}>{operations.access.mode}</Badge></CardAction>
        </CardHeader>
      </Card>

      <Section title="数据连接" description="操作可用性同时受成员权限、连接状态、功能开关和现场验收约束。">
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {operations.connectors.length ? operations.connectors.map((connector) => <Card key={connector.id} size="sm">
            <CardHeader><CardTitle>{connector.name}</CardTitle><CardDescription>本地连接器 #{connector.id}</CardDescription><CardAction>{statusBadge(connector.status)}</CardAction></CardHeader>
            <CardContent className="space-y-1 text-muted-foreground">
              <p>最近检查：{displayDate(connector.lastCheckedAt)}</p>
              <p>功能开关：{connector.actionEnabled ? "已开启" : "已关闭"} · 现场验收：{connector.actionVerified ? "已通过" : "未通过"}</p>
              {operations.access.canOperate ? <p className="text-foreground">{connector.actionReady ? "当前可提交受控任务操作" : "当前不可提交写操作"}</p> : null}
            </CardContent>
          </Card>) : <Card size="sm"><CardContent className="text-muted-foreground">暂无提供飞行目录的连接器</CardContent></Card>}
        </div>
      </Section>

      <Section title="航线" description="展示连接器同步的安全摘要与本地任务关联。">
        <DataTable items={operations.waylines} columns={[
          { key: "name", label: "名称", render: (item) => item.taskId ? <Link className="font-medium text-primary hover:underline" href={`/projects/${projectId}/tasks/${item.taskId}`}>{item.name}</Link> : item.name },
          { key: "status", label: "状态", render: (item) => statusBadge(item.status) },
          { key: "deviceModelKey", label: "设备模型" },
          { key: "templateTypes", label: "模板", render: (item) => item.templateTypes.join("、") || "—" },
          { key: "payloadCount", label: "负载数" },
          { key: "sizeBytes", label: "大小", render: (item) => displayBytes(item.sizeBytes) },
          { key: "remoteUpdatedAt", label: "上游更新时间", render: (item) => displayDate(item.remoteUpdatedAt) },
        ]} />
      </Section>

      <Section title="飞行任务运行" description="任务状态以本地 task_run 对账结果为准，HTTP 受理不等于飞行成功。">
        <DataTable items={operations.taskRuns} columns={[
          { key: "taskName", label: "任务", render: (item) => <Link className="font-medium text-primary hover:underline" href={`/projects/${projectId}/tasks/runs/${item.taskRunId}`}>{item.taskName}</Link> },
          { key: "deviceName", label: "设备" },
          { key: "status", label: "对账状态", render: (item) => statusBadge(item.status) },
          { key: "taskType", label: "类型" },
          { key: "currentWaypoint", label: "航点", render: (item) => item.currentWaypoint === null ? "—" : `${item.currentWaypoint}/${item.totalWaypoints ?? "?"}` },
          { key: "exceptionCount", label: "异常数" },
          { key: "remoteUpdatedAt", label: "最近对账", render: (item) => displayDate(item.remoteUpdatedAt) },
        ]} />
      </Section>

      <Section title="轨迹摘要" description="为保护位置隐私，此列表只返回任务轨迹点数与时间范围，不返回坐标。">
        <DataTable items={operations.tracks} columns={[
          { key: "taskName", label: "任务", render: (item) => <Link className="font-medium text-primary hover:underline" href={`/projects/${projectId}/tasks/runs/${item.taskRunId}`}>{item.taskName}</Link> },
          { key: "deviceName", label: "设备" }, { key: "pointCount", label: "轨迹点" },
          { key: "firstCapturedAt", label: "开始", render: (item) => displayDate(item.firstCapturedAt) },
          { key: "lastCapturedAt", label: "结束", render: (item) => displayDate(item.lastCapturedAt) },
        ]} />
      </Section>

      <Section title="飞行媒体" description="媒体通过本地资产授权访问；列表不包含对象键、下载 URL 或签名参数。">
        <DataTable items={operations.media} columns={[
          { key: "name", label: "名称" }, { key: "kind", label: "类型" },
          { key: "status", label: "状态", render: (item) => statusBadge(item.status) },
          { key: "sizeBytes", label: "大小", render: (item) => displayBytes(item.sizeBytes) },
          { key: "taskRunId", label: "任务运行", render: (item) => item.taskRunId ? <Link className="text-primary hover:underline" href={`/projects/${projectId}/tasks/runs/${item.taskRunId}`}>#{item.taskRunId}</Link> : "—" },
          { key: "capturedAt", label: "采集时间", render: (item) => displayDate(item.capturedAt) },
        ]} />
      </Section>

      <Section title="飞行记录" description="导出进度和失败码来自安全摘要，文件访问继续走项目资产授权。">
        <DataTable items={operations.flightRecords} columns={[
          { key: "name", label: "名称" }, { key: "status", label: "状态", render: (item) => statusBadge(item.status) },
          { key: "progress", label: "进度", render: (item) => item.progress === null ? "—" : `${item.progress}%` },
          { key: "fileTypes", label: "格式", render: (item) => item.fileTypes.join("、") || "—" },
          { key: "failedReasonCode", label: "失败码" },
          { key: "createdAt", label: "创建时间", render: (item) => displayDate(item.createdAt) },
        ]} />
      </Section>

      <Section title="飞行与 AI 告警" description="告警关联本地任务、感知事件和案件，不暴露上游标识。">
        <DataTable items={operations.alerts} columns={[
          { key: "title", label: "告警", render: (item) => item.issueId ? <Link className="font-medium text-primary hover:underline" href={`/projects/${projectId}/issues/${item.issueId}`}>{item.title}</Link> : item.title },
          { key: "kind", label: "类型" }, { key: "severity", label: "级别", render: (item) => statusBadge(item.severity) },
          { key: "status", label: "状态", render: (item) => statusBadge(item.status) },
          { key: "alertCount", label: "数量" }, { key: "occurredAt", label: "发生时间", render: (item) => displayDate(item.occurredAt) },
        ]} />
      </Section>

      <Section title="受控写操作结果" description="只展示本地作业 ID、尝试次数、安全错误码与远端对账后的最终状态。">
        <DataTable items={operations.actions} columns={[
          { key: "action", label: "操作" },
          { key: "taskRunId", label: "任务运行", render: (item) => <Link className="text-primary hover:underline" href={`/projects/${projectId}/tasks/runs/${item.taskRunId}`}>#{item.taskRunId}</Link> },
          { key: "status", label: "状态", render: (item) => statusBadge(item.status) },
          { key: "final", label: "最终结果", render: (item) => item.final ? "已收敛" : "等待对账" },
          { key: "attemptCount", label: "写入尝试" }, { key: "reconciliationCount", label: "对账次数" },
          { key: "lastErrorCode", label: "安全错误码" },
          { key: "updatedAt", label: "更新时间", render: (item) => displayDate(item.updatedAt) },
        ]} />
      </Section>
    </div>
  </Page>;
}
