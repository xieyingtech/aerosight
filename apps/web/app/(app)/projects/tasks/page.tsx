"use client";
import Link from "next/link";

import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";


import { positiveParam, StaticAPIPage } from "@/components/static-api-page";
import { APIStateView } from "@/components/api-state";
import { useAPI } from "@/lib/use-api";
export default function TasksPage() {
 return <StaticAPIPage<Record<string,unknown>[]> endpoint={(query)=>{const id=positiveParam(query);return id?`/api/projects/${id}/tasks`:null;}}>
 {(items,query)=><Tasks projectId={positiveParam(query)!} items={items}/>}
 </StaticAPIPage>;
}
function Tasks({items,projectId}:{items:Record<string,unknown>[];projectId:number}) {
 const runs=useAPI<Record<string,unknown>[]>(`/api/projects/${projectId}/task-runs`);
  return <Page title="任务编排"><div className="space-y-6">
    <section className="space-y-2"><h2 className="font-medium">任务模板</h2><DataTable columns={[{ key: "name", label: "名称" }, { key: "triggerType", label: "触发类型" }, { key: "status", label: "状态" }]} items={items} /></section>
    <APIStateView state={runs}>{(runRows)=><section className="space-y-2"><h2 className="font-medium">任务运行</h2><DataTable columns={[
      { key: "taskName", label: "任务", render: (run) => <Link className="font-medium text-primary hover:underline" href={`/projects/tasks/runs/detail/?projectId=${projectId}&runId=${String(run.id)}`}>{String(run.taskName)}</Link> },
      { key: "deviceName", label: "设备" }, { key: "status", label: "状态" }, { key: "stateVersion", label: "状态版本" }
    ]} items={runRows} /></section>}</APIStateView>
  </div></Page>;
}
