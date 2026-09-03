import { ConnectorCreateDialog } from "@/components/connector-create-dialog";
import { DjiFlightHubConnections, type OtherConnectorSummary } from "@/components/dji-flighthub-wizard";
import { Page } from "@/components/page";
import { getProject } from "@/lib/data";
import { listDeviceAdapters } from "@/lib/device-adapters";
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
  const flightHubIds = new Set(flightHubConnections.map((connector) => connector.id));
  const otherConnectors: OtherConnectorSummary[] = connectors
    .filter((connector) => !flightHubIds.has(connector.id))
    .map((connector) => ({
      id: connector.id,
      name: connector.name,
      status: connector.status,
      typeLabel: connector.adapterType === "dji" ? "DJI Cloud API" : "模拟器",
      version: connector.protocolVersion,
      lastCheckedAt: connector.lastCheckedAt,
    }));

  return (
    <Page
      actions={<ConnectorCreateDialog projectId={projectId} />}
      description="查看和管理项目已接入的平台连接"
      title="连接器"
    >
      <DjiFlightHubConnections
        initialConnectors={flightHubConnections}
        initialIdentities={flightHubActivity.identities}
        initialSyncRuns={flightHubActivity.syncRuns}
        otherConnectors={otherConnectors}
        projectId={projectId}
      />
    </Page>
  );
}
