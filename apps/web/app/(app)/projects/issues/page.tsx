"use client";
import Link from "next/link";
import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
import type { IssueListItem } from "@/lib/issues";

function displayDate(value: string | Date) {
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

import { positiveParam, StaticAPIPage } from "@/components/static-api-page";
export default function WorkspacePage() {
  return <StaticAPIPage<IssueListItem[]> endpoint={(query) => {const id=positiveParam(query); return id ? `/api/projects/${id}/issues` : null;}}>
    {(items, query) => { const projectId=positiveParam(query)!;
  return <Page description="任务按条件创建或合并案件；每个案件保留任务、算法、检测和媒体证据链。" title="案件">
    <DataTable columns={[
      { key: "number", label: "编号", render: (item) => <Link className="font-medium hover:underline" href={`/projects/issues/detail/?projectId=${projectId}&issueId=${item.id}`}>#{item.number}</Link> },
      { key: "title", label: "标题", render: (item) => <div><Link className="font-medium hover:underline" href={`/projects/issues/detail/?projectId=${projectId}&issueId=${item.id}`}>{item.title}</Link><p className="mt-1 text-xs text-muted-foreground">{item.hasMapLocation ? "含地图位置" : "仅影像级"} · {item.occurrenceCount} 次出现</p></div> },
      { key: "status", label: "状态", render: (item) => <Badge variant="outline">{item.status === "open" ? "开放" : item.status}</Badge> },
      { key: "priority", label: "优先级", render: (item) => <Badge>{item.priority}</Badge> },
      { key: "lastSeenAt", label: "最近发现", render: (item) => displayDate(item.lastSeenAt) }
    ]} items={items} />
  </Page>;
    }}</StaticAPIPage>;
}
