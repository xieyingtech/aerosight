import { DeviceTree } from "@/components/device-tree";
import { DeviceDiscoveryManager } from "@/components/device-discovery-manager";
import { readProjectDiscoveries } from "@/lib/device-discoveries";
import { readProjectDeviceTree } from "@/lib/device-tree";
import { Page } from "@/components/page";

export default async function DevicesPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const projectId = Number(id);
  const [nodes, catalog] = await Promise.all([readProjectDeviceTree(projectId), readProjectDiscoveries(projectId)]);
  return <Page description="管理设备资产、DeviceType、Driver、拓扑、实时通道和有效能力" title="设备管理">
    <DeviceDiscoveryManager {...catalog} projectId={projectId} />
    <DeviceTree nodes={nodes} projectId={projectId} />
  </Page>;
}
