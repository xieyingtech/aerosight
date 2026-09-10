"use client";
import type { ComponentProps } from "react";
import { DeviceTree } from "@/components/device-tree";
import { DeviceDiscoveryManager } from "@/components/device-discovery-manager";
import { Page } from "@/components/page";
import { positiveParam, StaticAPIPage } from "@/components/static-api-page";
import { APIStateView } from "@/components/api-state";
import { useAPI } from "@/lib/use-api";
type Catalog = Omit<ComponentProps<typeof DeviceDiscoveryManager>, "projectId" | "onChanged">;
export default function DevicesPage() {
 return <StaticAPIPage<Catalog> endpoint={query => {const id=positiveParam(query);return id?`/api/projects/${id}/device-adapters/discoveries`:null;}}>
 {(catalog,query,reload)=><Workspace key={positiveParam(query)!} projectId={positiveParam(query)!} catalog={catalog} reload={reload}/>}
 </StaticAPIPage>;
}
function Workspace({projectId,catalog,reload}:{projectId:number;catalog:Catalog;reload:()=>void}) {
 const nodes=useAPI<ComponentProps<typeof DeviceTree>["nodes"]>(`/api/projects/${projectId}/device-tree`);
 return <Page title="设备管理" description="管理设备资产、DeviceType、Driver、拓扑、实时通道和有效能力">
 <DeviceDiscoveryManager {...catalog} projectId={projectId} onChanged={reload}/>
 <APIStateView state={nodes}>{data=><DeviceTree projectId={projectId} nodes={data}/>}</APIStateView>
 </Page>;
}
