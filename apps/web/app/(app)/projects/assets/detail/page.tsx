"use client";

import Link from "next/link";
import {ArrowLeftIcon} from "lucide-react";
import {Page} from "@/components/page";
import {AssetViewer} from "@/components/asset-viewer";
import {positiveParam,StaticAPIPage} from "@/components/static-api-page";
import {projectPageHref} from "@/lib/page-routes";
import type {AlgorithmAsset} from "@/lib/algorithm-workspace";

export default function MaterialDetailPage(){return <StaticAPIPage<AlgorithmAsset[]> endpoint={query=>{const pid=positiveParam(query),aid=positiveParam(query,"assetId");return pid && aid?`/api/projects/${pid}/assets`:null;}}>{(assets,query)=>{
  const projectId=positiveParam(query)!;
  const assetId=positiveParam(query,"assetId")!;
  const asset=assets.find(a=>a.id === assetId);
  const start=Number(query.get("startMs")),end=Number(query.get("endMs"));
  const startMs=Number.isSafeInteger(start) && start>=0?start:0;
  const endMs=Number.isSafeInteger(end) && end>startMs?end:undefined;
  const type=query.get("type"),q=query.get("q");
  const back=projectPageHref(projectId,"assets",{...(q?{q}:{}),...(type === "image" || type === "video"?{type}:{})});
  return <Page title="素材详情" description="查看原片、拍摄信息和素材来源"><div className="space-y-4"><Link href={back} className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground"><ArrowLeftIcon className="size-4"/>返回素材</Link>{asset?<AssetViewer key={`${projectId}-${assetId}`} projectId={projectId} asset={asset} startMs={startMs} endMs={endMs}/>:<p role="alert" className="rounded-xl border p-10 text-center text-muted-foreground">素材不存在或已不可用，请返回素材列表。</p>}</div></Page>;
}}</StaticAPIPage>;}
