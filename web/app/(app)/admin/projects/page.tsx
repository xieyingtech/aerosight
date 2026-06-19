import { apiFetch } from "@/lib/api";
import { DataTable, Page } from "@/components/ui";

export default async function AdminProjectsPage() {
  const items = await apiFetch<Record<string, unknown>[]>("/api/admin/projects");
  return <Page title="项目管理"><DataTable columns={[{ key: "name", label: "名称" }, { key: "teamName", label: "团队" }, { key: "createdByUserName", label: "创建人" }]} items={items} /></Page>;
}
