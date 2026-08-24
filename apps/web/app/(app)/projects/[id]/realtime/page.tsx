import { Page } from "@/components/page";
import { getProject, requireUser } from "@/lib/data";
import { readProjectSituationSnapshot } from "@/lib/project-snapshot";
import { SituationExplorer } from "@/components/situation-explorer";

export default async function RealtimeOperationsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const projectId = Number(id);
  const [project, user] = await Promise.all([getProject(projectId), requireUser()]);
  const snapshot = await readProjectSituationSnapshot(user.id, projectId);
  if (!snapshot) throw new Error("PROJECT_NOT_FOUND");
  return (
    <Page description={`${project.name} 的在线设备、直播与任务控制`} title="实时作业">
      <SituationExplorer mapClassName="h-[620px]" snapshot={snapshot} />
    </Page>
  );
}
