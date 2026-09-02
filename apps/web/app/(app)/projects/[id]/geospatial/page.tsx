import Link from "next/link";

import { DataTable } from "@/components/data-table";
import { FlightHubGeospatialMap } from "@/components/dji-flighthub-geospatial-map";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { readFlightHubGeospatial } from "@/lib/dji-flighthub-geospatial";

function displayDate(value: string | null) {
  if (!value) return "—";
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

function freshnessBadge(value: string) {
  const variant = value === "fresh" ? "default" : value === "missing" ? "destructive" : "outline";
  const label = { fresh: "新鲜", stale: "已过期", missing: "远端缺失", unknown: "未知" }[value] ?? value;
  return <Badge variant={variant}>{label}</Badge>;
}

function Section({ title, description, children }: { title: string; description: string; children: React.ReactNode }) {
  return <section className="space-y-2">
    <div><h2 className="font-medium">{title}</h2><p className="text-sm text-muted-foreground">{description}</p></div>
    {children}
  </section>;
}

export default async function GeospatialPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const projectId = Number(id);
  const workspace = await readFlightHubGeospatial(projectId);
  return <Page title="地图空域" description="司空标注、飞行区、离线地图与 AirSense 的项目级只读投影。">
    <div className="space-y-8">
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {workspace.connectors.length ? workspace.connectors.map((connector) => {
          const sync = workspace.syncStates.find((item) => item.connectorId === connector.id);
          return <Card key={connector.id} size="sm">
            <CardHeader><CardTitle>{connector.name}</CardTitle><CardDescription>来源：{workspace.source}</CardDescription><CardAction><Badge variant="outline">{connector.status}</Badge></CardAction></CardHeader>
            <CardContent className="space-y-1 text-sm text-muted-foreground">
              <p>目录同步：{sync?.status ?? "unknown"} · 最近成功：{displayDate(sync?.lastSucceededAt ?? null)}</p>
              <p>最近检查：{displayDate(connector.lastCheckedAt)}{sync?.lastErrorCode ? ` · ${sync.lastErrorCode}` : ""}</p>
            </CardContent>
          </Card>;
        }) : <Card size="sm"><CardContent className="text-muted-foreground">暂无司空连接器</CardContent></Card>}
      </div>

      <Section title="空域地图" description="紫色为司空标注，青色为飞行区，红色为新鲜 AirSense 目标；未校准坐标不用于飞行控制预检。">
        <FlightHubGeospatialMap workspace={workspace} />
      </Section>

      <Section title="司空标注" description="点、线、面来自连接器投影；版本以不可逆指纹展示，不暴露远端标识。">
        <DataTable items={workspace.mapElements} columns={[
          { key: "name", label: "名称" }, { key: "geometryType", label: "几何" },
          { key: "source", label: "来源" }, { key: "versionFingerprint", label: "版本" },
          { key: "freshness", label: "新鲜度", render: (item) => freshnessBadge(item.freshness) },
          { key: "coordinateReference", label: "坐标基准" },
          { key: "lastSeenAt", label: "最近发现", render: (item) => displayDate(item.lastSeenAt) },
        ]} />
      </Section>

      <Section title="飞行区" description="展示司空飞行区边界、类型、版本和坐标验收状态。">
        <DataTable items={workspace.flightAreas} columns={[
          { key: "name", label: "名称" }, { key: "areaType", label: "类型" }, { key: "geometryType", label: "几何" },
          { key: "source", label: "来源" }, { key: "versionFingerprint", label: "版本" },
          { key: "freshness", label: "新鲜度", render: (item) => freshnessBadge(item.freshness) },
          { key: "remoteUpdatedAt", label: "上游更新", render: (item) => displayDate(item.remoteUpdatedAt) },
        ]} />
      </Section>

      <Section title="离线地图" description="只展示目录和生成进度；下载仍通过项目授权的短期访问接口。">
        <DataTable items={workspace.offlineMaps} columns={[
          { key: "name", label: "名称" },
          { key: "modelNames", label: "模型", render: (item) => item.modelNames.join("、") || `${item.modelCount} 个模型` },
          { key: "progress", label: "进度", render: (item) => item.progress === null ? "—" : `${item.progress}%` },
          { key: "stateCode", label: "状态" }, { key: "source", label: "来源" },
          { key: "versionFingerprint", label: "版本" },
          { key: "freshness", label: "新鲜度", render: (item) => freshnessBadge(item.freshness) },
          { key: "remoteUpdatedAt", label: "上游更新", render: (item) => displayDate(item.remoteUpdatedAt) },
        ]} />
      </Section>

      <Section title="AirSense" description="活动告警显示为实时空域目标；过期或 missing 项保留历史但不作为实时威胁。">
        <DataTable items={workspace.airSenseWarnings} columns={[
          { key: "name", label: "目标", render: (item) => item.issueId
            ? <Link className="font-medium text-primary hover:underline" href={`/projects/${projectId}/issues/${item.issueId}`}>{item.name}</Link> : item.name },
          { key: "warningLevel", label: "级别" }, { key: "source", label: "来源" },
          { key: "versionFingerprint", label: "版本" },
          { key: "freshness", label: "新鲜度", render: (item) => freshnessBadge(item.freshness) },
          { key: "remoteUpdatedAt", label: "数据时间", render: (item) => displayDate(item.remoteUpdatedAt) },
          { key: "expiresAt", label: "过期时间", render: (item) => displayDate(item.expiresAt) },
        ]} />
      </Section>
    </div>
  </Page>;
}
