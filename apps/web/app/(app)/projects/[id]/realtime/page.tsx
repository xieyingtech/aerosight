import { RadioTowerIcon } from "lucide-react";
import { Page } from "@/components/page";
import { ProjectMap } from "@/components/project-map";
import { getProject, requireUser } from "@/lib/data";
import { readProjectSituationSnapshot } from "@/lib/project-snapshot";
import { ProjectTimeline } from "@/components/project-timeline";

export default async function RealtimeOperationsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const projectId = Number(id);
  const [project, user] = await Promise.all([getProject(projectId), requireUser()]);
  const snapshot = await readProjectSituationSnapshot(user.id, projectId);
  if (!snapshot) throw new Error("PROJECT_NOT_FOUND");
  return (
    <Page description={`${project.name} 的在线设备、直播与任务控制`} title="实时作业">
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
        <ProjectMap className="h-[620px]" snapshot={snapshot} />
        <div className="flex min-h-72 flex-col items-center justify-center rounded-lg border border-dashed bg-muted/20 p-8 text-center">
          <RadioTowerIcon className="mb-3 size-8 text-muted-foreground" />
          <p className="font-medium">暂无活动作业</p>
          <p className="mt-1 text-sm text-muted-foreground">设备上线或任务启动后，将在这里显示直播与控制状态。</p>
        </div>
      </div>
      <ProjectTimeline snapshot={snapshot} />
    </Page>
  );
}
