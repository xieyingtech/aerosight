import { Page } from "@/components/page";
import { TaskTemplateWorkbench } from "@/components/task-template-workbench";
import { readTaskTemplateWorkbench } from "@/lib/task-template-workbench";

export default async function TaskTemplatePage({ params }: { params: Promise<{ id: string; taskId: string }> }) {
  const { id,taskId } = await params;
  const projectId = Number(id);
  const model = await readTaskTemplateWorkbench(projectId,Number(taskId));
  return <Page title={String(model.task.name)} description="版本化 Task 模板、触发器与类型化步骤配置">
    <TaskTemplateWorkbench projectId={projectId} taskId={Number(taskId)} model={model} />
  </Page>;
}
