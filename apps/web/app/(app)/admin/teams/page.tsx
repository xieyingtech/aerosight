import { listAdminTeams } from "@/lib/data";
import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";

export default async function AdminTeamsPage() {
  const items = await listAdminTeams();
  return <Page title="团队管理"><DataTable columns={[{ key: "name", label: "名称" }, { key: "ownerName", label: "Owner" }, { key: "memberCount", label: "成员数" }]} items={items} /></Page>;
}
