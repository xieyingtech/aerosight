import { apiFetch } from "@/lib/api";
import { DataTable, Page } from "@/components/ui";

export default async function TasksPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const items = await apiFetch<Record<string, unknown>[]>(`/api/projects/${id}/tasks`);
  return <Page title="任务编排"><DataTable columns={[{ key: "name", label: "名称" }, { key: "triggerType", label: "触发类型" }, { key: "status", label: "状态" }]} items={items} /></Page>;
}
