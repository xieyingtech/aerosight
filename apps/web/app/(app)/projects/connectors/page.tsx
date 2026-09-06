"use client";
import type { ComponentProps } from "react";
import { DjiAdapterWizard } from "@/components/dji-adapter-wizard";
import { DjiFlightHubWizard } from "@/components/dji-flighthub-wizard";
import { Page } from "@/components/page";
import { positiveParam, StaticAPIPage } from "@/components/static-api-page";
import { APIStateView } from "@/components/api-state";
import { useAPI } from "@/lib/use-api";
type FlightHubProps=ComponentProps<typeof DjiFlightHubWizard>;
type FlightHubData={connectors:FlightHubProps["initialConnectors"];identities:FlightHubProps["initialIdentities"];syncRuns:FlightHubProps["initialSyncRuns"]};
export default function ConnectorsPage(){
 return <StaticAPIPage<{id:number;role:string}> endpoint={(query)=>{const id=positiveParam(query);return id?`/api/projects/${id}`:null;}}>
 {project=>project.role==="member"?<p className="p-4 text-sm text-destructive" role="alert">你没有项目管理权限。</p>:<Workspace key={project.id} projectId={project.id}/>}
 </StaticAPIPage>;
}
function Workspace({projectId}:{projectId:number}){
 const adapters=useAPI<ComponentProps<typeof DjiAdapterWizard>["initialAdapters"]>(`/api/projects/${projectId}/device-adapters`);
 const flightHub=useAPI<FlightHubData>(`/api/projects/${projectId}/connectors/dji-flighthub`);
 const features=useAPI<{flightHubEnabled:boolean}>(`/api/projects/${projectId}/features`);
 return <Page title="连接器" description="管理外部 IoT 平台连接、网络端点、加密凭据和设备发现范围"><div className="space-y-5">
 <APIStateView state={features}>{flags=><APIStateView state={flightHub}>{data=><DjiFlightHubWizard enabled={flags.flightHubEnabled} initialConnectors={data.connectors} initialIdentities={data.identities} initialSyncRuns={data.syncRuns} projectId={projectId}/>}</APIStateView>}</APIStateView>
 <APIStateView state={adapters}>{data=><DjiAdapterWizard initialAdapters={data} projectId={projectId}/>}</APIStateView>
 </div></Page>;
}
