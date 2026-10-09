"use client";

import type { ComponentProps } from "react";
import { Page } from "@/components/page";
import { OverviewMap } from "@/components/overview-map";
import { StaticAPIPage, positiveParam } from "@/components/static-api-page";

type Snapshot = ComponentProps<typeof OverviewMap>["snapshot"];
function Overview({ snapshot }: { snapshot: Snapshot }) {
  return <Page title={snapshot.project.name} variant="canvas">
    <OverviewMap snapshot={snapshot} />
  </Page>;
}
export default function ProjectOverview() {
  return <StaticAPIPage<Snapshot> endpoint={(query) => { const id = positiveParam(query); return id ? `/api/projects/${id}/snapshot` : null; }}>{(snapshot) => <Overview key={snapshot.project.id} snapshot={snapshot} />}</StaticAPIPage>;
}
