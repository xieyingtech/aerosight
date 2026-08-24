import { listProjectItems } from "@/lib/data";
import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";

export default async function IssuesPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const items = await listProjectItems(Number(id), "issues");
  return <Page title="问题中心"><DataTable columns={[{ key: "number", label: "编号" }, { key: "title", label: "标题" }, { key: "status", label: "状态" }, { key: "priority", label: "优先级" }]} items={items} /></Page>;
}
