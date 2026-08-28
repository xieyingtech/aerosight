import { DjiAdapterWizard } from "@/components/dji-adapter-wizard";
import { Page } from "@/components/page";
import { getProject } from "@/lib/data";
import { listDeviceAdapters } from "@/lib/device-adapters";

export default async function ConnectorsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const projectId = Number(id);
  const project = await getProject(projectId);
  if (project.role === "member") throw new Error("PROJECT_ACCESS_DENIED");
  const connectors = await listDeviceAdapters(projectId);

  return (
    <Page
      description="管理外部 IoT 平台连接、网络端点、加密凭据和设备发现范围"
      title="连接器"
    >
      <DjiAdapterWizard initialAdapters={connectors} projectId={projectId} />
    </Page>
  );
}
