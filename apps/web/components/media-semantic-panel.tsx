"use client";

import {useEffect, useState} from "react";
import {apiJSON} from "@/lib/api-client";
import {useAPI} from "@/lib/use-api";
import {Button} from "@/components/ui/button";

type IndexStatus = {enabled: boolean; state: string; attempts: number; errorCode: string | null; segments: number};

export function MediaIndexStatus({projectId,assetId}: {projectId: number; assetId: number}) {
  const index = useAPI<IndexStatus>(`/api/projects/${projectId}/assets/${assetId}/semantic-index`);
  const [busy,setBusy] = useState(false);
  const [error,setError] = useState<string | null>(null);
  useEffect(()=>{if(!index.data?.enabled || !["queued","running","failed"].includes(index.data.state) || index.data.attempts >= 10)return;const timer=setInterval(index.reload,5000);return ()=>clearInterval(timer);},[index.data,index.reload]);
  async function retry(){setBusy(true);setError(null);try{await apiJSON(`/api/projects/${projectId}/assets/${assetId}/semantic-index/retry`,{method:"POST"});index.reload();}catch{setError("重试失败，需要项目管理员权限");}finally{setBusy(false);}}
  const names:Record<string,string> = {disabled:"未启用",queued:"等待处理",running:"正在解析",indexed:"可搜索",failed:"处理失败",obsolete:"素材版本已失效"};
  return <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground"><span>内容索引：{index.error ? "状态读取失败" : index.loading ? "读取中…" : names[index.data?.state ?? ""] ?? "未处理"}<span className="inline-block whitespace-pre-line">{index.data?.state === "indexed" ? `\n${index.data.segments} 个片段` : ""}</span></span>{index.data?.enabled && <Button variant="outline" size="sm" disabled={busy || index.data.state === "running"} onClick={retry}>{busy ? "排队中…" : index.data.state === "indexed" ? "重建索引" : "重新处理"}</Button>}{index.data?.errorCode && <span>{index.data.errorCode}</span>}{error && <span role="alert">{error}</span>}</div>;
}
