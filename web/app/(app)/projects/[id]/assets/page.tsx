import { apiFetch } from "@/lib/api";
import { DataTable, Page } from "@/components/ui";

export default async function AssetsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const items = await apiFetch<Record<string, unknown>[]>(`/api/projects/${id}/assets`);
  return <Page title="素材库"><DataTable columns={[{ key: "kind", label: "类型" }, { key: "mimeType", label: "MIME" }, { key: "createdAt", label: "创建时间" }]} items={items} /></Page>;
}
