"use client";
import {useEffect,useRef,useState} from 'react';
import {useAPI} from '@/lib/use-api';
import {useMediaPlayback} from '@/lib/use-media-playback';
import {validBox} from '@/lib/algorithm-workspace';
import {videoFrameAt,type VideoAnnotationFrame} from '@/lib/video-annotations';
import {Button} from '@/components/ui/button';
import {DownloadIcon,RefreshCwIcon} from 'lucide-react';

export function AlgorithmVideoResult({projectId,runId,assetId,status,summary}:{projectId:number;runId:string;assetId:number;status:string;summary:Record<string,unknown>}){
 const video=useRef<HTMLVideoElement>(null);
 const access=useMediaPlayback(`/api/projects/${projectId}/assets/${assetId}/access?action=play`,video);
 const annotations=useAPI<{frames:VideoAnnotationFrame[]}>(status==='succeeded'?`/api/projects/${projectId}/algorithm-runs/${runId}/annotations`:null);
 const [current,setCurrent]=useState<VideoAnnotationFrame>();const [boxes,setBoxes]=useState(true);const [failed,setFailed]=useState(false);
 const frames=annotations.data?.frames;
 useEffect(()=>{if(!frames)return;let timer:number;const update=()=>{const frame=videoFrameAt(frames,(video.current?.currentTime??0)*1000);setCurrent(previous=>previous?.index===frame?.index?previous:frame);timer=requestAnimationFrame(update);};timer=requestAnimationFrame(update);return()=>cancelAnimationFrame(timer);},[frames]);
 const detections=current?.result.detections??[];
 const count=Number(summary.processedFrames??0),total=Number(summary.totalFrames??0);
 const visibleFrames=frames?.slice(Math.max(0,(current?.index??0)-20),Math.max(60,(current?.index??0)+40));
 return <section className="space-y-4">
  <div className="flex flex-wrap items-center justify-between gap-3"><div><h2 className="font-semibold">视频分析结果</h2><p className="mt-1 text-xs text-muted-foreground"><span className="inline-flex max-w-full flex-wrap items-center gap-x-3 gap-y-1"><span>{String(summary.fps??1)} FPS</span><span className="rounded-md bg-muted px-2 py-0.5 text-xs">{count} / {total||'—'} 帧<span className="inline-block whitespace-pre-line">{status==='succeeded'?'\n完整分析已完成':['failed','timed_out'].includes(status)?'\n分析未完成':status==='queued'?'\n等待后台分析':'\n后台分析中'}</span></span></span></p></div><div className="flex items-center gap-3"><label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={boxes} onChange={e=>setBoxes(e.target.checked)}/>显示目标框</label>{status==='succeeded'&&<Button variant="outline" asChild><a href={`/api/projects/${projectId}/algorithm-runs/${runId}/annotations?format=jsonl`} download><DownloadIcon/>导出标注 JSONL</a></Button>}</div></div>
  {total>0&&status!=='succeeded'&&<progress aria-label="视频分析进度" value={count} max={total} className="h-2 w-full"/>}
  <div className="relative flex h-[400px] items-center justify-center overflow-hidden rounded-lg bg-slate-950">
   {access.error||failed?<div className="space-y-3 text-center text-sm text-slate-300"><p>视频暂时无法加载</p><Button variant="secondary" onClick={()=>{setFailed(false);access.reload();}}><RefreshCwIcon/>重新加载</Button></div>:<video ref={video} key={access.data?.url} src={access.data?.url} controls playsInline preload="metadata" onLoadedMetadata={access.loaded} onError={()=>{if(!access.recover())setFailed(true);}} className="h-full w-full"/>}
   {boxes&&current&&!failed&&<svg aria-label="视频动态目标框" className="pointer-events-none absolute inset-0 h-full w-full" viewBox={`0 0 ${current.width} ${current.height}`} preserveAspectRatio="xMidYMid meet">{detections.filter(validBox).map((d,index)=>{const b=d.pixelGeometry!;return <g key={`${d.detectionKey}-${index}`}><rect x={b.x} y={b.y} width={b.width} height={b.height} fill="none" stroke="#22c55e" strokeWidth="2" vectorEffect="non-scaling-stroke"/><text x={b.x+4} y={Math.max(20,b.y-6)} fontSize={current.width/45} fill="#22c55e" stroke="#052e16" strokeWidth="2" paintOrder="stroke">{d.label} {(d.confidence*100).toFixed(0)}%</text></g>;})}</svg>}
  </div>
  {annotations.error&&<p role="alert" className="text-sm text-destructive">标注加载失败 <button onClick={annotations.reload} className="underline">重试</button></p>}
  {status==='succeeded'&&!annotations.data&&!annotations.error&&<p className="text-sm text-muted-foreground">正在加载视频时间轴标注…</p>}
  {current&&<div className="space-y-2 rounded-lg border p-4"><p className="text-sm"><span className="inline-flex max-w-full flex-wrap items-center gap-x-3 gap-y-1"><span>当前标注：{(current.timeMs/1000).toFixed(2)} 秒</span><span>{detections.length} 个目标</span></span></p><p className="text-xs text-muted-foreground"><span className="inline-block whitespace-pre-line">{detections.map(d=>`${d.label} ${(d.confidence*100).toFixed(0)}%`).join('\n')||'此采样帧没有检测目标'}</span></p>{current.result.kind!=='detection'&&<pre className="max-h-48 overflow-auto text-xs">{JSON.stringify(current.result,null,2)}</pre>}</div>}
  {!!frames?.length&&<div className="flex max-h-28 flex-wrap gap-1 overflow-auto">{visibleFrames?.map(f=><button key={f.index} onClick={()=>{if(video.current)video.current.currentTime=f.timeMs/1000;}} aria-pressed={current?.index===f.index} className={`rounded border px-2 py-1 text-xs ${current?.index===f.index?'bg-primary text-primary-foreground':''}`}>{(f.timeMs/1000).toFixed(1)} 秒</button>)}</div>}
  <p className="text-xs text-muted-foreground">标注与原视频分别保存。目标框按采样时间更新，并持续显示至下一采样点；未分析的帧不代表已完成逐帧检测，也未进行跨帧目标身份追踪。</p>
 </section>;
}
