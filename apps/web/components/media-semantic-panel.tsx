"use client";

import {useEffect, useState} from "react";
import {apiJSON} from "@/lib/api-client";
import {useAPI} from "@/lib/use-api";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";

type IndexStatus = {enabled: boolean; state: string; attempts: number; errorCode: string | null; segments: number};
export type MediaMatch = {assetId: number; startMs: number; endMs: number; description: string; reference: {href: string}};

export function MediaIndexStatus({projectId,assetId}: {projectId: number; assetId: number}) {
  const index = useAPI<IndexStatus>(`/api/projects/${projectId}/assets/${assetId}/semantic-index`);
  const [busy,setBusy] = useState(false);
  const [error,setError] = useState<string | null>(null);
  useEffect(()=>{if(!index.data?.enabled || !["queued","running","failed"].includes(index.data.state) || index.data.attempts >= 10)return;const timer=setInterval(index.reload,5000);return ()=>clearInterval(timer);},[index.data,index.reload]);
  async function retry(){setBusy(true);setError(null);try{await apiJSON(`/api/projects/${projectId}/assets/${assetId}/semantic-index/retry`,{method:"POST"});index.reload();}catch{setError("重试失败，需要项目管理员权限");}finally{setBusy(false);}}
  const names:Record<string,string> = {disabled:"未启用",queued:"等待处理",running:"正在解析",indexed:"可搜索",failed:"处理失败",obsolete:"素材版本已失效"};
  return <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground"><span>内容索引：{index.error ? "状态读取失败" : index.loading ? "读取中…" : names[index.data?.state ?? ""] ?? "未处理"}{index.data?.state === "indexed" ? ` · ${index.data.segments} 个片段` : ""}</span>{index.data?.enabled && <Button variant="outline" size="sm" disabled={busy || index.data.state === "running"} onClick={retry}>{busy ? "排队中…" : index.data.state === "indexed" ? "重建索引" : "重新处理"}</Button>}{index.data?.errorCode && <span>{index.data.errorCode}</span>}{error && <span role="alert">{error}</span>}</div>;
}

export function MediaSemanticSearch({projectId,onSelect}: {projectId: number; onSelect:(match:MediaMatch)=>void}) {
  const [query,setQuery] = useState("");
  const [result,setResult] = useState<{projectId:number;items:MediaMatch[]} | null>(null);
  const [busy,setBusy] = useState(false);
  const [error,setError] = useState<string | null>(null);
  const items=result?.projectId === projectId ? result.items : null;
  async function search(){if(!query.trim() || busy)return;setBusy(true);setError(null);try{const response=await apiJSON<{items:MediaMatch[]}>(`/api/projects/${projectId}/media-search`,{method:"POST",body:JSON.stringify({query})});setResult({projectId,items:response.items});}catch{setError("内容搜索暂不可用，请检查索引状态或稍后重试");setResult(null);}finally{setBusy(false);}}
  return <div className="space-y-2"><form onSubmit={e=>{e.preventDefault();void search();}} className="space-y-2"><Input aria-label="按画面内容搜索" placeholder="按画面搜索，例如林间石阶…" value={query} onChange={e=>setQuery(e.target.value)}/><Button type="submit" variant="outline" size="sm" disabled={busy || !query.trim()}>{busy ? "搜索中…" : "内容搜索"}</Button>{items !== null && <Button type="button" size="sm" variant="ghost" onClick={()=>setResult(null)}>清除结果</Button>}</form>{error && <p role="alert" className="text-xs text-destructive">{error}</p>}{items !== null && <div className="max-h-80 space-y-2 overflow-y-auto"><p className="text-xs text-muted-foreground">{items.length ? "模型描述仅供定位，请打开原片复核" : "没有匹配片段，可能还有素材尚未完成索引"}</p>{items.map((m,i)=><button key={`${m.assetId}-${m.startMs}-${i}`} className="block w-full rounded-md border p-2 text-left text-xs hover:bg-muted" onClick={()=>onSelect(m)}><span className="block font-medium">素材 #{m.assetId} · {(m.startMs/1000).toFixed(1)}–{(m.endMs/1000).toFixed(1)} 秒</span><span className="mt-1 block text-muted-foreground">{m.description}</span></button>)}</div>}</div>;
}
