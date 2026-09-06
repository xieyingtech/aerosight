"use client";

import Link from "next/link";
import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
import { StaticAPIPage, positiveParam } from "@/components/static-api-page";

type Detail = { team: { name: string; role: string; memberCount: number }; projects: Record<string, unknown>[] };
const roleLabel = (role: string) => role === "owner" ? "所有者" : role === "admin" ? "管理员" : "成员";

export default function TeamDetailPage() {
  return <StaticAPIPage<Detail> endpoint={(query) => { const id = positiveParam(query, "teamId"); return id ? `/api/teams/${id}` : null; }}>{(detail) =>
    <Page description={`${roleLabel(detail.team.role)} · ${detail.team.memberCount} 名成员`} title={detail.team.name}>
      <DataTable columns={[
        { key: "name", label: "项目", render: (item) => <div><Link className="font-medium text-sky-700 hover:underline" href={`/projects/detail/?projectId=${String(item.id)}`}>{String(item.name)}</Link><p className="mt-1 text-xs text-muted-foreground">{String(item.description ?? "暂无描述")}</p></div> },
        { key: "updatedAt", label: "最近更新", render: (item) => new Date(String(item.updatedAt)).toLocaleDateString() }
      ]} items={detail.projects} />
      <div className="text-sm text-muted-foreground">我的角色 <Badge>{roleLabel(detail.team.role)}</Badge></div>
    </Page>
  }</StaticAPIPage>;
}
