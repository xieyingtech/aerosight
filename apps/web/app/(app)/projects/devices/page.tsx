"use client";

import type { ComponentProps } from "react";
import { DeviceTree } from "@/components/device-tree";
import { Page } from "@/components/page";
import { positiveParam, StaticAPIPage } from "@/components/static-api-page";

type Data = ComponentProps<typeof DeviceTree>["nodes"];
export default function WorkspacePage() {
  return <StaticAPIPage<Data> endpoint={(query) => { const id = positiveParam(query); return id ? `/api/projects/${id}/device-tree` : null; }}>
    {(data, query) => <Page title="设备管理" description="管理设备资产、DeviceType、Driver、拓扑、实时通道和有效能力"><DeviceTree key={positiveParam(query)!} projectId={positiveParam(query)!} nodes={data} /></Page>}
  </StaticAPIPage>;
}
