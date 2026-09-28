"use client";
import { useAPI } from "@/lib/use-api";

export function EvidenceImage({projectId,assetId}:{projectId:number;assetId:number}){
  const state = useAPI<{url:string}>(`/api/projects/${projectId}/assets/${assetId}/access?action=preview`);
  const url = state.data?.url;
  return url?<img alt="算法识别结果巡检原图" className="max-h-80 w-full rounded-lg bg-black/5 object-contain" src={url}/>:<div className="flex h-48 items-center justify-center rounded-lg bg-muted text-sm text-muted-foreground">原图加载中或不可用</div>;
}
