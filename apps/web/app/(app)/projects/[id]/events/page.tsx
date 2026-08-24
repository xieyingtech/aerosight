import { BellRingIcon } from "lucide-react";
import { Page } from "@/components/page";
import { getProject } from "@/lib/data";

export default async function EventsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  await getProject(Number(id));
  return (
    <Page description="汇总疑似违建识别、设备异常和人工处置状态" title="告警事件">
      <div className="flex min-h-64 flex-col items-center justify-center rounded-lg border border-dashed bg-muted/20 p-8 text-center">
        <BellRingIcon className="mb-3 size-8 text-muted-foreground" />
        <p className="font-medium">暂无开放告警</p>
        <p className="mt-1 text-sm text-muted-foreground">算法识别结果进入事件规则后，会在此保留完整证据链。</p>
      </div>
    </Page>
  );
}
