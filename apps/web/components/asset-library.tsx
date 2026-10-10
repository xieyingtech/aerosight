"use client";

import {useEffect, useRef, useState} from "react";
import Link from "next/link";
import {useRouter} from "next/navigation";
import {materialRows, mediaType, type MediaType, type MediaMatch} from "@/lib/material-search";
import {projectPageHref} from "@/lib/page-routes";
import {FileIcon, FilmIcon, ImageIcon, Loader2Icon, PlusIcon, RefreshCwIcon, SearchIcon, UploadCloudIcon, XIcon} from "lucide-react";
import {apiJSON} from "@/lib/api-client";
import {useAPI} from "@/lib/use-api";
import {assetName, type AlgorithmAsset} from "@/lib/algorithm-workspace";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter} from "@/components/ui/dialog";

function date(value: string | null) {return value ? new Date(value).toLocaleString("zh-CN", {hour12: false}) : "未记录";}
function MediaIcon({asset, className}: {asset: AlgorithmAsset; className?: string}) {const Icon = mediaType(asset) === "video" ? FilmIcon : mediaType(asset) === "image" ? ImageIcon : FileIcon; return <Icon className={className}/>;}

export function AssetLibrary({projectId,initialQuery="",initialFilter="all"}: {projectId:number;initialQuery?:string;initialFilter?:MediaType}) {
  const assets = useAPI<AlgorithmAsset[]>(`/api/projects/${projectId}/assets`);
  const router=useRouter();
  const [query,setQuery]=useState(initialQuery);
  const [matches,setMatches]=useState<MediaMatch[]>([]);
  const [busy,setBusy]=useState(false);
  const [searchError,setSearchError]=useState<string|null>(null);
  const [searchAttempt,setSearchAttempt]=useState(0);
  const generation=useRef(0);
  useEffect(()=>{
    const current=++generation.current;
    const controller=new AbortController();
    setMatches([]);setSearchError(null);
    const needle=query.trim();
    setBusy(!!needle);
    if(!needle)return ()=>controller.abort();
    const timer=setTimeout(async()=>{
      try{const result=await apiJSON<{items:MediaMatch[]}>(`/api/projects/${projectId}/media-search`,{method:"POST",body:JSON.stringify({query:needle,limit:20}),signal:controller.signal});if(generation.current === current)setMatches(result.items);}
      catch{if(!controller.signal.aborted && generation.current === current)setSearchError("内容搜索暂不可用，仍可按名称和来源查找素材。");}
      finally{if(!controller.signal.aborted && generation.current === current)setBusy(false);}
    },searchAttempt?0:350);
    return ()=>{clearTimeout(timer);controller.abort();};
  },[projectId,query,searchAttempt]);
  const [filter, setFilter] = useState<MediaType>(initialFilter);
  const [open, setOpen] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [dragging, setDragging] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const rows=materialRows(assets.data ?? [],query,filter,matches);
  function detailHref(assetId:number,segment?:MediaMatch){return projectPageHref(projectId,"assets/detail",{assetId,...(query?{q:query}:{}),...(filter !== "all"?{type:filter}:{}),...(segment?{startMs:segment.startMs,endMs:segment.endMs}:{})});}
  useEffect(()=>{const url=new URL(window.location.href);if(query)url.searchParams.set("q",query);else url.searchParams.delete("q");if(filter !== "all")url.searchParams.set("type",filter);else url.searchParams.delete("type");window.history.replaceState(window.history.state,"",url.pathname+url.search);},[query,filter]);
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
    try {await apiJSON<{assetId: number}>(`/api/projects/${projectId}/assets/import`, {method: "POST", body: form}); setQuery(""); setFilter("all"); assets.reload(); setOpen(false); setFile(null);}
    catch(e) {setError(e instanceof Error ? e.message : "上传失败，请重试");}
    finally {setUploading(false);}
  }
  return <div className="space-y-4">
    <form onSubmit={e=>{e.preventDefault();setSearchAttempt(v=>v+1);}} className="flex w-full items-center gap-2">
      <div className="relative min-w-0 flex-1"><SearchIcon className="pointer-events-none absolute left-3 top-3 size-4 text-muted-foreground"/><Input aria-label="搜索素材" placeholder="搜索名称、来源或画面内容…" className="h-10 w-full pl-10 pr-10" value={query} onChange={e=>{setMatches([]);setQuery(e.target.value);setSearchAttempt(0);}}/>{query && <button type="button" aria-label="清除搜索" className="absolute right-3 top-3 text-muted-foreground hover:text-foreground" onClick={()=>{setQuery("");setMatches([]);setSearchAttempt(0);}}><XIcon className="size-4"/></button>}</div>
      <Button type="submit" disabled={!query.trim()}>{busy?<Loader2Icon className="animate-spin"/>:<SearchIcon/>}搜索</Button>
    </form>
    <div className="flex flex-wrap items-center justify-between gap-3"><div className="flex items-center gap-3"><div className="flex gap-1 rounded-lg bg-muted/60 p-1">{([["all","全部"],["video","视频"],["image","图片"]] as const).map(([value,label])=><button key={value} aria-pressed={filter === value} onClick={()=>setFilter(value)} className={`rounded-md px-4 py-1.5 text-xs transition-colors focus-visible:outline-ring ${filter === value ? "bg-background font-medium shadow-sm" : "text-muted-foreground hover:text-foreground"}`}>{label}</button>)}</div><span className="text-xs text-muted-foreground">{rows.length} 个素材</span></div><div className="flex gap-2"><Button variant="outline" size="sm" aria-label="刷新素材" onClick={()=>{assets.reload();setSearchAttempt(v=>v+1);}}><RefreshCwIcon className={assets.loading?"animate-spin":""}/>刷新</Button><Button size="sm" onClick={()=>{setError(null);setFile(null);setOpen(true);}}><PlusIcon/>导入素材</Button></div></div>
    {query.trim() && <p role="status" className="text-xs text-muted-foreground">{busy?"名称和来源匹配已优先展示，正在补充内容结果…":"名称和来源匹配优先，内容匹配供定位，请打开原片复核。"}</p>}
    {searchError && <p role="alert" className="text-sm text-destructive">{searchError}</p>}
    <div className="overflow-x-auto rounded-xl border bg-background"><table className="w-full text-left text-sm"><thead className="border-b bg-muted/30 text-xs text-muted-foreground"><tr><th className="px-5 py-3 font-medium">素材名称</th><th className="px-4 py-3 font-medium">类型</th><th className="px-4 py-3 font-medium">来源</th><th className="px-4 py-3 font-medium">拍摄 / 导入时间</th>{query.trim() && <th className="px-4 py-3 font-medium">匹配</th>}</tr></thead><tbody className="divide-y">
      {assets.error?<tr><td colSpan={query.trim()?5:4} role="alert" className="p-8 text-center text-destructive">素材读取失败，请刷新重试</td></tr>:assets.loading?<tr><td colSpan={query.trim()?5:4} className="p-8 text-center text-muted-foreground">正在加载素材…</td></tr>:rows.map(({asset:a,match,segments})=><tr key={a.id} className="cursor-pointer hover:bg-muted/40 focus-visible:outline-ring" tabIndex={0} aria-label={`查看素材 ${assetName(a)}`} onClick={()=>router.push(detailHref(a.id,match === "content"?segments[0]:undefined))} onKeyDown={e=>{if(e.target !== e.currentTarget)return;if(e.key === "Enter" || e.key === " "){e.preventDefault();router.push(detailHref(a.id,match === "content"?segments[0]:undefined));}}}>
        <td className="min-w-48 max-w-80 px-5 py-4"><Link href={detailHref(a.id,match === "content"?segments[0]:undefined)} onClick={e=>e.stopPropagation()} className="flex items-center gap-3 hover:text-primary"><span className="rounded-lg bg-muted p-2 text-muted-foreground"><MediaIcon asset={a} className="size-4"/></span><span className="truncate font-medium" title={assetName(a)}>{assetName(a)}</span></Link></td>
        <td className="whitespace-nowrap px-4 py-4 text-xs text-muted-foreground">{mediaType(a) === "video"?"视频":mediaType(a) === "image"?"图片":"文件"}</td>
        <td className="max-w-60 px-4 py-4 text-xs text-muted-foreground"><p className="line-clamp-2" title={a.sourceDescription ?? ""}>{a.sourceDescription || "—"}</p></td>
        <td className="whitespace-nowrap px-4 py-4 text-xs text-muted-foreground"><p>{date(a.capturedAt ?? a.createdAt)}</p><p className="mt-1 text-[11px]">{a.capturedAt?"拍摄时间":"导入时间"}</p></td>
        {query.trim() && <td className="whitespace-nowrap px-4 py-4"><span className={`text-xs ${match === "content"?"text-muted-foreground":"text-primary"}`}>{match === "exact"?"精准匹配":match === "text"?"名称 / 来源":"内容匹配"}</span></td>}
      </tr>)}
      {!assets.loading && !assets.error && !rows.length && <tr><td colSpan={query.trim()?5:4} className="p-12 text-center text-muted-foreground"><ImageIcon className="mx-auto mb-3 size-7 opacity-50"/><p>{assets.data?.length?busy?"正在搜索内容…":"没有匹配的素材":"还没有素材"}</p><p className="mt-2 text-xs">{assets.data?.length?"试试其他关键词或分类":"导入照片或视频开始使用"}</p></td></tr>}
    </tbody></table></div>
    <Dialog open={open} onOpenChange={value=>{if(!uploading) setOpen(value);}}><DialogContent className="sm:max-w-lg" showCloseButton={!uploading}><DialogHeader><DialogTitle>导入素材</DialogTitle><DialogDescription>将照片或视频添加到项目素材。</DialogDescription></DialogHeader>
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
