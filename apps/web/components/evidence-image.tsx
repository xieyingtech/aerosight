"use client";
import { useEffect,useState } from "react";

export function EvidenceImage({projectId,assetId}:{projectId:number;assetId:number}){
  const [url,setURL]=useState<string|null>(null);
  useEffect(()=>{let active=true;fetch(`/api/projects/${projectId}/assets/${assetId}/access?action=preview`).then(async response=>response.ok?response.json():null).then(value=>{if(active&&value?.url)setURL(value.url)});return()=>{active=false}},[projectId,assetId]);
  return url?<img alt="疑似违建巡检原图" className="max-h-80 w-full rounded-lg bg-black/5 object-contain" src={url}/>:<div className="flex h-48 items-center justify-center rounded-lg bg-muted text-sm text-muted-foreground">原图加载中或不可用</div>;
}
