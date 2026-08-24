import { getProject, requireUser } from "@/lib/data";
import { Page } from "@/components/page";
import { ProjectMap } from "@/components/project-map";
import { readProjectSituationSnapshot } from "@/lib/project-snapshot";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export default async function ProjectOverview({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const projectId = Number(id);
  const [project, user] = await Promise.all([getProject(projectId), requireUser()]);
  const snapshot = await readProjectSituationSnapshot(user.id, projectId);
  if (!snapshot) throw new Error("PROJECT_NOT_FOUND");
  return (
    <Page description={project.description ?? "统一查看空地设备、任务、媒体与告警的实时态势"} title={project.name}>
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {[["设备", snapshot.devices.length], ["活动任务", snapshot.activeTasks.length], ["媒体", snapshot.mediaPoints.length], ["开放告警", snapshot.openAlerts.length]].map(([label, value]) => (
          <Card key={String(label)}><CardHeader className="pb-1"><CardTitle className="text-sm font-medium text-muted-foreground">{label}</CardTitle></CardHeader><CardContent className="text-2xl font-semibold">{value}</CardContent></Card>
        ))}
      </div>
      <ProjectMap snapshot={snapshot} />
    </Page>
  );
}
