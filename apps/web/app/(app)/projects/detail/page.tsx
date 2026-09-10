"use client";

import type { ComponentProps } from "react";
import { Page } from "@/components/page";
import { OverviewMap } from "@/components/overview-map";
import { StaticAPIPage, positiveParam } from "@/components/static-api-page";
import { useAPI } from "@/lib/use-api";
import { APIStateView } from "@/components/api-state";

type Snapshot = ComponentProps<typeof OverviewMap>["snapshot"];
function Overview({ snapshot }: { snapshot: Snapshot }) {
  const project = useAPI<{ name: string; description: string | null }>(`/api/projects/${snapshot.project.id}`);
  return <APIStateView state={project}>{(project) => <Page description={project.description ?? "统一查看空地设备、任务、媒体与案件的实时态势"} title={project.name} variant="workspace">
    <OverviewMap snapshot={snapshot} />
  </Page>}</APIStateView>;
}
export default function ProjectOverview() {
  return <StaticAPIPage<Snapshot> endpoint={(query) => { const id = positiveParam(query); return id ? `/api/projects/${id}/snapshot` : null; }}>{(snapshot) => <Overview key={snapshot.project.id} snapshot={snapshot} />}</StaticAPIPage>;
}
