import { apiFetch } from "@/lib/api";
import { DataTable, Page } from "@/components/ui";

export default async function AgentsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const items = await apiFetch<Record<string, unknown>[]>(`/api/projects/${id}/agents`);
  return <Page title="智能体"><DataTable columns={[{ key: "name", label: "名称" }, { key: "status", label: "状态" }, { key: "description", label: "描述" }]} items={items} /></Page>;
}
