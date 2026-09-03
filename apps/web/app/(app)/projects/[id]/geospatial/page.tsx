import Link from "next/link";

import { FlightHubGeospatialMap } from "@/components/dji-flighthub-geospatial-map";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
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

function ResourceGroup({ title, count, children }: { title: string; count: number; children: React.ReactNode }) {
  return <section className="rounded-xl border bg-card p-4">
    <div className="mb-3 flex items-center justify-between gap-3"><h2 className="font-medium">{title}</h2><Badge variant="outline">{count}</Badge></div>
    {children}
  </section>;
}

export default async function GeospatialPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const projectId = Number(id);
  const workspace = await readFlightHubGeospatial(projectId);
  return <Page title="地图与空间资源" description="查看连接器同步的标注、飞行区、离线地图与空间告警" variant="workspace">
    <div className="grid min-h-[600px] flex-1 gap-3 xl:min-h-0 xl:grid-cols-[minmax(0,1fr)_360px]">
      <FlightHubGeospatialMap className="h-[65svh] min-h-[520px] xl:h-full xl:min-h-0" workspace={workspace} />
      <aside className="min-h-0 space-y-3 xl:overflow-y-auto xl:pr-1">
        <section className="rounded-xl border bg-card p-4">
          <div className="flex items-center justify-between gap-3"><div><h2 className="font-medium">空间数据</h2><p className="mt-1 text-xs text-muted-foreground">来源：{workspace.source}</p></div><Badge variant="outline">{workspace.connectors.length} 个连接</Badge></div>
          <div className="mt-3 space-y-2 text-xs text-muted-foreground">{workspace.connectors.length ? workspace.connectors.map((connector) => {
            const sync = workspace.syncStates.find((item) => item.connectorId === connector.id);
            return <div className="rounded-lg bg-muted/40 p-3" key={connector.id}><div className="flex items-center justify-between gap-2"><span className="font-medium text-foreground">{connector.name}</span><Badge variant="outline">{connector.status}</Badge></div><p className="mt-1">同步 {sync?.status ?? "unknown"} · {displayDate(sync?.lastSucceededAt ?? null)}</p></div>;
          }) : <p>暂无提供空间数据的连接器。</p>}</div>
        </section>
        <ResourceGroup count={workspace.mapElements.length} title="地图标注">
          <div className="space-y-2">{workspace.mapElements.slice(0, 8).map((item) => <div className="rounded-lg bg-muted/35 p-3 text-xs" key={item.id}><div className="flex items-center justify-between gap-2"><span className="font-medium">{item.name}</span>{freshnessBadge(item.freshness)}</div><p className="mt-1 text-muted-foreground">{item.geometryType ?? "未知几何"} · {item.coordinateReference ?? "坐标待确认"}</p></div>)}{!workspace.mapElements.length && <p className="text-xs text-muted-foreground">暂无地图标注。</p>}</div>
        </ResourceGroup>
        <ResourceGroup count={workspace.flightAreas.length} title="飞行区">
          <div className="space-y-2">{workspace.flightAreas.slice(0, 8).map((item) => <div className="rounded-lg bg-muted/35 p-3 text-xs" key={item.id}><div className="flex items-center justify-between gap-2"><span className="font-medium">{item.name}</span>{freshnessBadge(item.freshness)}</div><p className="mt-1 text-muted-foreground">{item.areaType ?? "未分类"} · {displayDate(item.remoteUpdatedAt)}</p></div>)}{!workspace.flightAreas.length && <p className="text-xs text-muted-foreground">暂无飞行区。</p>}</div>
        </ResourceGroup>
        <ResourceGroup count={workspace.airSenseWarnings.length} title="空间告警">
          <div className="space-y-2">{workspace.airSenseWarnings.slice(0, 8).map((item) => <div className="rounded-lg bg-muted/35 p-3 text-xs" key={item.id}><div className="flex items-center justify-between gap-2">{item.issueId ? <Link className="font-medium text-primary hover:underline" href={`/projects/${projectId}/issues/${item.issueId}`}>{item.name}</Link> : <span className="font-medium">{item.name}</span>}{freshnessBadge(item.freshness)}</div><p className="mt-1 text-muted-foreground">级别 {item.warningLevel ?? "—"} · {displayDate(item.remoteUpdatedAt)}</p></div>)}{!workspace.airSenseWarnings.length && <p className="text-xs text-muted-foreground">暂无活动空间告警。</p>}</div>
        </ResourceGroup>
        <ResourceGroup count={workspace.offlineMaps.length} title="离线地图">
          <div className="space-y-2">{workspace.offlineMaps.slice(0, 8).map((item) => <div className="rounded-lg bg-muted/35 p-3 text-xs" key={item.id}><div className="flex items-center justify-between gap-2"><span className="font-medium">{item.name}</span>{freshnessBadge(item.freshness)}</div><p className="mt-1 text-muted-foreground">{item.progress === null ? "等待进度" : `${item.progress}%`} · {item.modelCount} 个资源</p></div>)}{!workspace.offlineMaps.length && <p className="text-xs text-muted-foreground">暂无离线地图。</p>}</div>
        </ResourceGroup>
      </aside>
    </div>
  </Page>;
}
