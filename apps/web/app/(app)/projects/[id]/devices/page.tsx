import { DeviceTree } from "@/components/device-tree";
import { readProjectDeviceTree } from "@/lib/device-tree";
import { Page } from "@/components/page";

export default async function DevicesPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const nodes = await readProjectDeviceTree(Number(id));
  return <Page description="统一查看 DeviceType、Driver、拓扑、实时通道和有效能力" title="设备监控"><DeviceTree nodes={nodes} projectId={Number(id)} /></Page>;
}
