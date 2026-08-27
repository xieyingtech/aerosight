import Link from "next/link";
import { listProjectItems } from "@/lib/data";
import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";
import { listMissionRuns } from "@/lib/mission-workbench";

export default async function TasksPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const projectId = Number(id);
  const [items, runs] = await Promise.all([listProjectItems(projectId, "tasks"), listMissionRuns(projectId)]);
  return <Page title="任务编排"><div className="space-y-6">
    <section className="space-y-2"><h2 className="font-medium">任务模板</h2><DataTable columns={[{ key: "name", label: "名称" }, { key: "triggerType", label: "触发类型" }, { key: "status", label: "状态" }]} items={items} /></section>
    <section className="space-y-2"><h2 className="font-medium">任务运行</h2><DataTable columns={[
      { key: "taskName", label: "任务", render: (run) => <Link className="font-medium text-primary hover:underline" href={`/projects/${projectId}/tasks/runs/${String(run.id)}`}>{String(run.taskName)}</Link> },
      { key: "deviceName", label: "设备" }, { key: "status", label: "状态" }, { key: "stateVersion", label: "状态版本" }
    ]} items={runs} /></section>
  </div></Page>;
}
