"use client";

import Link from "next/link";
import { useState } from "react";
import { CircleCheckIcon, CircleDotIcon, SearchIcon, MapPinIcon } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { projectPageHref } from "@/lib/page-routes";
import type { IssueListItem } from "@/lib/web-api-types";
import { issuePriorityLabel } from "@/lib/issue-view-core";

export function IssueList({ items, projectId }: { items: IssueListItem[]; projectId: number }) {
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("open");
  const [priority, setPriority] = useState("");
  const [sort, setSort] = useState("updated");
  const open = items.filter(item => item.status !== "closed").length;
  const priorities = Array.from(new Set(items.map(item => item.priority)));
  const filtered = items.filter(item => (status === "all" || (status === "closed" ? item.status === "closed" : item.status !== "closed"))
    && (!priority || item.priority === priority)
    && (!query.trim() || [item.title, `#${item.number}`, ...(item.labels ?? [])].some(value => value.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()))))
    .sort((a,b) => sort === "oldest" ? a.number - b.number : sort === "newest" ? b.number - a.number : new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime());
  return <div className="space-y-4">
    <div className="relative"><SearchIcon className="absolute left-3 top-2.5 size-4 text-muted-foreground" /><Input aria-label="搜索案件" className="pl-9" placeholder="搜索标题、编号或标签…" value={query} onChange={event => setQuery(event.target.value)} /></div>
    <section className="overflow-hidden rounded-md border" aria-label="案件列表">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b bg-muted/40 px-4 py-3">
        <div className="flex flex-wrap items-center gap-4 text-sm">
          <button type="button" aria-pressed={status === "open"} className={`inline-flex items-center gap-1.5 ${status === "open" ? "font-semibold" : "text-muted-foreground hover:text-foreground"}`} onClick={() => setStatus("open")}><CircleDotIcon className="size-4" />{open} 开放</button>
          <button type="button" aria-pressed={status === "closed"} className={`inline-flex items-center gap-1.5 ${status === "closed" ? "font-semibold" : "text-muted-foreground hover:text-foreground"}`} onClick={() => setStatus("closed")}><CircleCheckIcon className="size-4" />{items.length - open} 已关闭</button>
          <button type="button" aria-pressed={status === "all"} className={status === "all" ? "font-semibold" : "text-muted-foreground hover:text-foreground"} onClick={() => setStatus("all")}>全部</button>
        </div>
        <div className="flex gap-3 text-xs">
          <select aria-label="筛选优先级" className="max-w-36 bg-transparent outline-none" value={priority} onChange={event => setPriority(event.target.value)}><option value="">所有优先级</option>{priorities.map(value => <option key={value} value={value}>{issuePriorityLabel(value)}</option>)}</select>
          <select aria-label="案件排序" className="bg-transparent outline-none" value={sort} onChange={event => setSort(event.target.value)}><option value="updated">最近更新</option><option value="newest">最新创建</option><option value="oldest">最早创建</option></select>
        </div>
      </div>
      {filtered.length ? <ul className="divide-y">{filtered.map(item => <li className="flex items-start gap-3 px-4 py-4 hover:bg-muted/30" key={item.id}>
        {item.status === "closed" ? <CircleCheckIcon className="mt-0.5 size-5 shrink-0 text-purple-600" /> : <CircleDotIcon className="mt-0.5 size-5 shrink-0 text-emerald-600" />}
        <div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-x-2 gap-y-1.5"><Link className="break-words font-semibold hover:text-primary" href={projectPageHref(projectId, "issues/detail", {issueId: String(item.id)})}>{item.title}</Link>{(item.labels ?? []).map(label => <Badge variant="secondary" key={label}>{label}</Badge>)}</div>
          <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground"><span>#{item.number}</span><span>更新于 {new Date(item.updatedAt).toLocaleDateString("zh-CN")}</span>{item.hasMapLocation && <span className="inline-flex items-center gap-1"><MapPinIcon className="size-3" />含地图位置</span>}</div>
        </div><Badge className="shrink-0" variant="outline">{issuePriorityLabel(item.priority)}优先级</Badge>
      </li>)}</ul> : <div className="flex flex-col items-center gap-2 px-6 py-16 text-center"><CircleDotIcon className="mb-2 size-8 text-muted-foreground" /><h2 className="font-semibold">{items.length ? "没有匹配的案件" : "暂无案件"}</h2><p className="text-sm text-muted-foreground">{items.length ? "调整搜索或筛选条件查看其他案件。" : "任务和人工处置产生的案件会显示在这里。"}</p></div>}
    </section><p className="text-xs text-muted-foreground">显示 {filtered.length} 条，共 {items.length} 条案件</p>
  </div>;
}
