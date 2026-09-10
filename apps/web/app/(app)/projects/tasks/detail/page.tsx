"use client";
import type {ComponentProps} from "react";
import {Page} from "@/components/page";
import {TaskTemplateWorkbench} from "@/components/task-template-workbench";
import {StaticAPIPage,positiveParam} from "@/components/static-api-page";
type Model=ComponentProps<typeof TaskTemplateWorkbench>["model"]&{task:Record<string,unknown>};
export default function TaskTemplatePage(){return <StaticAPIPage<Model> endpoint={q=>{const pid=positiveParam(q),tid=positiveParam(q,"taskId");return pid&&tid?`/api/projects/${pid}/tasks/${tid}/workbench`:null;}}>{(model,q,reload)=><Page title={String(model.task.name)} description="版本化 Task 模板、触发器与类型化步骤配置"><TaskTemplateWorkbench key={`${positiveParam(q)}:${positiveParam(q,"taskId")}`} projectId={positiveParam(q)!} taskId={positiveParam(q,"taskId")!} model={model} onChanged={reload}/></Page>}</StaticAPIPage>;}
