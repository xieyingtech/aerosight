"use client";
import type { ComponentProps } from "react";
import { ConnectorCreateDialog } from "@/components/connector-create-dialog";
import { DjiFlightHubConnections, type OtherConnectorSummary } from "@/components/dji-flighthub-wizard";
import type { AdapterSummary } from "@/components/dji-adapter-wizard";
import { Page } from "@/components/page";
import { positiveParam, StaticAPIPage } from "@/components/static-api-page";
import { APIStateView } from "@/components/api-state";
import { useAPI } from "@/lib/use-api";
type Props=ComponentProps<typeof DjiFlightHubConnections>;
type Data={connectors:Props["initialConnectors"];identities:Props["initialIdentities"];syncRuns:Props["initialSyncRuns"]};
export default function ConnectorsPage(){return <StaticAPIPage<{id:number;role:string}> endpoint={q=>{const id=positiveParam(q);return id?`/api/projects/${id}`:null;}}>{p=>p.role==="member"?<p role="alert">你没有项目管理权限。</p>:<Workspace key={p.id} projectId={p.id}/>}</StaticAPIPage>;}
function Workspace({projectId}:{projectId:number}){
 const adapters=useAPI<AdapterSummary[]>(`/api/projects/${projectId}/device-adapters`);
 const flightHub=useAPI<Data>(`/api/projects/${projectId}/connectors/dji-flighthub`);
 return <Page title="连接器" description="查看和管理项目已接入的平台连接" actions={<ConnectorCreateDialog projectId={projectId} onChanged={()=>{flightHub.reload();adapters.reload();}}/>}>
 <APIStateView state={adapters}>{all=><APIStateView state={flightHub}>{data=>{
 const ids=new Set(data.connectors.map(c=>c.id));const other:OtherConnectorSummary[]=all.filter(c=>!ids.has(c.id)).map(c=>({id:c.id,name:c.name,status:c.status,typeLabel:c.adapterType==="dji"?"DJI Cloud API":"模拟器",version:c.protocolVersion,lastCheckedAt:c.lastCheckedAt}));
 return <DjiFlightHubConnections projectId={projectId} initialConnectors={data.connectors} initialIdentities={data.identities} initialSyncRuns={data.syncRuns} otherConnectors={other}/>;
 }}</APIStateView>}</APIStateView></Page>;
}
