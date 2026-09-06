"use client";

import { DataTable } from "@/components/data-table";
import { Page } from "@/components/page";

import { positiveParam, StaticAPIPage } from "@/components/static-api-page";
export default function WorkspacePage() {
  return <StaticAPIPage<Record<string, unknown>[]> endpoint={(query) => {const id=positiveParam(query); return id ? `/api/projects/${id}/assets` : null;}}>
    {(items, query) => { const projectId=positiveParam(query)!;
  return <Page title="素材库"><DataTable columns={[{ key: "kind", label: "类型" }, { key: "mimeType", label: "MIME" }, { key: "createdAt", label: "创建时间" }]} items={items} /></Page>;
    }}</StaticAPIPage>;
}
