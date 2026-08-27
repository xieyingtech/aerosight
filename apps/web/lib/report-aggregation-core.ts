export type ReportEvidenceReference={type:"task_run"|"task_version"|"device"|"track"|"step"|"event"|"feedback"|"asset";id:string;version:string;href:string;assetId?:number;checksumSha256?:string|null};
export type ReportDataGap={section:string;code:string;message:string};
export type ReportAggregateInput={projectId:number;taskRun?:Record<string,unknown>|null;taskVersion?:Record<string,unknown>|null;device?:Record<string,unknown>|null;track?:Record<string,unknown>|null;steps?:Record<string,unknown>[]|null;events?:Record<string,unknown>[]|null;feedback?:Record<string,unknown>[]|null;evidence?:ReportEvidenceReference[]|null};

export function aggregateInspectionReport(input:ReportAggregateInput){
  const gaps:ReportDataGap[]=[];const requireValue=(section:string,value:unknown)=>{if(value===null||value===undefined||(Array.isArray(value)&&value.length===0))gaps.push({section,code:"SOURCE_MISSING",message:`${section} 数据不可用`})};
  requireValue("taskRun",input.taskRun);requireValue("taskVersion",input.taskVersion);requireValue("device",input.device);requireValue("track",input.track);requireValue("steps",input.steps);requireValue("evidence",input.evidence);
  const evidence=input.evidence??[];const evidenceKeys=new Set(evidence.map(ref=>`${ref.type}:${ref.id}:${ref.version}`));
  const conclusions:Array<{kind:"observed-fact"|"human-conclusion";text:string;evidenceRefs:string[]}>=[];
  if(input.taskRun){const reference=evidence.find(ref=>ref.type==="task_run");if(reference)conclusions.push({kind:"observed-fact",text:`任务运行状态：${String(input.taskRun.status??"unknown")}`,evidenceRefs:[`${reference.type}:${reference.id}:${reference.version}`]})}
  for(const item of input.feedback??[]){const reference=evidence.find(ref=>ref.type==="feedback"&&ref.id===String(item.id));if(reference)conclusions.push({kind:"human-conclusion",text:String(item.reason??item.action??"人工处置"),evidenceRefs:[`${reference.type}:${reference.id}:${reference.version}`]})}
  for(const conclusion of conclusions)for(const ref of conclusion.evidenceRefs)if(!evidenceKeys.has(ref))throw new Error("REPORT_CONCLUSION_EVIDENCE_MISSING");
  return {projectId:input.projectId,completeness:gaps.length?"incomplete" as const:"complete" as const,dataGaps:gaps,sections:{taskRun:input.taskRun??null,taskVersion:input.taskVersion??null,device:input.device??null,track:input.track??null,steps:input.steps??[],events:input.events??[],feedback:input.feedback??[]},conclusions,evidence};
}

export function planReportPublication(report:{completeness:"complete"|"incomplete"|"failed";evidence:ReportEvidenceReference[]},options:{allowIncomplete:boolean}){
  if(report.completeness==="failed")throw new Error("REPORT_FAILED_NOT_PUBLISHABLE");
  if(report.completeness==="incomplete"&&!options.allowIncomplete)throw new Error("REPORT_INCOMPLETE_CONFIRMATION_REQUIRED");
  return {status:"published" as const,retainedAssetIds:[...new Set(report.evidence.flatMap(ref=>ref.assetId?[ref.assetId]:[]))]};
}

export function canExportGeneratedReport(permissions:ReadonlySet<string>){return permissions.has("report:export")}
