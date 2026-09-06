"use client";

import { useAPI } from "@/lib/use-api";
import { APIStateView } from "@/components/api-state";
import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";

export default function AdminTeamsPage() {
  const state = useAPI<Record<string, unknown>[]>("/api/admin/teams");
  return <APIStateView state={state}>{(items) => <Page title="团队管理"><DataTable columns={[{ key: "name", label: "名称" }, { key: "ownerName", label: "Owner" }, { key: "memberCount", label: "成员数" }]} items={items} /></Page>}</APIStateView>;
}
