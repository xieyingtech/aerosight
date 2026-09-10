"use client";

import { useAPI } from "@/lib/use-api";
import { APIStateView } from "@/components/api-state";
import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";

export default function AdminProjectsPage() {
  const state = useAPI<Record<string, unknown>[]>("/api/admin/projects");
  return <APIStateView state={state}>{(items) => <Page title="项目管理"><DataTable columns={[{ key: "name", label: "名称" }, { key: "teamName", label: "团队" }, { key: "createdByName", label: "创建人" }]} items={items} /></Page>}</APIStateView>;
}
