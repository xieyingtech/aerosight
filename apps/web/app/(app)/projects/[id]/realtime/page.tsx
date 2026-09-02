import { Page } from "@/components/page";
import { getProject, requireUser } from "@/lib/data";
import { readProjectSituationSnapshot } from "@/lib/project-snapshot";
import { RealtimeOperationsWorkbench } from "@/components/realtime-operations-workbench";
import { DJIFlightHubLiveMediaPanel } from "@/components/dji-flighthub-live-media-panel";
import { readFlightHubLiveMedia } from "@/lib/dji-flighthub-live-media";

export default async function RealtimeOperationsPage({ params, searchParams }: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ deviceId?: string; streamId?: string }>;
}) {
  const { id } = await params;
  const query = await searchParams;
  const projectId = Number(id);
  const [project, user] = await Promise.all([getProject(projectId), requireUser()]);
  const [snapshot, liveMedia] = await Promise.all([readProjectSituationSnapshot(user.id, projectId), readFlightHubLiveMedia(projectId)]);
  if (!snapshot) throw new Error("PROJECT_NOT_FOUND");
  return (
    <Page description={`${project.name} 的在线设备、直播与任务控制`} title="实时作业">
      <DJIFlightHubLiveMediaPanel media={liveMedia} />
      <RealtimeOperationsWorkbench initialDeviceId={query.deviceId} initialSnapshot={snapshot} initialStreamId={query.streamId} />
    </Page>
  );
}
