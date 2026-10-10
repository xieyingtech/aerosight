"use client";

import {useEffect} from "react";
import {useRouter} from "next/navigation";
import {Page} from "@/components/page";
import {AssetLibrary} from "@/components/asset-library";
import {positiveParam,StaticAPIPage,type PageQuery} from "@/components/static-api-page";
import {projectPageHref} from "@/lib/page-routes";
function MaterialList({query}:{query:PageQuery}) {
  const router=useRouter();
  const projectId=positiveParam(query)!;
  const assetId=positiveParam(query,"assetId");
  const type=query.get("type");
  const parameters=Object.fromEntries(new URLSearchParams(query.toString()));
  const detail=assetId?projectPageHref(projectId,"assets/detail",{...parameters,assetId}):null;
  useEffect(()=>{if(detail)router.replace(detail);},[detail,router]);
  if(detail)return <p role="status" className="text-sm text-muted-foreground">正在打开素材详情…</p>;
  return <Page title="素材" description="查找照片与视频，打开素材查看详情"><AssetLibrary key={projectId} projectId={projectId} initialQuery={query.get("q") ?? ""} initialFilter={type === "video" || type === "image"?type:"all"}/></Page>;
}
export default function WorkspacePage(){return <StaticAPIPage<unknown[]> endpoint={query=>{const id=positiveParam(query);return id?`/api/projects/${id}/assets`:null;}}>{(_items,query)=><MaterialList query={query}/>}</StaticAPIPage>;}
