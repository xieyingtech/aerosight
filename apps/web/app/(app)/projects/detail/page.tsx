"use client";

import type { ComponentProps } from "react";
import { preload } from "react-dom";
import { Page } from "@/components/page";
import { OverviewMap } from "@/components/overview-map";
import { StaticAPIPage, positiveParam } from "@/components/static-api-page";
import { vectorStreetMapStyle } from "@/lib/map-style";

type Snapshot = ComponentProps<typeof OverviewMap>["snapshot"];
function Overview({ snapshot }: { snapshot: Snapshot }) {
  return <Page title={snapshot.project.name} variant="canvas">
    <OverviewMap snapshot={snapshot} />
  </Page>;
}
export default function ProjectOverview() {
  preload(vectorStreetMapStyle, { as: "fetch", crossOrigin: "anonymous" });
  return <StaticAPIPage<Snapshot> endpoint={(query) => { const id = positiveParam(query); return id ? `/api/projects/${id}/snapshot` : null; }}>{(snapshot) => <Overview key={snapshot.project.id} snapshot={snapshot} />}</StaticAPIPage>;
}
