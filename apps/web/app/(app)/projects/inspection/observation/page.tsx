"use client";
import Link from "next/link";
import {useState} from "react";
import {Button} from "@/components/ui/button";
import {Page} from "@/components/page";
import {StaticAPIPage,positiveParam,uuidParam} from "@/components/static-api-page";
import {Card,CardContent,CardHeader,CardTitle} from "@/components/ui/card";
type Asset={sourceDeviceMode?:string;assetId:number;version:number;checksumSha256?:string;objectVersion?:string;sourceRunId?:number};
type Observation={id:string;run:{runId:number};mode:string;completeness:string;scopeDescription:string;observedFrom:string;observedTo:string;assets:Asset[];dataGaps?:string[];flight?:{flightUuid:string;projectedRunId?:number}};
function OriginalImage({projectId,observationId,asset}:{projectId:number;observationId:string;asset:Asset}){
 const [failed,setFailed]=useState(false);
 return <Card><CardHeader><CardTitle>原图 {asset.assetId} · 版本 {asset.version}</CardTitle></CardHeader><CardContent className="space-y-2 text-sm">
 {failed?<p role="alert" className="text-destructive">原图不可用、版本发生变化或无法验证。请刷新重试；不会用当前文件替代冻结证据。</p>:<img className="max-h-96 w-full rounded object-contain" loading="lazy" src={`/api/projects/${projectId}/inspection/observations/${observationId}/assets/${asset.assetId}/content`} alt={`观察原图 ${asset.assetId}`} onError={()=>setFailed(true)}/>}
 <p className="text-sm">{asset.sourceDeviceMode==="simulator"?"关联模拟设备，不作为实飞证明":"实拍来源尚未核验"}</p>
 <p className="break-all text-xs">SHA256：{asset.checksumSha256??"未记录，无法验证原图"}</p>
 {asset.sourceRunId&&<Link className="underline" href={`/projects/tasks/runs/detail/?projectId=${projectId}&runId=${asset.sourceRunId}`}>查看来源运行</Link>}
 </CardContent></Card>;
}
export default function ObservationPage(){return <StaticAPIPage<Observation> endpoint={q=>{const pid=positiveParam(q),id=uuidParam(q,"observationId");return pid&&id?`/api/projects/${pid}/inspection/observations/${id}`:null;}}>{(observation,q,reload)=><Page title="观察范围与原图" description={observation.scopeDescription}><div className="space-y-4">
 <Button variant="outline" onClick={reload}>刷新观察与原图</Button>
 <Link className="text-sm underline" href={`/projects/tasks/runs/detail/?projectId=${positiveParam(q)}&runId=${observation.run.runId}`}>返回业务运行</Link>
 <p className="text-sm">来源：{observation.mode==="assets"?"既有图片":observation.mode==="existing-flight"?"司空已完成飞行":observation.mode==="flighthub-flight"?"司空飞行":"未标明来源"} · 完整性：{observation.completeness} · {observation.assets.length} 张图片</p>
 <p className="text-sm">{observation.observedFrom} — {observation.observedTo}</p>
 <p className="text-sm text-muted-foreground">结论仅适用于本页列出的图片和时段。既有图片分析不代表本次现场航拍；图片位置不等于目标坐标。</p>
 {observation.flight&&<p className="text-sm">飞行标识：{observation.flight.flightUuid}</p>}
 {observation.flight?.projectedRunId&&<Link className="text-sm underline" href={`/projects/tasks/runs/detail/?projectId=${positiveParam(q)}&runId=${observation.flight.projectedRunId}`}>查看司空来源飞行记录</Link>}
 {!!observation.dataGaps?.length&&<ul className="list-inside list-disc text-sm">{observation.dataGaps.map(gap=><li key={gap}>{gap}</li>)}</ul>}
 <div className="grid gap-4 lg:grid-cols-2">{observation.assets.map(asset=><OriginalImage key={`${asset.assetId}:${asset.version}`} projectId={positiveParam(q)!} observationId={observation.id} asset={asset}/>)}</div>
 </div></Page>}</StaticAPIPage>;}
