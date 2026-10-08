"use client";

import { Page } from "@/components/page";
import { AssetLibrary } from "@/components/asset-library";

import { positiveParam, StaticAPIPage } from "@/components/static-api-page";
export default function WorkspacePage() {
  return <StaticAPIPage<Record<string, unknown>[]> endpoint={(query) => {const id=positiveParam(query); return id ? `/api/projects/${id}/assets` : null;}}>
    {(_items, query) => { const projectId=positiveParam(query)!;
  return <Page title="素材库"><AssetLibrary key={projectId} projectId={projectId} /></Page>;
    }}</StaticAPIPage>;
}
