import { listProjectItems } from "@/lib/data";
import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";

export default async function TasksPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const items = await listProjectItems(Number(id), "tasks");
  return <Page title="任务编排"><DataTable columns={[{ key: "name", label: "名称" }, { key: "triggerType", label: "触发类型" }, { key: "status", label: "状态" }]} items={items} /></Page>;
}
