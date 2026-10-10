"use client";
import {useEffect,useRef,useState} from "react";
import {DownloadIcon, FileIcon, Loader2Icon, Maximize2Icon, RefreshCwIcon} from "lucide-react";
import {MediaIndexStatus} from "@/components/media-semantic-panel";
import {apiJSON} from "@/lib/api-client";
import {useMediaPlayback} from "@/lib/use-media-playback";
import {assetName,type AlgorithmAsset} from "@/lib/algorithm-workspace";
import {mediaType} from "@/lib/material-search";
import {Button} from "@/components/ui/button";
import {Dialog,DialogContent,DialogHeader,DialogTitle,DialogDescription} from "@/components/ui/dialog";
function date(value:string|null){return value?new Date(value).toLocaleString("zh-CN",{hour12:false}):"未记录";}
export function AssetViewer({projectId, asset, startMs = 0, endMs}: {projectId: number; asset: AlgorithmAsset; startMs?: number; endMs?: number}) {
  const kind = mediaType(asset);
  const video = useRef<HTMLVideoElement>(null);
  const access = useMediaPlayback(kind === "file" ? null : `/api/projects/${projectId}/assets/${asset.id}/access?action=${kind === "video" ? "play" : "preview"}`,video);
  const [failed, setFailed] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [downloadError, setDownloadError] = useState<string | null>(null);
  const [dimensions, setDimensions] = useState<string | null>(null);
  const [duration, setDuration] = useState<string | null>(null);
  useEffect(()=>{if(video.current && video.current.readyState >= 1) video.current.currentTime=startMs/1000;},[startMs]);
  async function download() {
    setDownloading(true); setDownloadError(null);
    try {const result = await apiJSON<{url: string}>(`/api/projects/${projectId}/assets/${asset.id}/access?action=download`); const anchor = document.createElement("a"); anchor.href = result.url; anchor.download = assetName(asset); anchor.click();}
    catch {setDownloadError("下载地址获取失败，请重试");}
    finally {setDownloading(false);}
  }
  const image = <img src={access.data?.url} alt={assetName(asset)} onError={()=>setFailed(true)} onLoad={e=>setDimensions(`${e.currentTarget.naturalWidth} × ${e.currentTarget.naturalHeight}`)} className="h-full w-full object-contain"/>;
  return <section className="min-w-0 overflow-hidden rounded-xl border bg-background">
    <header className="flex flex-wrap items-center justify-between gap-3 border-b px-5 py-4">
      <div className="min-w-0 flex-1"><div className="mb-1 flex items-center gap-2 text-xs text-muted-foreground"><span className="rounded bg-muted px-2 py-0.5">{kind === "video" ? "视频" : kind === "image" ? "图片" : "文件"}</span><span>#{asset.id}</span></div><h2 className="break-all text-sm font-medium">{assetName(asset)}</h2></div>
      <div className="flex gap-1">{kind === "image" && access.data && !failed && <Button variant="ghost" size="icon" aria-label="放大查看图片" title="放大查看" onClick={()=>setExpanded(true)}><Maximize2Icon/></Button>}<Button variant="outline" onClick={download} disabled={downloading}>{downloading ? <Loader2Icon className="animate-spin"/> : <DownloadIcon/>}下载</Button></div>
    </header>
    <div className="flex h-[340px] items-center justify-center bg-slate-950 p-3 sm:h-[460px]">
      {access.error || failed ? <div className="space-y-3 text-center text-sm text-slate-300"><p>暂时无法预览此素材</p><Button variant="secondary" onClick={()=>{setFailed(false); access.reload();}}><RefreshCwIcon/>重新加载</Button></div>
        : access.loading ? <Loader2Icon aria-label="加载预览" className="size-6 animate-spin text-slate-400"/>
        : kind === "video" ? <video ref={video} key={access.data?.url} src={access.data?.url} controls playsInline preload="metadata" className="h-full w-full" onError={()=>{if(!access.recover())setFailed(true);}} onLoadedMetadata={e=>{access.loaded();const v=e.currentTarget; setDimensions(`${v.videoWidth} × ${v.videoHeight}`); if(Number.isFinite(v.duration)) setDuration(`${v.duration.toFixed(1)} 秒`);if(startMs>0)v.currentTime=Math.min(startMs/1000,v.duration);}} onTimeUpdate={e=>{if(endMs !== undefined && e.currentTarget.currentTime >= endMs/1000)e.currentTarget.pause();}}/>
        : kind === "image" ? image : <div className="space-y-3 text-center text-slate-400"><FileIcon className="mx-auto size-10"/><p className="text-sm">此文件暂不支持在线预览，可以下载查看</p></div>}
    </div>
    <div className="space-y-4 px-5 py-4">
      <MediaIndexStatus projectId={projectId} assetId={asset.id}/>
      {downloadError && <p role="alert" className="text-sm text-destructive">{downloadError}</p>}
      <dl className="grid grid-cols-2 gap-x-6 gap-y-4 text-sm xl:grid-cols-4">{[["拍摄时间",date(asset.capturedAt)],["导入时间",date(asset.createdAt)],["分辨率",dimensions ?? "未记录"],[kind === "video" ? "时长" : "格式",kind === "video" ? duration ?? "未记录" : asset.mimeType?.split("/")[1]?.toUpperCase() ?? "未记录"]].map(([label,value])=><div key={label}><dt className="mb-1 text-xs text-muted-foreground">{label}</dt><dd className="break-words text-xs leading-5">{value}</dd></div>)}</dl>
      <div className="border-t pt-3"><p className="mb-1 text-xs text-muted-foreground">来源说明</p><p className="whitespace-pre-wrap break-words text-sm leading-6">{asset.sourceDescription || "暂无来源说明"}</p></div>
    </div>
    <Dialog open={expanded} onOpenChange={setExpanded}><DialogContent className="sm:max-w-[90vw]"><DialogHeader><DialogTitle>{assetName(asset)}</DialogTitle><DialogDescription>原图预览</DialogDescription></DialogHeader><div className="h-[75vh] bg-slate-950 p-3">{image}</div></DialogContent></Dialog>
  </section>;
}
