import { getProject, requireUser } from "@/lib/data";
import { Page } from "@/components/page";
import { readProjectSituationSnapshot } from "@/lib/project-snapshot";
import { OverviewMap } from "@/components/overview-map";

export default async function ProjectOverview({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const projectId = Number(id);
  const [project, user] = await Promise.all([getProject(projectId), requireUser()]);
  const snapshot = await readProjectSituationSnapshot(user.id, projectId);
  if (!snapshot) throw new Error("PROJECT_NOT_FOUND");
  return (
    <Page description={project.description ?? "统一查看空地设备、任务、媒体与案件的实时态势"} title={project.name} variant="workspace">
      <OverviewMap snapshot={snapshot} />
    </Page>
  );
}
