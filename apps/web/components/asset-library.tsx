"use client";

import {useRef, useState} from "react";
import {DownloadIcon, FileIcon, FilmIcon, ImageIcon, Loader2Icon, Maximize2Icon, PlusIcon, RefreshCwIcon, SearchIcon, UploadCloudIcon, XIcon} from "lucide-react";
import {apiJSON} from "@/lib/api-client";
import {useAPI} from "@/lib/use-api";
import {useMediaPlayback} from "@/lib/use-media-playback";
import {assetName, type AlgorithmAsset} from "@/lib/algorithm-workspace";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter} from "@/components/ui/dialog";

type MediaType = "all" | "video" | "image";
function mediaType(asset: AlgorithmAsset) {return asset.mimeType?.startsWith("video/") ? "video" : asset.mimeType?.startsWith("image/") ? "image" : "file";}
function date(value: string | null) {return value ? new Date(value).toLocaleString("zh-CN", {hour12: false}) : "未记录";}
function MediaIcon({asset, className}: {asset: AlgorithmAsset; className?: string}) {const Icon = mediaType(asset) === "video" ? FilmIcon : mediaType(asset) === "image" ? ImageIcon : FileIcon; return <Icon className={className}/>;}

function AssetViewer({projectId, asset}: {projectId: number; asset: AlgorithmAsset}) {
  const kind = mediaType(asset);
  const video = useRef<HTMLVideoElement>(null);
  const access = useMediaPlayback(kind === "file" ? null : `/api/projects/${projectId}/assets/${asset.id}/access?action=${kind === "video" ? "play" : "preview"}`,video);
  const [failed, setFailed] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [downloadError, setDownloadError] = useState<string | null>(null);
  const [dimensions, setDimensions] = useState<string | null>(null);
  const [duration, setDuration] = useState<string | null>(null);
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
        : kind === "video" ? <video ref={video} key={access.data?.url} src={access.data?.url} controls playsInline preload="metadata" className="h-full w-full" onError={()=>{if(!access.recover())setFailed(true);}} onLoadedMetadata={e=>{access.loaded();const v=e.currentTarget; setDimensions(`${v.videoWidth} × ${v.videoHeight}`); if(Number.isFinite(v.duration)) setDuration(`${v.duration.toFixed(1)} 秒`);}}/>
        : kind === "image" ? image : <div className="space-y-3 text-center text-slate-400"><FileIcon className="mx-auto size-10"/><p className="text-sm">此文件暂不支持在线预览，可以下载查看</p></div>}
    </div>
    <div className="space-y-4 px-5 py-4">
      {downloadError && <p role="alert" className="text-sm text-destructive">{downloadError}</p>}
      <dl className="grid grid-cols-2 gap-x-6 gap-y-4 text-sm xl:grid-cols-4">{[["拍摄时间",date(asset.capturedAt)],["导入时间",date(asset.createdAt)],["分辨率",dimensions ?? "未记录"],[kind === "video" ? "时长" : "格式",kind === "video" ? duration ?? "未记录" : asset.mimeType?.split("/")[1]?.toUpperCase() ?? "未记录"]].map(([label,value])=><div key={label}><dt className="mb-1 text-xs text-muted-foreground">{label}</dt><dd className="break-words text-xs leading-5">{value}</dd></div>)}</dl>
      <div className="border-t pt-3"><p className="mb-1 text-xs text-muted-foreground">来源说明</p><p className="whitespace-pre-wrap break-words text-sm leading-6">{asset.sourceDescription || "暂无来源说明"}</p></div>
    </div>
    <Dialog open={expanded} onOpenChange={setExpanded}><DialogContent className="sm:max-w-[90vw]"><DialogHeader><DialogTitle>{assetName(asset)}</DialogTitle><DialogDescription>原图预览</DialogDescription></DialogHeader><div className="h-[75vh] bg-slate-950 p-3">{image}</div></DialogContent></Dialog>
  </section>;
}

export function AssetLibrary({projectId}: {projectId: number}) {
  const assets = useAPI<AlgorithmAsset[]>(`/api/projects/${projectId}/assets`);
  const [selected, setSelected] = useState<number | null>(null);
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<MediaType>("all");
  const [open, setOpen] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [dragging, setDragging] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const list = (assets.data ?? []).filter(a=>(filter === "all" || mediaType(a) === filter) && `${assetName(a)} ${a.sourceDescription ?? ""} ${a.id}`.toLowerCase().includes(query.toLowerCase()));
  const asset = list.find(a=>a.id === selected) ?? list[0];
  function chooseFile(value: File | null) {
    setError(null); setFile(null);
    if (!value) return;
    if(!["image/jpeg","image/png","video/mp4"].includes(value.type)) {setError("请选择 JPG、PNG 图片或 MP4 视频"); return;}
    if(value.size > (value.type === "video/mp4" ? 512 : 40)*1024*1024) {setError(value.type === "video/mp4" ? "视频不能超过 512 MB" : "图片不能超过 40 MB"); return;}
    setFile(value);
  }
  async function upload(form: FormData) {
    if(!file || uploading) return;
    setUploading(true); setError(null); form.set("file", file);
    try {const result = await apiJSON<{assetId: number}>(`/api/projects/${projectId}/assets/import`, {method: "POST", body: form}); setSelected(result.assetId); setQuery(""); setFilter("all"); assets.reload(); setOpen(false); setFile(null);}
    catch(e) {setError(e instanceof Error ? e.message : "上传失败，请重试");}
    finally {setUploading(false);}
  }
  return <div className="grid items-start gap-5 lg:grid-cols-[280px_minmax(0,1fr)]">
    <aside className="overflow-hidden rounded-xl border bg-background">
      <div className="space-y-3 border-b p-4"><div className="flex items-center justify-between"><h2 className="text-sm font-medium">项目素材 <span className="ml-1 text-xs font-normal text-muted-foreground">{assets.data?.length ?? 0}</span></h2><div className="flex gap-1"><Button variant="ghost" size="icon-sm" aria-label="刷新素材" title="刷新" onClick={assets.reload}><RefreshCwIcon className={assets.loading ? "animate-spin" : ""}/></Button><Button size="icon-sm" aria-label="导入素材" title="导入素材" onClick={()=>{setError(null);setFile(null);setOpen(true);}}><PlusIcon/></Button></div></div>
        <div className="relative"><SearchIcon className="pointer-events-none absolute left-2.5 top-2.5 size-4 text-muted-foreground"/><Input aria-label="搜索素材" placeholder="搜索名称或来源…" className="pl-9" value={query} onChange={e=>setQuery(e.target.value)}/></div>
        <div className="flex gap-1 rounded-lg bg-muted/60 p-1">{([["all","全部"],["video","视频"],["image","图片"]] as const).map(([value,label])=><button key={value} aria-pressed={filter === value} onClick={()=>setFilter(value)} className={`flex-1 rounded-md px-2 py-1.5 text-xs transition-colors focus-visible:outline-ring ${filter === value ? "bg-background font-medium shadow-sm" : "text-muted-foreground hover:text-foreground"}`}>{label}</button>)}</div>
      </div>
      <div className="max-h-[540px] space-y-1 overflow-y-auto p-2 lg:max-h-[620px]">
        {assets.error ? <p role="alert" className="p-4 text-sm text-destructive">素材读取失败，请刷新重试</p> : assets.loading ? <div className="flex items-center justify-center gap-2 p-8 text-xs text-muted-foreground"><Loader2Icon className="size-4 animate-spin"/>加载素材…</div> : list.map(a=><button key={a.id} aria-pressed={asset?.id === a.id} onClick={()=>setSelected(a.id)} className={`flex w-full items-start gap-3 rounded-lg p-3 text-left transition-colors focus-visible:outline-ring ${asset?.id === a.id ? "bg-primary/10 ring-1 ring-inset ring-primary/20" : "hover:bg-muted/60"}`}><span className={`flex size-9 shrink-0 items-center justify-center rounded-lg ${mediaType(a) === "video" ? "bg-blue-500/10 text-blue-600" : "bg-muted text-muted-foreground"}`}><MediaIcon asset={a} className="size-4"/></span><span className="min-w-0 flex-1"><span className="block truncate text-xs font-medium" title={assetName(a)}>{assetName(a)}</span><span className="mt-1 block truncate text-[11px] text-muted-foreground">{date(a.capturedAt ?? a.createdAt)}</span>{a.sourceDescription && <span className="mt-1 block truncate text-[11px] text-muted-foreground" title={a.sourceDescription}>{a.sourceDescription}</span>}</span></button>)}
        {!assets.loading && !assets.error && !list.length && <div className="space-y-2 px-3 py-10 text-center"><ImageIcon className="mx-auto size-7 text-muted-foreground/50"/><p className="text-sm text-muted-foreground">{assets.data?.length ? "没有匹配的素材" : "还没有素材"}</p><p className="text-xs text-muted-foreground">{assets.data?.length ? "试试其他关键词或分类" : "点击上方加号导入照片或视频"}</p></div>}
      </div>
      <div className="border-t px-4 py-2.5 text-xs text-muted-foreground">{list.length} 个素材{filter !== "all" || query ? " · 已筛选" : ""}</div>
    </aside>
    {asset ? <AssetViewer key={asset.id} projectId={projectId} asset={asset}/> : <div className="flex min-h-[460px] flex-col items-center justify-center gap-3 rounded-xl border border-dashed bg-muted/10 p-8 text-center"><ImageIcon className="size-10 text-muted-foreground/40"/><p className="text-sm text-muted-foreground">{assets.loading ? "正在加载素材" : "选择素材，查看照片与视频"}</p></div>}
    <Dialog open={open} onOpenChange={value=>{if(!uploading) setOpen(value);}}><DialogContent className="sm:max-w-lg" showCloseButton={!uploading}><DialogHeader><DialogTitle>导入素材</DialogTitle><DialogDescription>将照片或视频添加到项目素材库。</DialogDescription></DialogHeader>
      <form action={upload} className="space-y-5">
        <input ref={fileInput} type="file" aria-label="选择照片或视频" accept="image/jpeg,image/png,video/mp4" className="sr-only" tabIndex={-1} disabled={uploading} onChange={e=>chooseFile(e.target.files?.[0] ?? null)}/>
        <div onDragOver={e=>{e.preventDefault();if(!uploading) setDragging(true);}} onDragLeave={()=>setDragging(false)} onDrop={e=>{e.preventDefault();setDragging(false);if(!uploading) chooseFile(e.dataTransfer.files[0] ?? null);}} className={`rounded-xl border border-dashed p-5 transition-colors ${dragging ? "border-primary bg-primary/5" : "bg-muted/20"}`}>
          {file ? <div className="flex items-center gap-3"><div className="flex size-11 shrink-0 items-center justify-center rounded-lg bg-background">{file.type === "video/mp4" ? <FilmIcon className="size-5 text-blue-600"/> : <ImageIcon className="size-5 text-primary"/>}</div><div className="min-w-0 flex-1"><p className="truncate text-sm font-medium">{file.name}</p><p className="mt-1 text-xs text-muted-foreground">{(file.size/1024/1024).toFixed(2)} MB · 准备导入</p></div><Button type="button" variant="ghost" size="icon" aria-label="移除已选文件" disabled={uploading} onClick={()=>{setFile(null);if(fileInput.current) fileInput.current.value="";}}><XIcon/></Button></div>
            : <button type="button" disabled={uploading} className="flex w-full flex-col items-center gap-3 py-4 text-center focus-visible:outline-ring" onClick={()=>{if(fileInput.current) {fileInput.current.value="";fileInput.current.click();}}}><span className="flex size-12 items-center justify-center rounded-xl border bg-background"><UploadCloudIcon className="size-6 text-primary"/></span><span className="text-sm font-medium">拖放文件到这里，或点击选择</span><span className="text-xs text-muted-foreground">JPG / PNG 最大 40 MB · MP4 最大 512 MB</span></button>}
        </div>
        <label className="block space-y-2"><span className="text-sm font-medium">来源说明 <span className="font-normal text-muted-foreground">（选填）</span></span><Input name="sourceDescription" placeholder="例如：司空媒体库 · 园区巡检" disabled={uploading}/></label>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        {uploading && <p role="status" className="text-xs text-muted-foreground">正在上传，请保持窗口打开…</p>}
        <DialogFooter><Button type="button" variant="outline" disabled={uploading} onClick={()=>setOpen(false)}>取消</Button><Button type="submit" disabled={!file || uploading}>{uploading ? <Loader2Icon className="animate-spin"/> : <PlusIcon/>}{uploading ? "正在导入…" : "导入素材"}</Button></DialogFooter>
      </form>
    </DialogContent></Dialog>
  </div>;
}
