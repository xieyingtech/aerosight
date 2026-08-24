import { BoxesIcon } from "lucide-react";
import { Page } from "@/components/page";
import { requireCurrentProjectPermission } from "@/lib/data";

export default async function AlgorithmsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  await requireCurrentProjectPermission(Number(id), "algorithm:manage");
  return (
    <Page description="通过 type、Base URL 和 secretRef 接入外部感知模型" title="算法服务">
      <div className="flex min-h-64 flex-col items-center justify-center rounded-lg border border-dashed bg-muted/20 p-8 text-center">
        <BoxesIcon className="mb-3 size-8 text-muted-foreground" />
        <p className="font-medium">尚未配置算法服务</p>
        <p className="mt-1 max-w-lg text-sm text-muted-foreground">后续可在统一协议框架中接入违建识别等 HTTP、KServe 或 OGC 算法服务。</p>
      </div>
    </Page>
  );
}
