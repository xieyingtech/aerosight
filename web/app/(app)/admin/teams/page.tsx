import { apiFetch } from "@/lib/api";
import { DataTable, Page } from "@/components/ui";

export default async function AdminTeamsPage() {
  const items = await apiFetch<Record<string, unknown>[]>("/api/admin/teams");
  return <Page title="团队管理"><DataTable columns={[{ key: "name", label: "名称" }, { key: "ownerName", label: "Owner" }, { key: "memberCount", label: "成员数" }]} items={items} /></Page>;
}
