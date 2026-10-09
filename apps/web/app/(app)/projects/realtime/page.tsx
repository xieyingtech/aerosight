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
    <Page title="实时作业" variant="canvas">
      <RealtimeOperationsWorkbench key={snapshot.project.id} initialDeviceId={query.get("deviceId") ?? undefined} initialSnapshot={snapshot} autoLive={query.get("autoLive")==="1"} initialStreamId={query.get("streamId") ?? undefined} />
    </Page>
  );
    }}</StaticAPIPage>;
}

