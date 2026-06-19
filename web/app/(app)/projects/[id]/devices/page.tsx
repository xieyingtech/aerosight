import { apiFetch } from "@/lib/api";
import { DataTable, Page } from "@/components/ui";

export default async function DevicesPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const items = await apiFetch<Record<string, unknown>[]>(`/api/projects/${id}/devices`);
  return <Page title="设备监控"><DataTable columns={[{ key: "name", label: "名称" }, { key: "type", label: "类型" }, { key: "status", label: "状态" }]} items={items} /></Page>;
}
