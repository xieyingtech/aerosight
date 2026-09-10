"use client";
import type { ComponentProps } from "react";
import { Page } from "@/components/page";
import { RealtimeOperationsWorkbench } from "@/components/realtime-operations-workbench";
import { positiveParam, StaticAPIPage } from "@/components/static-api-page";
type Snapshot = ComponentProps<typeof RealtimeOperationsWorkbench>["initialSnapshot"];
export default function RealtimeOperationsPage() {
  return <StaticAPIPage<Snapshot> endpoint={(query) => { const id = positiveParam(query); return id ? `/api/projects/${id}/snapshot` : null; }}>
    {(snapshot, query) => {
  return (
    <Page description={`${snapshot.project.name} 的在线设备、直播与任务控制`} title="实时作业" variant="workspace">
      <RealtimeOperationsWorkbench key={snapshot.project.id} initialDeviceId={query.get("deviceId") ?? undefined} initialSnapshot={snapshot} initialStreamId={query.get("streamId") ?? undefined} />
    </Page>
  );
    }}</StaticAPIPage>;
}
