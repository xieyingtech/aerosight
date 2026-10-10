"use client";
import { canonicalPageHref } from "@/lib/page-routes";

import Link from "next/link";
import {useAPI} from "@/lib/use-api";
import {APIStateView} from "@/components/api-state";
import {Button} from "@/components/ui/button";
type Readiness={availableImages:number;flightHubConnectors:number;completedFlights:number;detectionVersions:number;copilotConfigured:boolean;modelConfigured:boolean;verification:"catalogue-only"};
export function InspectionReadiness({projectId}:{projectId:number}){
 const state=useAPI<Readiness>(`/api/projects/${projectId}/inspection/readiness`);
 return <section aria-label="巡检资源就绪检查" className="space-y-3 rounded border p-3">
 <div className="flex items-center justify-between"><h3 className="text-sm font-medium">巡检资源就绪检查</h3><Button type="button" size="sm" variant="outline" onClick={state.reload}>刷新资源</Button></div>
 <p className="text-xs text-muted-foreground">此处仅核对当前项目目录与配置。图片授权、远端可访问性、密钥解密、模型效果和实际飞行均需另行验证；发布与运行时仍会检查具体输入。</p>
 <APIStateView state={state}>{ready=><div className="space-y-2 text-sm">
 <p><span className="inline-flex max-w-full flex-wrap items-center gap-x-3 gap-y-1"><span>可用图片：{ready.availableImages}</span><span><Link className="underline" href={canonicalPageHref(`/projects/assets/?projectId=${projectId}`)}>查看图片</Link><span className="inline-block whitespace-pre-line">{ready.availableImages===0&&"\n图片模板需先准备授权图片"}</span></span></span></p>
 <p><span className="inline-flex max-w-full flex-wrap items-center gap-x-3 gap-y-1"><span>司空连接器：{ready.flightHubConnectors}</span><span>已完成飞行目录：{ready.completedFlights}</span><span><Link className="underline" href={canonicalPageHref(`/projects/connectors/?projectId=${projectId}`)}>连接与同步</Link></span></span></p>
 <p><span className="inline-flex max-w-full flex-wrap items-center gap-x-3 gap-y-1"><span>外部检测已发布版本：{ready.detectionVersions}</span><span><Link className="underline" href={canonicalPageHref(`/projects/algorithms/?projectId=${projectId}`)}>配置算法</Link><span className="inline-block whitespace-pre-line">{ready.detectionVersions===0&&"\n外部检测暂缺可用配置"}</span></span></span></p>
 <p><span className="inline-flex max-w-full flex-wrap items-center gap-x-3 gap-y-1"><span>巡检智能体：{ready.copilotConfigured?"已配置":"未配置"}</span><span>默认 AI 模型：{ready.modelConfigured?"已配置，尚未验证调用":"未配置，请联系管理员"}</span><span><Link className="underline" href={canonicalPageHref(`/projects/agents/?projectId=${projectId}`)}>查看智能体</Link></span></span></p>
 <p className="text-xs text-muted-foreground">司空原生告警能否覆盖目标类别，应以具体架次的识别证据为准；零告警不代表没有异常。</p>
 </div>}</APIStateView>
 </section>;
}
