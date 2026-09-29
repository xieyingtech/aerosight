"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { ArrowLeft, CheckCircle2, CircleDashed, FileText, ImageIcon, List, XCircle } from "lucide-react";
import { AlgorithmAssetPreview } from "@/components/algorithm-asset-preview";
import { AlgorithmRunRetryButton } from "@/components/algorithm-run-retry-button";
import { Page } from "@/components/page";
import { Badge } from "@/components/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { positiveParam, uuidParam, StaticAPIPage, type PageQuery } from "@/components/static-api-page";
import { resolveObjectSelection, objectReviewExport } from "@/lib/algorithm-object-selection";
import { apiJSON } from "@/lib/api-client";
import { useAPI } from "@/lib/use-api";
import { assetName, formatDuration, runDetections, runStatus, type AlgorithmAsset } from "@/lib/algorithm-workspace";
import type { AlgorithmRunDetail } from "@/lib/web-api-types";

type Section = "result" | "summary" | "logs";
function RunDetail({ initial, projectId, query }: { initial: AlgorithmRunDetail; projectId: number; query: PageQuery }) {
 const [model, setModel] = useState(initial);
 const [section, setSection] = useState<Section>("result");
 const [showBoxes, setShowBoxes] = useState(true);
 const [showAll, setShowAll] = useState(false);
 const [refreshError, setRefreshError] = useState(false);
 const {run, attempts, view} = model;
 const assets = useAPI<AlgorithmAsset[]>(`/api/projects/${projectId}/assets`);
 const asset = assets.data?.find(a => a.id === run.inputAssetId);
 const allDetections = runDetections(run.canonicalResult);
 const selection = resolveObjectSelection(allDetections, query);
 const detections = showAll ? allDetections : selection.detections;
 const selectionActive = selection.active && !showAll;
 const downloadReview = () => {
  const data=objectReviewExport(run.id,run.inputAssetId,view.input.assetVersion,detections,selectionActive?selection.reason:'全部检测候选');
  const url=URL.createObjectURL(new Blob([JSON.stringify(data,null,2)],{type:'application/json'}));
  const link=document.createElement('a');link.href=url;link.download=`object-review-${run.id.slice(0,8)}.json`;link.click();setTimeout(()=>URL.revokeObjectURL(url),1000);
 };
 const isDetection = run.canonicalResult.kind === "detection";
 const durations = attempts.map(a => a.durationMs).filter((n): n is number => typeof n === "number" && Number.isFinite(n));
 const duration = durations.length ? durations.reduce((a,b)=>a+b,0) : null;
 const active = ["queued", "running", "polling", "waiting_callback"].includes(run.status);
 useEffect(() => {
  if (!active) return;
  const controller = new AbortController();
  let timer: ReturnType<typeof setTimeout>;
  async function refresh() {
   try { const next = await apiJSON<AlgorithmRunDetail>(`/api/projects/${projectId}/algorithm-runs/${initial.run.id}`, {signal:controller.signal}); if (!controller.signal.aborted) {setModel(next); setRefreshError(false);} }
   catch {if (!controller.signal.aborted) setRefreshError(true);}
   if (!controller.signal.aborted) timer = setTimeout(refresh, 3000);
  }
  timer = setTimeout(refresh, 1500);
  return () => {controller.abort(); clearTimeout(timer);};
 }, [active, projectId, initial.run.id]);
 const StatusIcon = run.status === "succeeded" ? CheckCircle2 : ["failed", "timed_out"].includes(run.status) ? XCircle : CircleDashed;
 return <Page title={run.definitionName} description={`运行 #${run.id.slice(0,8)}`} actions={view.retryAllowed ? <AlgorithmRunRetryButton projectId={projectId} runId={run.id}/> : undefined}>
  <Link href={`/projects/${projectId}/algorithms/`} className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground"><ArrowLeft className="size-4"/>所有运行</Link>
  <div className="mt-4 flex flex-wrap items-center gap-x-6 gap-y-2 border-y py-4 text-sm">
   <span className="inline-flex items-center gap-2"><StatusIcon className={`size-4 ${run.status==='succeeded'?'text-green-600':['failed','timed_out'].includes(run.status)?'text-destructive':''}`}/>{runStatus(run.status)}</span>
   <span>{run.providerName}</span><span className="text-muted-foreground">调用耗时 {formatDuration(duration)}</span>
   <time className="text-muted-foreground">{new Date(run.createdAt).toLocaleString('zh-CN')}</time>
  </div>
  {run.errorCode && <p role="alert" className="mt-4 rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">{run.errorMessage || run.errorCode}</p>}
  {refreshError && <p role="status" className="mt-3 text-sm text-muted-foreground">状态更新暂时失败，正在重试。</p>}
  <div className="mt-6 grid gap-6 lg:grid-cols-[200px_minmax(0,1fr)]">
   <nav aria-label="运行详情" className="flex gap-1 lg:flex-col">
    {([{id:'result',label:'识别结果',icon:ImageIcon},{id:'summary',label:'运行概览',icon:FileText},{id:'logs',label:'调用记录',icon:List}] as const).map(item=><button key={item.id} onClick={()=>setSection(item.id)} aria-current={section===item.id?'page':undefined} className={`flex items-center gap-2 rounded-md px-3 py-2 text-left text-sm ${section===item.id?'bg-muted font-medium':'text-muted-foreground hover:bg-muted/60'}`}><item.icon className="size-4"/>{item.label}</button>)}
   </nav>
   <div className="min-w-0 space-y-5">
    {section==='result' && <>
     {selectionActive && <div className="space-y-2 rounded-lg border border-primary/20 bg-primary/5 p-4 text-sm">
      <p className="font-medium">{selection.valid?`查询结果：保留 ${detections.length} / ${allDetections.length} 个检测候选`:'筛选链接包含失效目标，无法展示该选择。'}</p>
      {selection.reason && <p>{selection.reason}</p>}
      <button className="text-primary underline underline-offset-2" onClick={()=>setShowAll(true)}>查看全部检测目标</button>
     </div>}
     {isDetection && run.status==='succeeded' && <div className="flex flex-wrap items-center justify-between gap-3 text-sm"><p className="text-muted-foreground">检测与筛选结果需要人工核查，目标框为原图像素坐标。</p><button disabled={selectionActive&&!selection.valid} className="text-primary underline underline-offset-2 disabled:opacity-50" onClick={downloadReview}>导出待核查清单</button></div>}
     <div className="flex flex-wrap items-center justify-between gap-3"><h2 className="font-semibold">{isDetection?'识别结果':'算法结果'}{isDetection && <Badge variant="secondary" className="ml-2">{detections.length} 个目标</Badge>}</h2>{isDetection && <label className="flex items-center gap-2 text-sm text-muted-foreground"><input type="checkbox" checked={showBoxes} onChange={e=>setShowBoxes(e.target.checked)}/>显示目标框</label>}</div>
     {active ? <p className="py-12 text-center text-sm text-muted-foreground">{runStatus(run.status)}，结果将自动更新。</p> : <div className={`grid items-start gap-4 ${isDetection ? 'xl:grid-cols-[minmax(0,1fr)_minmax(260px,0.7fr)]' : ''}`}>
      {view.input.mimeType?.startsWith('image/') && <div className="rounded-lg border bg-muted/20 p-4"><AlgorithmAssetPreview key={run.inputAssetId} projectId={projectId} assetId={run.inputAssetId} runId={run.status==='succeeded'?run.id:undefined} detections={showBoxes?detections:[]}/><p className="mt-3 text-center text-xs text-muted-foreground">{asset?assetName(asset):`素材 #${run.inputAssetId}`}</p></div>}
      {isDetection ? <div className="overflow-hidden rounded-lg border"><Table><TableHeader><TableRow><TableHead className="w-20">目标 ID</TableHead><TableHead>类别</TableHead><TableHead className="text-right">置信度</TableHead></TableRow></TableHeader><TableBody>{detections.map((d,i)=><TableRow key={d.detectionKey||i}><TableCell className="text-muted-foreground">{d.detectionKey??i+1}</TableCell><TableCell>{d.label}</TableCell><TableCell className="text-right tabular-nums">{(d.confidence*100).toFixed(1)}%</TableCell></TableRow>)}{!detections.length && <TableRow><TableCell colSpan={3} className="py-8 text-center text-muted-foreground">{selectionActive?'本次筛选未匹配目标':run.status==='succeeded'?'未识别到目标':'暂无识别结果'}</TableCell></TableRow>}</TableBody></Table></div> : <pre className="max-h-96 overflow-auto rounded-md border bg-muted/30 p-4 text-xs">{JSON.stringify(run.canonicalResult.result??run.canonicalResult,null,2)}</pre>}
     </div>}
     {!!view.diagnostics.length && <div className="text-sm text-destructive">{view.diagnostics.map(d=><p key={d}>{d}</p>)}</div>}
     <details className="rounded-lg border p-4 text-sm"><summary className="cursor-pointer text-muted-foreground">标准化结果 JSON</summary><pre className="mt-3 max-h-96 overflow-auto text-xs">{JSON.stringify(run.canonicalResult,null,2)}</pre></details>
    </>}
    {section==='summary' && <>
     <h2 className="font-semibold">运行概览</h2>
     <dl className="grid grid-cols-[100px_minmax(0,1fr)] gap-x-5 gap-y-4 border-y py-5 text-sm">
      <dt className="text-muted-foreground">输入素材</dt><dd>{asset?assetName(asset):`素材 #${run.inputAssetId}`} · v{view.input.assetVersion??'—'}</dd>
      <dt className="text-muted-foreground">算法服务</dt><dd>{run.providerName}</dd>
      <dt className="text-muted-foreground">模型</dt><dd>{view.provenance.modelOrProcess??'—'}</dd>
      <dt className="text-muted-foreground">模型版本</dt><dd>{view.provenance.modelRevision??'—'}</dd>
      <dt className="text-muted-foreground">开始时间</dt><dd>{run.startedAt?new Date(run.startedAt).toLocaleString('zh-CN'):'—'}</dd>
      <dt className="text-muted-foreground">结束时间</dt><dd>{run.finishedAt?new Date(run.finishedAt).toLocaleString('zh-CN'):'—'}</dd>
     </dl>
     <h3 className="text-sm font-medium">运行参数</h3><pre className="overflow-auto rounded-md border bg-muted/30 p-4 text-xs">{JSON.stringify(view.input.parameters,null,2)}</pre>
     {asset?.sourceDescription && <p className="text-sm text-muted-foreground">{asset.sourceDescription}</p>}
     <details className="rounded-lg border p-4 text-sm"><summary className="cursor-pointer text-muted-foreground">结果溯源</summary><pre className="mt-3 overflow-auto text-xs">{JSON.stringify({provenance:view.provenance,rawResult:view.rawResult},null,2)}</pre></details>
    </>}
    {section==='logs' && <><h2 className="font-semibold">调用记录</h2><div className="overflow-hidden rounded-lg border"><Table><TableHeader><TableRow><TableHead>尝试</TableHead><TableHead>状态</TableHead><TableHead>HTTP</TableHead><TableHead>耗时</TableHead><TableHead>错误</TableHead></TableRow></TableHeader><TableBody>{attempts.map(a=><TableRow key={String(a.attempt)}><TableCell>#{String(a.attempt)}</TableCell><TableCell>{runStatus(String(a.status))}</TableCell><TableCell>{String(a.responseStatus??'—')}</TableCell><TableCell>{formatDuration(typeof a.durationMs==='number'?a.durationMs:null)}</TableCell><TableCell className="text-muted-foreground">{String(a.errorCategory??'—')}</TableCell></TableRow>)}{!attempts.length && <TableRow><TableCell colSpan={5} className="py-8 text-center text-muted-foreground">尚无调用记录</TableCell></TableRow>}</TableBody></Table></div></>}
   </div>
  </div>
 </Page>;
}
export default function DetailPage() {
 return <StaticAPIPage<AlgorithmRunDetail> endpoint={query=>{const pid=positiveParam(query);const id=uuidParam(query,'runId');return pid&&id?`/api/projects/${pid}/algorithm-runs/${id}`:null;}}>{(model,query)=><RunDetail key={`${model.run.id}:${query.toString()}`} initial={model} projectId={positiveParam(query)!} query={query}/>}</StaticAPIPage>;
}
