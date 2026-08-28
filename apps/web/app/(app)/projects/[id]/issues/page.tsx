import { listProjectItems } from "@/lib/data";
import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";

export default async function IssuesPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const items = await listProjectItems(Number(id), "issues");
  return <Page description="由任务自动创建或由项目成员手动记录，可评论并指派给成员或 Copilot。" title="案件"><DataTable columns={[{ key: "number", label: "编号" }, { key: "title", label: "标题" }, { key: "status", label: "状态" }, { key: "priority", label: "优先级" }]} items={items} /></Page>;
}
