"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeftIcon, CheckIcon, CloudIcon, PlusIcon } from "lucide-react";

import { DjiFlightHubSetup } from "@/components/dji-flighthub-wizard";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";

type ConnectorType = "dji.flighthub2";

export function ConnectorCreateDialog({ projectId, flightHubEnabled }: { projectId: number; flightHubEnabled: boolean }) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [selectedType, setSelectedType] = useState<ConnectorType | null>(null);

  const handleOpenChange = (nextOpen: boolean) => {
    setOpen(nextOpen);
    if (!nextOpen) setSelectedType(null);
  };

  const handleCreated = () => {
    handleOpenChange(false);
    router.refresh();
  };

  return <Dialog onOpenChange={handleOpenChange} open={open}>
    <DialogTrigger asChild><Button><PlusIcon />新建连接器</Button></DialogTrigger>
    <DialogContent className="max-h-[calc(100vh-2rem)] overflow-y-auto sm:max-w-3xl">
      <DialogHeader>
        <DialogTitle>{selectedType ? "配置 DJI 司空 2" : "选择连接器类型"}</DialogTitle>
        <DialogDescription>{selectedType ? "验证组织 Token 并选择要同步的司空项目。" : "请选择当前项目要接入的平台。"}</DialogDescription>
      </DialogHeader>

      {selectedType === null ? <div className="py-2">
        <button
          className="flex w-full items-start gap-4 rounded-xl border p-4 text-left transition-colors hover:border-primary hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          onClick={() => setSelectedType("dji.flighthub2")}
          type="button"
        >
          <span className="rounded-lg bg-primary/10 p-2 text-primary"><CloudIcon className="size-5" /></span>
          <span className="min-w-0 flex-1">
            <span className="flex items-center gap-2 font-medium">DJI 司空 2 <span className="rounded-full bg-muted px-2 py-0.5 text-xs font-normal text-muted-foreground">公有云 · 只读</span></span>
            <span className="mt-1 block text-sm text-muted-foreground">通过组织 Token 同步已有司空项目、机场与飞行器目录，无需现场配置设备网络。</span>
          </span>
          <CheckIcon className="mt-1 size-4 text-muted-foreground" />
        </button>
        <p className="mt-3 text-xs text-muted-foreground">其他依赖现场设备的接入方式将在具备设备并完成验收后开放。</p>
      </div> : <div className="space-y-4 py-2">
        <Button onClick={() => setSelectedType(null)} size="sm" type="button" variant="ghost"><ArrowLeftIcon />返回类型选择</Button>
        <DjiFlightHubSetup enabled={flightHubEnabled} onCreated={handleCreated} projectId={projectId} />
      </div>}
    </DialogContent>
  </Dialog>;
}
