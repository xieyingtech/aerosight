"use client";
import {useState} from "react";
import {FilmIcon} from "lucide-react";
import {useAPI} from "@/lib/use-api";
import {assetName, type AlgorithmAsset} from "@/lib/algorithm-workspace";
import {Button} from "@/components/ui/button";
import {Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription} from "@/components/ui/dialog";
import {OfflineVideoAlgorithm} from "@/components/offline-video-algorithm";

export function AlgorithmVideoAnalysis({projectId, canRun}: {projectId: number; canRun: boolean}) {
  const [open,setOpen] = useState(false);
  const [selected,setSelected] = useState("");
  const assets = useAPI<AlgorithmAsset[]>(open ? `/api/projects/${projectId}/assets` : null);
  const videos = assets.data?.filter(a=>a.mimeType?.startsWith("video/")) ?? [];
  const asset = videos.find(a=>String(a.id) === selected);
  return <><Button variant="outline" disabled={!canRun} onClick={()=>setOpen(true)}><FilmIcon/>视频分析</Button><Dialog open={open} onOpenChange={setOpen}><DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl"><DialogHeader><DialogTitle>离线视频分析</DialogTitle><DialogDescription>选择素材库中的视频，按时间间隔抽帧运行算法并查看结果。关闭窗口会停止继续抽帧。</DialogDescription></DialogHeader>
    <label className="space-y-2"><span className="block text-sm font-medium">输入视频</span><select aria-label="输入视频" value={selected} onChange={e=>setSelected(e.target.value)} className="h-9 w-full rounded-lg border bg-background px-3 text-sm"><option value="">选择视频素材…</option>{videos.map(a=><option key={a.id} value={a.id}>{assetName(a)}</option>)}</select></label>
    {assets.error && <p role="alert" className="text-destructive">视频素材读取失败 <button onClick={assets.reload}>重试</button></p>}{assets.loading && <p className="text-sm text-muted-foreground">加载视频素材…</p>}{!assets.loading && !assets.error && !videos.length && <p className="text-sm text-muted-foreground">暂无视频，请先在素材库导入 MP4 视频。</p>}
    {asset && <OfflineVideoAlgorithm key={asset.id} projectId={projectId} asset={asset}/>}
  </DialogContent></Dialog></>;
}
