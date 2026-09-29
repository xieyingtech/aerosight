"use client";
import {useState} from 'react';
import {useAPI} from '@/lib/use-api';
import {validBox,type Detection} from '@/lib/algorithm-workspace';
export function AlgorithmAssetPreview({projectId,assetId,detections=[],compact=false,runId}:{projectId:number;assetId:number;detections?:Detection[];compact?:boolean;runId?:string}){
 const state=useAPI<{url:string}>(runId?null:`/api/projects/${projectId}/assets/${assetId}/access?action=preview`);
 const [size,setSize]=useState<{width:number;height:number}|null>(null);const [failed,setFailed]=useState(false);
 if(failed||state.error)return <p className="flex min-h-32 items-center justify-center rounded-md bg-muted p-4 text-sm text-muted-foreground">原图暂不可预览，识别数据仍可查看。</p>;
 if(!runId&&!state.data)return <div className="flex h-32 items-center justify-center bg-muted text-sm text-muted-foreground">加载图片…</div>;
 return <div className={`mx-auto ${compact?'max-w-52':'max-w-sm'}`}><div className="relative">
  <img alt="输入图片" src={runId?`/api/projects/${projectId}/algorithm-runs/${runId}/image`:state.data!.url} className="block h-auto w-full rounded-md" onLoad={e=>setSize({width:e.currentTarget.naturalWidth,height:e.currentTarget.naturalHeight})} onError={()=>setFailed(true)}/>
  {size&&detections.length>0&&<svg aria-label="识别目标框" className="pointer-events-none absolute inset-0 h-full w-full" viewBox={`0 0 ${size.width} ${size.height}`}>
   {detections.filter(validBox).map((d,i)=>{const b=d.pixelGeometry!;return <g key={`${d.detectionKey}-${i}`}><title>{d.label} {(d.confidence*100).toFixed(1)}%</title><rect x={b.x} y={b.y} width={b.width} height={b.height} fill="none" stroke="#22c55e" strokeWidth={2} vectorEffect="non-scaling-stroke"/><text x={b.x+4} y={Math.max(size.width/28,b.y-6)} fontSize={size.width/30} fill="#22c55e" stroke="#052e16" strokeWidth={2} paintOrder="stroke">{d.label} {(d.confidence*100).toFixed(0)}%</text></g>;})}
  </svg>}
 </div></div>;
}
