"use client";
import {useState} from "react";
import {Button} from "@/components/ui/button";
export function AgentEventDraftButton({projectId,eventId}:{projectId:number;eventId:string}){const [state,setState]=useState<string|null>(null);async function generate(){setState("生成中…");const response=await fetch(`/api/projects/${projectId}/events/${eventId}/agent-drafts`,{method:"POST"});const result=await response.json();setState(response.ok?`草稿 #${result.draftId} 已生成`:result.error??"生成失败")};return <div className="flex items-center gap-3"><Button onClick={generate} variant="outline">生成智能体分析草稿</Button>{state?<span className="text-sm text-muted-foreground">{state}</span>:null}</div>}
