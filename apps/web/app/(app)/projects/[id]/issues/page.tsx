import Link from "next/link";
import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
import { listIssues } from "@/lib/issues";

function displayDate(value: string | Date) {
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

export default async function IssuesPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const projectId = Number(id);
  const items = await listIssues(projectId);
  return <Page description="任务按条件创建或合并案件；每个案件保留任务、算法、检测和媒体证据链。" title="案件">
    <DataTable columns={[
      { key: "number", label: "编号", render: (item) => <Link className="font-medium hover:underline" href={`/projects/${projectId}/issues/${item.id}`}>#{item.number}</Link> },
      { key: "title", label: "标题", render: (item) => <div><Link className="font-medium hover:underline" href={`/projects/${projectId}/issues/${item.id}`}>{item.title}</Link><p className="mt-1 text-xs text-muted-foreground">{item.hasMapLocation ? "含地图位置" : "仅影像级"} · {item.occurrenceCount} 次出现</p></div> },
      { key: "status", label: "状态", render: (item) => <Badge variant="outline">{item.status === "open" ? "开放" : item.status}</Badge> },
      { key: "priority", label: "优先级", render: (item) => <Badge>{item.priority}</Badge> },
      { key: "lastSeenAt", label: "最近发现", render: (item) => displayDate(item.lastSeenAt) }
    ]} items={items} />
  </Page>;
}
