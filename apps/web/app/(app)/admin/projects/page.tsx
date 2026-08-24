import { listAdminProjects } from "@/lib/data";
import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";

export default async function AdminProjectsPage() {
  const items = await listAdminProjects();
  return <Page title="项目管理"><DataTable columns={[{ key: "name", label: "名称" }, { key: "teamName", label: "团队" }, { key: "createdByUserName", label: "创建人" }]} items={items} /></Page>;
}
