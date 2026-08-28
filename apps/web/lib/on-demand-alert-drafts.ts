import "server-only";

import { createAgentExecutionContext } from "@/lib/agent-execution-context-core";
import { executeAgentDraftTool } from "@/lib/agent-draft-tools";
import { collectAgentTextStream } from "@/lib/agent-provider-registry";
import { loadAgentProviderRegistry } from "@/lib/agent-provider-loader";
import { appendAgentMessage, createAgentSession } from "@/lib/agent-sessions";
import { requireCurrentProjectPermission } from "@/lib/data";
import { readPerceptionEvent } from "@/lib/perception-events";

const PROMPT_TEMPLATE_VERSION="perception-event-summary/v1";

export async function generateOnDemandAlertDraft(projectId:number,eventId:string,requestId?:string|null){
  const {user,access}=await requireCurrentProjectPermission(projectId,"agent:use");
  const evidence=await readPerceptionEvent(projectId,eventId);
  const session=await createAgentSession(projectId,requestId);
  const context=createAgentExecutionContext({userId:user.id,teamId:access.teamId,projectId,sessionId:session.id});
  const references=[
    {type:"event" as const,id:eventId,version:`state:${String(evidence.event.stateVersion)}`,observedAt:new Date(String(evidence.event.lastDetectedAt)).toISOString(),quality:"platform-event"},
    ...evidence.detections.flatMap((detection)=>[
      {type:"detection" as const,id:String(detection.id),version:`model:${String(detection.modelVersion)};mapping:${String(detection.mappingVersion)}`,observedAt:new Date(String(detection.capturedAt)).toISOString(),quality:String(detection.locationQuality)},
      {type:"asset" as const,id:String(detection.inputAssetId),version:String(detection.assetChecksumSha256??`version:${String(detection.assetVersion)}`),observedAt:new Date(String(detection.capturedAt)).toISOString(),quality:"immutable-lineage"}
    ])
  ];
  const prompt=`${PROMPT_TEMPLATE_VERSION}\n你是巡检告警分析助手。只根据下列结构化事实生成中文摘要，区分事实、规则推断、模型推断和建议；明确不确定性，不改变人工结论。\n${JSON.stringify({event:evidence.event,detections:evidence.detections.map((item)=>({id:item.id,label:item.label,confidence:item.confidence,locationQuality:item.locationQuality,modelVersion:item.modelVersion,mappingVersion:item.mappingVersion,capturedAt:item.capturedAt,inputAssetId:item.inputAssetId})),feedback:evidence.feedback})}`;
  const configured=await loadAgentProviderRegistry();
  const generatedAt=new Date();
  const summary=await collectAgentTextStream(configured.registry.languageModel(configured.modelId),prompt);
  const draft=await executeAgentDraftTool(context,"draft_report",{title:`告警 ${eventId} 分析草稿`,sections:[{heading:"智能体摘要",body:summary}],evidenceRefs:references},requestId,
    {modelId:configured.modelId,promptTemplateVersion:PROMPT_TEMPLATE_VERSION,toolCalls:[{name:"query_events",status:"succeeded",evidenceCount:references.length}],generatedAt});
  await appendAgentMessage({projectId,sessionId:session.id,role:"assistant",content:summary,requestId,toolCalls:[{name:"query_events",status:"succeeded",summary:"已读取告警事实与证据版本",evidenceRefs:references.map((reference)=>({type:reference.type,id:reference.id,version:reference.version}))},{name:"draft_report",status:"succeeded",summary:`已创建报告草稿 ${draft.id}`,evidenceRefs:references.map((reference)=>({type:reference.type,id:reference.id,version:reference.version}))}]});
  return {draftId:draft.id,sessionId:session.id,modelId:configured.modelId,promptTemplateVersion:PROMPT_TEMPLATE_VERSION};
}
