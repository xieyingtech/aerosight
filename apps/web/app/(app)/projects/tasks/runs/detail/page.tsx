"use client";
import type { ComponentProps } from "react";
import { MissionRunWorkbench } from "@/components/mission-run-workbench";
import { Page } from "@/components/page";
import { positiveParam, StaticAPIPage } from "@/components/static-api-page";
type Model = ComponentProps<typeof MissionRunWorkbench>["model"];
export default function MissionRunPage() {
 return <StaticAPIPage<Model> endpoint={(query)=>{const pid=positiveParam(query);const run=positiveParam(query,"runId");return pid&&run?`/api/projects/${pid}/task-runs/${run}`:null;}}>
 {(model,query,reload)=><Page title="任务运行工作台" description="预检、设备、步骤和命令确认使用同一运行快照"><MissionRunWorkbench model={model} projectId={positiveParam(query)!} onChanged={reload}/></Page>}
 </StaticAPIPage>;
}
