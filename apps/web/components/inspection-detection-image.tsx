"use client";
import Link from "next/link";
import {useState} from "react";
export type ExternalDetectionResult={algorithmRunId:string;modelRevision:string;asset:{assetId:number;version:number};result:{detections:Array<{detectionKey:string;label:string;confidence:number;pixelGeometry:{type:string;x:number;y:number;width:number;height:number}}>}};
export function InspectionDetectionImage({projectId,observationId,item}:{projectId:number;observationId:string;item:ExternalDetectionResult}){
 const [size,setSize]=useState<{width:number;height:number}|null>(null);
 const [failed,setFailed]=useState(false);
 const detections=item.result.detections;
 return <section className="space-y-3 rounded-lg border p-4">
  <div className="flex flex-wrap justify-between gap-2"><h2 className="font-medium">原图 #{item.asset.assetId} · {detections.length} 个预测目标</h2><Link className="text-sm underline" href={`/projects/algorithms/runs/detail/?projectId=${projectId}&runId=${item.algorithmRunId}`}>模型 {item.modelRevision} · 运行详情</Link></div>
  <p className="text-xs text-muted-foreground">框为原图像素坐标，预测结果需复核；不代表目标地理位置或违规认定。</p>
  {failed?<p role="alert">冻结原图不可用，请检查资产版本后刷新。</p>:<div className="relative mx-auto max-w-6xl">
   <img alt={`巡检原图 ${item.asset.assetId}，叠加 ${detections.length} 个预测框`} className="block h-auto w-full" src={`/api/projects/${projectId}/inspection/observations/${observationId}/assets/${item.asset.assetId}/content`} onLoad={e=>setSize({width:e.currentTarget.naturalWidth,height:e.currentTarget.naturalHeight})} onError={()=>setFailed(true)}/>
   {size&&<svg aria-label="检测框叠加图" className="pointer-events-none absolute inset-0 h-full w-full" viewBox={`0 0 ${size.width} ${size.height}`}>{detections.filter(d=>d.pixelGeometry.type==="bbox").map(d=>{const b=d.pixelGeometry;return <g key={d.detectionKey}><title>{d.label} {(d.confidence*100).toFixed(1)}%</title><rect x={b.x} y={b.y} width={b.width} height={b.height} fill="none" stroke="#ff2424" strokeWidth={2} vectorEffect="non-scaling-stroke"/><text x={b.x} y={Math.max(28,b.y-10)} fill="#ff2424" stroke="white" strokeWidth={3} paintOrder="stroke" fontSize={size.width/180}>{d.label} {(d.confidence*100).toFixed(0)}%</text></g>;})}</svg>}
  </div>}
 </section>;
}
