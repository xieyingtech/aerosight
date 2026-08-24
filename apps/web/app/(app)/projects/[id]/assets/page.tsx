import { listProjectItems } from "@/lib/data";
import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";

export default async function AssetsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const items = await listProjectItems(Number(id), "assets");
  return <Page title="素材库"><DataTable columns={[{ key: "kind", label: "类型" }, { key: "mimeType", label: "MIME" }, { key: "createdAt", label: "创建时间" }]} items={items} /></Page>;
}
