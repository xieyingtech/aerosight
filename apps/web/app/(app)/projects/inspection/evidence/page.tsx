"use client";
import Link from "next/link";
import {InspectionDetectionImage,type ExternalDetectionResult} from "@/components/inspection-detection-image";
import {Button} from "@/components/ui/button";
import {Page} from "@/components/page";
import {StaticAPIPage,positiveParam,uuidParam} from "@/components/static-api-page";
import {Card,CardContent,CardHeader,CardTitle} from "@/components/ui/card";
type Evidence={id:string;observationId:string;source:string;modelVersion:string;completeness:string;targetAlgorithmConfirmed:boolean;dataGaps?:string[];candidates:Array<{id:string;evidenceRefs:string[];position:{source:string;quality:string}}> ;externalResults?:ExternalDetectionResult[];nativeAlerts?:unknown[]};
export default function EvidencePage(){return <StaticAPIPage<Evidence> endpoint={q=>{const pid=positiveParam(q),id=uuidParam(q,"evidenceSetId");return pid&&id?`/api/projects/${pid}/inspection/evidence-sets/${id}`:null;}}>{(evidence,q,reload)=><Page title="识别证据" description={`${evidence.source==="external"?"外部算法":evidence.source==="flighthub-ai"?"司空原生告警":"未标明识别来源"} · ${evidence.completeness}`}><div className="space-y-4">
 <Button variant="outline" onClick={reload}>刷新识别证据</Button>
 <Link className="text-sm underline" href={`/projects/inspection/observation/?projectId=${positiveParam(q)}&observationId=${evidence.observationId}`}>查看观察范围与原图</Link>
 <p className="text-sm">模型版本：{evidence.externalResults?.length?Array.from(new Set(evidence.externalResults.map(item=>item.modelRevision))).join("、"):evidence.modelVersion} · 目标算法{evidence.targetAlgorithmConfirmed?"已确认":"未确认"}</p>
 <p className="text-sm text-muted-foreground">零检测或零告警不直接代表没有问题。结论应结合分析范围、算法覆盖及研判；照片位置不代表目标坐标。</p>
 {!!evidence.dataGaps?.length&&<ul className="list-inside list-disc text-sm">{evidence.dataGaps.map(gap=><li key={gap}>{gap}</li>)}</ul>}
 {(evidence.externalResults??[]).map(item=><InspectionDetectionImage key={item.algorithmRunId} projectId={positiveParam(q)!} observationId={evidence.observationId} item={item}/>)}
 <Card><CardHeader><CardTitle>候选目标（{evidence.candidates.length}）</CardTitle></CardHeader><CardContent className="space-y-3">{evidence.candidates.map(candidate=><div className="rounded border p-3 text-sm" key={candidate.id}><p>{candidate.id}</p><p>位置：{candidate.position.source==="capture"?"拍摄位置，非目标定位":candidate.position.source==="unknown"?"未知":candidate.position.quality==="verified"?"已核实目标位置":"目标位置未核实"}</p><p className="break-all text-xs">证据引用：{candidate.evidenceRefs.join("、")}</p></div>)}</CardContent></Card>
 <details className="rounded border p-3"><summary>识别原始结构与来源</summary><pre className="mt-3 overflow-auto whitespace-pre-wrap text-xs">{JSON.stringify({externalResults:evidence.externalResults??[],nativeAlerts:evidence.nativeAlerts??[]},null,2)}</pre></details>
 </div></Page>}</StaticAPIPage>;}
