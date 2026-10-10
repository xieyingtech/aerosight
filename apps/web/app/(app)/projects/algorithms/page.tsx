"use client";
import Link from 'next/link';
import {useState} from 'react';
import {CheckCircle2Icon, CircleDashedIcon, XCircleIcon, WorkflowIcon, RefreshCwIcon} from 'lucide-react';
import {canonicalPageHref} from '@/lib/page-routes';
import {AlgorithmCatalog} from '@/components/algorithm-catalog';
import {Page} from '@/components/page';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Table,TableHeader,TableBody,TableRow,TableHead,TableCell} from '@/components/ui/table';
import {positiveParam,StaticAPIPage} from '@/components/static-api-page';
import {APIStateView} from '@/components/api-state';
import {useAPI} from '@/lib/use-api';
import {assetName,runStatus,runDetections,type AlgorithmAsset} from '@/lib/algorithm-workspace';
import {effectiveProjectPermissions,type ProjectTeamRole} from '@/lib/project-permission-policy';
import type {AlgorithmCatalogEntry,AlgorithmRunView} from '@/lib/web-api-types';
type Project={id:number;role:ProjectTeamRole;permissions:string[]};
export default function AlgorithmsPage(){return <StaticAPIPage<Project> endpoint={q=>{const id=positiveParam(q);return id?`/api/projects/${id}`:null;}}>{p=><Workspace key={p.id} project={p}/>}</StaticAPIPage>;}
function Workspace({project}:{project:Project}){
 const pid=project.id;const canRun=effectiveProjectPermissions(project.role,project.permissions).has('algorithm:manage');
 const runsState=useAPI<AlgorithmRunView[]>(`/api/projects/${pid}/algorithm-runs`);const catalogState=useAPI<{definitions:AlgorithmCatalogEntry[]}>(`/api/projects/${pid}/algorithm-definitions`);const assetsState=useAPI<AlgorithmAsset[]>(`/api/projects/${pid}/assets`);
 const [algorithm,setAlgorithm]=useState<string|null>(null);const [query,setQuery]=useState('');const [status,setStatus]=useState('all');
 return <APIStateView state={runsState}>{runs=><APIStateView state={catalogState}>{catalog=>{
 const filtered=runs.filter(r=>(!algorithm||r.definitionName===algorithm)&&(status==='all'||r.status===status)&&[r.definitionName,r.providerName,assetsState.data?.find(a=>a.id===r.inputAssetId)?.name??'',r.id].join(' ').toLowerCase().includes(query.toLowerCase()));
 return <Page title="算法" description="运行视觉算法，查看识别结果和执行记录" actions={<AlgorithmCatalog projectId={pid} entries={catalog.definitions} canRun={canRun}/>}>
 <div className="grid gap-6 lg:grid-cols-[220px_minmax(0,1fr)]"><aside className="space-y-1"><p className="px-3 pb-2 text-xs font-medium text-muted-foreground">算法</p><button onClick={()=>setAlgorithm(null)} className={`flex w-full items-center justify-between rounded-md px-3 py-2 text-sm ${algorithm===null?'bg-muted font-medium':'hover:bg-muted/50'}`}><span className="flex items-center gap-2"><WorkflowIcon className="size-4"/>全部运行</span><span className="text-xs text-muted-foreground">{runs.length}</span></button>
 {catalog.definitions.map(e=><button key={e.id} onClick={()=>setAlgorithm(e.name)} className={`w-full rounded-md px-3 py-2 text-left text-sm ${algorithm===e.name?'bg-muted font-medium':'hover:bg-muted/50'}`}><span className="block truncate">{e.name}</span><span className="block truncate text-xs text-muted-foreground">{e.execution.modelOrProcess}</span></button>)}{!catalog.definitions.length&&<p className="p-3 text-sm text-muted-foreground">尚未配置算法</p>}</aside>
 <div className="min-w-0 space-y-3"><div className="flex flex-wrap items-center gap-2"><Input className="min-w-48 flex-1" aria-label="搜索算法运行" placeholder="搜索算法、服务或素材…" value={query} onChange={e=>setQuery(e.target.value)}/><select aria-label="筛选运行状态" className="h-9 rounded-md border bg-background px-3 text-sm" value={status} onChange={e=>setStatus(e.target.value)}><option value="all">全部状态</option>{['succeeded','running','queued','failed','timed_out','polling','waiting_callback','cancelled'].map(s=><option key={s} value={s}>{runStatus(s)}</option>)}</select><Button variant="outline" size="icon" aria-label="刷新运行" onClick={runsState.reload}><RefreshCwIcon className="size-4"/></Button></div>
 <div className="overflow-hidden rounded-lg border"><div className="flex justify-between border-b bg-muted/30 px-4 py-3 text-sm"><h2 className="font-medium">{algorithm??'全部运行'}</h2><span className="text-muted-foreground">{filtered.length} 次运行</span></div><Table><TableHeader><TableRow><TableHead>运行</TableHead><TableHead>输入素材</TableHead><TableHead>状态</TableHead><TableHead>结果</TableHead><TableHead>创建时间</TableHead></TableRow></TableHeader><TableBody>{filtered.map(r=>{const a=assetsState.data?.find(a=>a.id===r.inputAssetId);const failed=['failed','timed_out'].includes(r.status);const Icon=r.status==='succeeded'?CheckCircle2Icon:failed?XCircleIcon:CircleDashedIcon;return <TableRow key={r.id}><TableCell><Link href={canonicalPageHref(`/projects/algorithms/runs/detail/?projectId=${pid}&runId=${r.id}`)} className="flex items-center gap-3"><Icon className={`size-4 shrink-0 ${r.status==='succeeded'?'text-green-600':failed?'text-destructive':'text-muted-foreground'}`}/><span><span className="block font-medium hover:underline">{r.definitionName}</span><span className="block text-xs text-muted-foreground"><span className="inline-flex max-w-full flex-wrap items-center gap-x-3 gap-y-1"><span>{r.providerName}</span><span>{r.id.slice(0,8)}</span></span></span></span></Link></TableCell><TableCell className="max-w-48 truncate">{a?assetName(a):`素材 #${r.inputAssetId}`}</TableCell><TableCell>{runStatus(r.status)}</TableCell><TableCell>{r.canonicalResult.kind==='video'?`${(r.canonicalResult.result as Record<string,unknown>)?.processedFrames??0} 帧`:r.status==='succeeded'&&r.canonicalResult.kind==='detection'?`${runDetections(r.canonicalResult).length} 个目标`:'—'}</TableCell><TableCell className="whitespace-nowrap text-xs text-muted-foreground">{new Date(r.createdAt).toLocaleString('zh-CN')}</TableCell></TableRow>;})}{!filtered.length&&<TableRow><TableCell colSpan={5} className="h-32 text-center text-muted-foreground">{runs.length?'没有匹配的运行':'暂无运行，选择算法和素材开始识别'}</TableCell></TableRow>}</TableBody></Table></div>
 </div></div></Page>;
 }}</APIStateView>}</APIStateView>;
}
