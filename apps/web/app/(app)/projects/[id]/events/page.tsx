import Link from "next/link";
import { BellRingIcon } from "lucide-react";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
import { Card,CardContent,CardHeader,CardTitle } from "@/components/ui/card";
import { listPerceptionEvents } from "@/lib/perception-events";

export default async function EventsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const projectId=Number(id);const events=await listPerceptionEvents(projectId);
  return (
    <Page description="算法结果均为疑似线索，不构成法律意义上的违建认定" title="疑似违建告警">
      {events.length?<div className="space-y-3">{events.map(event=><Link href={`/projects/${projectId}/events/${String(event.id)}`} key={String(event.id)}><Card className="transition-colors hover:bg-muted/30"><CardHeader><div className="flex flex-wrap items-center justify-between gap-2"><CardTitle>疑似违建</CardTitle><div className="flex gap-2"><Badge variant="outline">{String(event.severity)}</Badge><Badge variant="outline">{String(event.status)}</Badge></div></div></CardHeader><CardContent className="grid gap-2 text-sm md:grid-cols-3"><span>位置质量：{String(event.locationQuality)}</span><span>线索次数：{String(event.occurrenceCount)}</span><time className="text-muted-foreground">{event.lastDetectedAt instanceof Date?event.lastDetectedAt.toLocaleString("zh-CN"):String(event.lastDetectedAt)}</time></CardContent></Card></Link>)}</div>:<div className="flex min-h-64 flex-col items-center justify-center rounded-lg border border-dashed bg-muted/20 p-8 text-center">
        <BellRingIcon className="mb-3 size-8 text-muted-foreground" />
        <p className="font-medium">暂无开放告警</p>
        <p className="mt-1 text-sm text-muted-foreground">算法识别结果进入事件规则后，会在此保留完整证据链。</p>
      </div>}
    </Page>
  );
}
