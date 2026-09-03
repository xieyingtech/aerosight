import Link from "next/link";

import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { readFlightHubModels } from "@/lib/dji-flighthub-models";

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
function statusBadge(value: string | null) {
  if (!value) return <span>—</span>;
  return <Badge variant={new Set(["failed", "blocked", "missing", "deleted"]).has(value) ? "destructive" : "outline"}>{value}</Badge>;
}
function Section({ title, description, children }: { title: string; description: string; children: React.ReactNode }) {
  return <section className="space-y-2"><div><h2 className="font-medium">{title}</h2>
    <p className="text-sm text-muted-foreground">{description}</p></div>{children}</section>;
}
const actionLabels: Record<string, string> = {
  "traditional-create": "传统模型重建", "open-start": "开放模型启动", "open-stop": "开放模型停止",
  "open-upload": "开放模型上传", "model-delete": "删除模型", "model-resource-delete": "删除模型资源"
};

export default async function ModelsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const projectId = Number(id);
  const workspace = await readFlightHubModels(projectId);
  return <Page title="建模资源" description="查看连接器同步的模型目录、重建作业、产物与操作可用性。"
    actions={<Button asChild variant="outline"><Link href={`/projects/${projectId}/assets`}>查看模型资产</Link></Button>}>
    <div className="space-y-8">
      <Card size="sm"><CardHeader><CardTitle>{workspace.access.canOperate ? "操作者视图" : "只读视图"}</CardTitle>
        <CardDescription>{workspace.access.canOperate
          ? "操作入口仍受连接状态、默认关闭功能开关、field-write 现场验收和操作自身审批约束。"
          : "当前身份只能查看目录、进度、产物和失败原因，不提供模型写操作。"}</CardDescription>
        <CardAction><Badge variant={workspace.access.canOperate ? "default" : "secondary"}>{workspace.access.mode}</Badge></CardAction>
      </CardHeader></Card>

      <Section title="连接器与实际可用操作" description="“可用”是服务端权限、连接器状态、feature flag 与 capability evidence 的实时交集。">
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {workspace.connectors.length ? workspace.connectors.map((connector) => {
            const sync = workspace.syncStates.find((item) => item.connectorId === connector.id);
            return <Card key={connector.id} size="sm"><CardHeader><CardTitle>{connector.name}</CardTitle>
              <CardDescription>模型同步：{sync?.status ?? "unknown"}</CardDescription><CardAction>{statusBadge(connector.status)}</CardAction></CardHeader>
              <CardContent className="space-y-2 text-sm"><p className="text-muted-foreground">最近成功：{displayDate(sync?.lastSucceededAt ?? null)}</p>
                <div className="flex flex-wrap gap-1">{connector.actions.map((action) => <Badge key={action.action}
                  variant={action.available ? "default" : "secondary"}>{actionLabels[action.action] ?? action.action}：{action.available ? "可用" : "不可用"}</Badge>)}</div>
              </CardContent></Card>;
          }) : <Card size="sm"><CardContent className="text-muted-foreground">暂无提供建模能力的连接器。</CardContent></Card>}
        </div>
      </Section>

      <Section title="传统模型目录" description="展示同步的安全摘要与本地资产状态，不返回远端标识或临时下载地址。">
        <DataTable items={workspace.models} columns={[
          { key: "name", label: "名称" }, { key: "status", label: "目录状态", render: (item) => statusBadge(item.status) },
          { key: "fileType", label: "模型类型" }, { key: "showOnMap", label: "地图展示", render: (item) => item.showOnMap ? "是" : "否" },
          { key: "sizeBytes", label: "大小", render: (item) => displayBytes(item.sizeBytes) },
          { key: "assetStatus", label: "产物", render: (item) => statusBadge(item.assetStatus) },
          { key: "assetFailureCode", label: "失败原因" },
          { key: "lastSeenAt", label: "最近同步", render: (item) => displayDate(item.lastSeenAt) }
        ]} />
      </Section>

      <Section title="开放模型与资源" description="重建、压缩和资源导入进度来自最新投影；失败原因只展示稳定安全码。">
        <DataTable items={workspace.resources} columns={[
          { key: "assetKind", label: "产物类型" }, { key: "modelType", label: "模型类型" },
          { key: "modelStatus", label: "模型状态" },
          { key: "reconstructionProgress", label: "重建进度", render: (item) => item.reconstructionProgress === null ? "—" : `${item.reconstructionProgress}%` },
          { key: "zipProgress", label: "压缩进度", render: (item) => item.zipProgress === null ? "—" : `${item.zipProgress}%` },
          { key: "fileCount", label: "文件数" }, { key: "sizeBytes", label: "大小", render: (item) => displayBytes(item.sizeBytes) },
          { key: "assetStatus", label: "产物状态", render: (item) => statusBadge(item.assetStatus) },
          { key: "assetFailureCode", label: "失败原因" }
        ]} />
      </Section>

      <Section title="模型作业" description="统一展示重建、开放上传和删除作业；同步受理不等于完成，运行中作业保留真实阶段和对账次数。">
        <DataTable items={workspace.jobs} columns={[
          { key: "jobType", label: "作业类型" }, { key: "action", label: "操作", render: (item) => actionLabels[item.action] ?? item.action },
          { key: "status", label: "状态", render: (item) => statusBadge(item.status) },
          { key: "progress", label: "进度", render: (item) => item.progress === null ? "—" : `${item.progress}%` },
          { key: "stage", label: "阶段" }, { key: "attemptCount", label: "写入尝试" },
          { key: "reconciliationCount", label: "对账次数" }, { key: "lastErrorCode", label: "失败原因" },
          { key: "assetIds", label: "产物", render: (item) => item.assetIds.length ? `${item.assetIds.length} 项本地资产` : "—" },
          { key: "updatedAt", label: "更新时间", render: (item) => displayDate(item.updatedAt) }
        ]} />
      </Section>
    </div>
  </Page>;
}
