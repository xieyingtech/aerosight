import { listProjectItems } from "@/lib/data";
import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";

export default async function AgentsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const items = await listProjectItems(Number(id), "agents");
  return <Page title="智能体"><DataTable columns={[{ key: "name", label: "名称" }, { key: "status", label: "状态" }, { key: "description", label: "描述" }]} items={items} /></Page>;
}
