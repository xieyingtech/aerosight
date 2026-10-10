"use client";
import {useState} from 'react';
import {useRouter} from 'next/navigation';
import {PlayIcon} from 'lucide-react';
import {canonicalPageHref} from '@/lib/page-routes';
import {apiJSON,APIError} from '@/lib/api-client';
import {useAPI} from '@/lib/use-api';
import {assetName,eligibleAlgorithmAssets,type AlgorithmAsset} from '@/lib/algorithm-workspace';
import {coerceSchemaParameters} from '@/lib/algorithm-run-input';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {InputSelect} from '@/components/ui/input-select';
import {Dialog,DialogContent,DialogHeader,DialogTitle,DialogDescription} from '@/components/ui/dialog';
import {AlgorithmAssetPreview} from '@/components/algorithm-asset-preview';
type Entry={id:string;configurationSnapshotId:string;name:string;description:string|null;capabilityCode:string;execution:{mode:string;modelOrProcess:string};provider:{type:string;available:boolean};schemas:{parameters:Record<string,unknown>;output:Record<string,unknown>};display:Record<string,unknown>};
export function AlgorithmCatalog({projectId,entries,canRun}:{projectId:number;entries:Entry[];canRun:boolean}){
 const router=useRouter();const [open,setOpen]=useState(false);const [entryId,setEntryId]=useState<string|null>(null);const [assetId,setAssetId]=useState<string|null>(null);const [error,setError]=useState<string|null>(null);const [pending,setPending]=useState(false);
 const assetsState=useAPI<AlgorithmAsset[]>(open?`/api/projects/${projectId}/assets`:null);
 const entry=entries.find(e=>e.id===entryId)??entries[0];const assets=eligibleAlgorithmAssets(assetsState.data??[],entry?.capabilityCode==='detection');const selected=assets.find(a=>String(a.id)===assetId);
 const properties=entry?.schemas.parameters.properties as Record<string,Record<string,unknown>>|undefined;
 const isVideo=selected?.mimeType?.startsWith('video/')??false;
 const [videoFps,setVideoFps]=useState('1');
 async function run(data:FormData){if(!entry||!selected)return;setPending(true);setError(null);try{const fps=Number(videoFps);if(isVideo&&(!Number.isFinite(fps)||fps<0.2||fps>5))throw new Error('分析帧率必须在 0.2–5 FPS 之间');const result=await apiJSON<{runId:string}>(`/api/projects/${projectId}/algorithm-runs`,{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({configurationSnapshotId:Number(entry.configurationSnapshotId),assetId:selected.id,...(isVideo?{videoFps:fps}:{}),parameters:coerceSchemaParameters(entry.schemas.parameters,Object.fromEntries(data.entries()))})});setOpen(false);router.push(canonicalPageHref(`/projects/algorithms/runs/detail/?projectId=${projectId}&runId=${result.runId}`));}catch(e){setError(e instanceof APIError?(e.code==='VIDEO_ANALYSIS_INPUT_UNSUPPORTED'?'视频分析需要同步算法及有效视频时长，采样帧数不能超过 10,000 帧。':e.code):e instanceof Error?e.message:'提交失败，请稍后重试。');}finally{setPending(false);}}
 return <><Button disabled={!canRun||!entries.length} onClick={()=>{setAssetId(null);setError(null);setOpen(true);}}><PlayIcon className="size-4"/>运行算法</Button>
 <Dialog open={open} onOpenChange={v=>{if(!pending)setOpen(v);}}><DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl"><DialogHeader><DialogTitle>运行算法</DialogTitle><DialogDescription>选择算法和输入素材，提交后可查看识别结果与调用记录。</DialogDescription></DialogHeader>
 <form action={run} className="space-y-4"><div className="space-y-2"><label className="text-sm font-medium">算法</label><InputSelect ariaLabel="选择算法" value={entry?.id??null} options={entries.map(e=>({value:e.id,label:e.name,description:e.execution.modelOrProcess}))} onValueChange={v=>{setEntryId(v);setAssetId(null);}} disabled={pending}/>{entry?.description&&<p className="text-xs text-muted-foreground">{entry.description}</p>}</div>
 <div className="space-y-2"><label className="text-sm font-medium">输入素材</label><InputSelect ariaLabel="选择输入素材" value={assetId} options={assets.map(a=>({value:String(a.id),label:assetName(a),description:`${a.mimeType??a.kind}（${new Date(a.capturedAt??a.createdAt).toLocaleDateString('zh-CN')}）`,keywords:[String(a.id),a.sourceDescription??'']}))} onValueChange={setAssetId} placeholder="按文件名搜索并选择素材" disabled={pending||assetsState.loading} emptyMessage="没有可用于此算法的素材"/>
 {assetsState.error&&<p role="alert" className="text-sm text-destructive">素材加载失败。<button type="button" className="ml-2 underline" onClick={assetsState.reload}>重新加载</button></p>}
 {selected&&<div className="space-y-2 rounded-lg border bg-muted/20 p-3">{selected.mimeType?.startsWith('image/')&&<AlgorithmAssetPreview key={selected.id} projectId={projectId} assetId={selected.id} compact/>}<p className="text-center text-xs text-muted-foreground">{assetName(selected)}<span className="inline-block whitespace-pre-line">{selected.sourceDescription?`\n${selected.sourceDescription}`:''}</span></p></div>}
 </div>
 {isVideo&&<label className="block space-y-2 rounded-lg border bg-muted/20 p-3"><span className="text-sm font-medium">分析帧率（FPS）</span><Input aria-label="分析帧率" type="number" min={0.2} max={5} step={0.1} value={videoFps} onChange={e=>setVideoFps(e.target.value)} required disabled={pending}/><span className="block text-xs text-muted-foreground">0.2–5 FPS，默认每秒分析 1 帧，最多 10,000 帧。后台分析完整视频，完成后可同步叠框回放。</span></label>}
 {entry&&properties&&Object.keys(properties).length>0&&<fieldset key={entry.id} className="space-y-3"><legend className="mb-2 text-sm font-medium">运行参数</legend>{Object.entries(properties).map(([key,p])=><label key={key} className="grid gap-1 text-sm"><span>{String(p.title??key)}</span>{p.type==='boolean'?<select name={key} className="h-9 rounded-md border bg-background px-3"><option value="">默认</option><option value="true">是</option><option value="false">否</option></select>:<Input name={key} step={p.type==='number'?'any':undefined} type={p.type==='number'||p.type==='integer'?'number':'text'} placeholder={String(p.description??'使用算法默认值')}/>}</label>)}</fieldset>}
 {error&&<p role="alert" className="text-sm text-destructive">{error}</p>}{!entry?.provider.available&&<p className="text-sm text-muted-foreground">算法服务当前不可用。</p>}
 <div className="flex justify-end gap-2 border-t pt-4"><Button type="button" variant="outline" disabled={pending} onClick={()=>setOpen(false)}>取消</Button><Button type="submit" disabled={pending||!selected||!entry?.provider.available}>{pending?'正在提交…':'开始运行'}</Button></div>
 </form></DialogContent></Dialog></>;
}
