import { DjiAdapterWizard } from "@/components/dji-adapter-wizard";
import { DjiFlightHubWizard } from "@/components/dji-flighthub-wizard";
import { Page } from "@/components/page";
import { getProject } from "@/lib/data";
import { listDeviceAdapters } from "@/lib/device-adapters";
import { parseFlightHubWebConfig } from "@/lib/dji-flighthub-config";
import { listFlightHubConnections, listFlightHubDiscoveryActivity } from "@/lib/dji-flighthub-lifecycle";

export default async function ConnectorsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const projectId = Number(id);
  const project = await getProject(projectId);
  if (project.role === "member") throw new Error("PROJECT_ACCESS_DENIED");
  const [connectors, flightHubConnections, flightHubActivity] = await Promise.all([
    listDeviceAdapters(projectId),
    listFlightHubConnections(projectId),
    listFlightHubDiscoveryActivity(projectId),
  ]);
  const flightHubEnabled = parseFlightHubWebConfig(process.env).enabled;

  return (
    <Page
      description="管理外部 IoT 平台连接、网络端点、加密凭据和设备发现范围"
      title="连接器"
    >
      <div className="space-y-5">
        <DjiFlightHubWizard
          enabled={flightHubEnabled}
          initialConnectors={flightHubConnections}
          initialIdentities={flightHubActivity.identities}
          initialSyncRuns={flightHubActivity.syncRuns}
          projectId={projectId}
        />
        <DjiAdapterWizard initialAdapters={connectors} projectId={projectId} />
      </div>
    </Page>
  );
}
