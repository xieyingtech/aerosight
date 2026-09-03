import { Page } from "@/components/page";
import { getProject, requireUser } from "@/lib/data";
import { readProjectSituationSnapshot } from "@/lib/project-snapshot";
import { RealtimeOperationsWorkbench } from "@/components/realtime-operations-workbench";

export default async function RealtimeOperationsPage({ params, searchParams }: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ deviceId?: string; streamId?: string }>;
}) {
  const { id } = await params;
  const query = await searchParams;
  const projectId = Number(id);
  const [project, user] = await Promise.all([getProject(projectId), requireUser()]);
  const snapshot = await readProjectSituationSnapshot(user.id, projectId);
  if (!snapshot) throw new Error("PROJECT_NOT_FOUND");
  return (
    <Page description={`${project.name} 的在线设备、直播与任务控制`} title="实时作业" variant="workspace">
      <RealtimeOperationsWorkbench initialDeviceId={query.deviceId} initialSnapshot={snapshot} initialStreamId={query.streamId} />
    </Page>
  );
}
