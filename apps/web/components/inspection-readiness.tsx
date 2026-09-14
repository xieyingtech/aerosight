"use client";
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
 <p>可用图片：{ready.availableImages} · <Link className="underline" href={`/projects/assets/?projectId=${projectId}`}>查看图片</Link>{ready.availableImages===0&&" · 图片模板需先准备授权图片"}</p>
 <p>司空连接器：{ready.flightHubConnectors} · 已完成飞行目录：{ready.completedFlights} · <Link className="underline" href={`/projects/connectors/?projectId=${projectId}`}>连接与同步</Link></p>
 <p>外部检测已发布版本：{ready.detectionVersions} · <Link className="underline" href={`/projects/algorithms/?projectId=${projectId}`}>配置算法</Link>{ready.detectionVersions===0&&" · 外部检测暂缺可用配置"}</p>
 <p>巡检智能体：{ready.copilotConfigured?"已配置":"未配置"} · 默认 AI 模型：{ready.modelConfigured?"已配置，尚未验证调用":"未配置，请联系管理员"} · <Link className="underline" href={`/projects/agents/?projectId=${projectId}`}>查看智能体</Link></p>
 <p className="text-xs text-muted-foreground">司空原生告警能否覆盖目标类别，应以具体架次的识别证据为准；零告警不代表没有异常。</p>
 </div>}</APIStateView>
 </section>;
}
