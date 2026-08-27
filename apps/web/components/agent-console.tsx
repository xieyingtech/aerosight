"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { AgentSessionView } from "@/lib/agent-sessions";

export function AgentConsole({projectId,sessions}:{projectId:number;sessions:AgentSessionView[]}){
  const router=useRouter();const [error,setError]=useState<string|null>(null);const session=sessions[0];
  async function create(){setError(null);const response=await fetch(`/api/projects/${projectId}/agent-sessions`,{method:"POST"});if(!response.ok)setError((await response.json()).error??"创建失败");else router.refresh()}
  async function send(formData:FormData){if(!session)return;setError(null);const response=await fetch(`/api/projects/${projectId}/agent-sessions/${session.id}/messages`,{method:"POST",headers:{"content-type":"application/json"},body:JSON.stringify({content:formData.get("content")})});if(!response.ok)setError((await response.json()).error??"发送失败");else router.refresh()}
  return <div className="space-y-4"><div className="flex items-center justify-between"><p className="text-sm text-muted-foreground">会话严格绑定当前用户与项目；工具结果仅保留状态和证据引用。</p><Button onClick={create}>新建会话</Button></div>
    {session?<section className="space-y-3 rounded-xl border p-4"><p className="text-sm font-medium">会话 #{session.id} · {session.status}</p>{session.messages.length?session.messages.map(message=><article className="rounded-lg bg-muted/40 p-3" key={message.id}><p className="text-xs text-muted-foreground">{message.role} · {new Date(message.createdAt).toLocaleString()}</p><p className="mt-1 whitespace-pre-wrap text-sm">{message.content}</p>{Array.isArray(message.toolCalls)?message.toolCalls.map((tool,index)=>{const item=tool as {name?:string;status?:string;summary?:string;evidenceRefs?:Array<{type:string;id:string;version:string;href?:string}>};return <div className="mt-2 border-l-2 pl-3 text-xs" key={index}><p>{item.name} · {item.status}</p>{item.summary?<p className="text-muted-foreground">{item.summary}</p>:null}{item.evidenceRefs?.map(ref=><p key={`${ref.type}:${ref.id}:${ref.version}`}>{ref.href?<a className="underline" href={ref.href}>{ref.type}:{ref.id}</a>:<span>{ref.type}:{ref.id}</span>} · {ref.version}</p>)}</div>}):null}</article>):<p className="text-sm text-muted-foreground">暂无消息。</p>}<form action={send} className="flex gap-2"><Input name="content" placeholder="询问态势、生成草案或请求受保护调度" required/><Button type="submit">发送</Button></form></section>:<p className="rounded-xl border p-6 text-sm text-muted-foreground">创建会话后开始使用时空智能体。</p>}{error?<p className="text-sm text-destructive">{error}</p>:null}</div>
}
