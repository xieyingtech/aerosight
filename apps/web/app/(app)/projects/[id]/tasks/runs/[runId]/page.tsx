import { MissionRunWorkbench } from "@/components/mission-run-workbench";
import { Page } from "@/components/page";
import { readMissionWorkbench } from "@/lib/mission-workbench";

export default async function MissionRunPage({ params }: { params: Promise<{ id: string; runId: string }> }) {
  const { id, runId } = await params;
  const projectId = Number(id);
  const model = await readMissionWorkbench(projectId, Number(runId));
  return <Page title="任务运行工作台" description="预检、设备、步骤和命令确认使用同一运行快照">
    <MissionRunWorkbench model={model} projectId={projectId} />
  </Page>;
}
