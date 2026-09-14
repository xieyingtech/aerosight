"use client";
import Link from "next/link";
import {useRef,useState} from "react";
import {useAPI} from "@/lib/use-api";
import {apiFetch} from "@/lib/api-client";
import {APIStateView} from "@/components/api-state";
import {Button} from "@/components/ui/button";
import {Card,CardContent,CardHeader,CardTitle} from "@/components/ui/card";

type Decision={candidateId?:string;action:string;reason:string;evidenceRefs:string[];missingInformation?:string[];issueId?:number};
type Evidence={id:string;observationId:string;source:string;completeness:string;targetAlgorithmConfirmed:boolean;candidates:Array<{id:string;evidenceRefs:string[];position:{source:string;quality:string}}>};
export type AssessmentModel={id:string;taskRunId:number;evidenceSetId:string;status:string;runStatus:string;revision:number;modelVersion:string;promptVersion:string;originalOutput:string|null;canReview:boolean;revisions:Array<{revision:number;source:string;decisions:Decision[]}>};
export function InspectionAssessmentPanel({projectId,model,onChanged}:{projectId:number;model:AssessmentModel;onChanged:()=>void}){
 const state=useAPI<Evidence>(`/api/projects/${projectId}/inspection/evidence-sets/${model.evidenceSetId}`);
 return <div className="space-y-4">
  <div className="flex flex-wrap gap-4 text-sm"><span>研判：{model.status}</span><span>任务：{model.runStatus}</span><span>修订 {model.revision}</span><Link className="underline" href={`/projects/tasks/runs/detail/?projectId=${projectId}&runId=${model.taskRunId}`}>返回任务运行</Link><Button variant="outline" size="sm" onClick={onChanged}>刷新状态</Button></div>
  <p className="text-sm text-muted-foreground">结论只适用于证据中列出的图片和时段。照片位置不等于目标坐标；单期影像不能确认新增或违法。</p>
  <Card><CardHeader><CardTitle>模型原始研判</CardTitle></CardHeader><CardContent><p className="mb-2 text-xs text-muted-foreground">模型 {model.modelVersion??"未知"} · 提示模板 {model.promptVersion??"未知"}</p><pre className="max-h-72 overflow-auto whitespace-pre-wrap rounded bg-muted p-3 text-xs">{model.originalOutput??"尚无模型输出"}</pre></CardContent></Card>
  <APIStateView state={state}>{evidence=><ReviewForm projectId={projectId} model={model} evidence={evidence} onChanged={onChanged}/>}</APIStateView>
  <details className="rounded border p-3"><summary>修订历史（{model.revisions.length}）</summary>{model.revisions.map(revision=><div className="mt-3" key={revision.revision}><p className="text-sm">修订 {revision.revision} · {revision.source==="human"?"人工复核":"模型输出"}</p><pre className="overflow-auto whitespace-pre-wrap text-xs">{JSON.stringify(revision.decisions,null,2)}</pre></div>)}</details>
 </div>;
}
function ReviewForm({projectId,model,evidence,onChanged}:{projectId:number;model:AssessmentModel;evidence:Evidence;onChanged:()=>void}){
 const latest=model.revisions.at(-1)?.decisions??[];
 const [decisions,setDecisions]=useState<Decision[]>(()=>evidence.candidates.length?evidence.candidates.map(candidate=>latest.find(d=>d.candidateId===candidate.id)??{candidateId:candidate.id,action:"needs_review",reason:"",evidenceRefs:candidate.evidenceRefs,missingInformation:[]}):latest.length?latest:[{action:"needs_review",reason:"",evidenceRefs:[],missingInformation:[]}]);
 const [pending,setPending]=useState(false),[error,setError]=useState("");
 const requestKeys=useRef(new Map<string,string>());
 const editable=model.canReview&&model.status==="needs_review"&&model.runStatus==="paused";
 function update(index:number,value:Partial<Decision>){setDecisions(current=>current.map((decision,n)=>n===index?{...decision,...value}:decision));}
 async function submit(){
  setPending(true);setError("");
  const payload={expectedRevision:model.revision,decisions};const signature=JSON.stringify(payload);let key=requestKeys.current.get(signature);if(!key){key=crypto.randomUUID();requestKeys.current.set(signature,key);}
  try{const res=await apiFetch(`/api/projects/${projectId}/inspection/assessments/${model.id}/review`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({...payload,idempotencyKey:key})});const body=await res.json();if(!res.ok)throw new Error(String(body.error??"复核提交失败"));onChanged();}
  catch(e){setError(e instanceof Error?e.message:"复核提交失败");}finally{setPending(false);}
 }
 return <Card><CardHeader><CardTitle>证据与整批复核</CardTitle></CardHeader><CardContent className="space-y-4">
  <p className="text-sm">识别来源：{evidence.source==="external"?"外部算法":evidence.source==="flighthub-ai"?"司空原生告警":"未标明识别来源"} · 完整性：{evidence.completeness}</p>
  <div className="flex gap-4 text-sm"><a className="underline" href={`/projects/inspection/observation/?projectId=${projectId}&observationId=${evidence.observationId}`} target="_blank" rel="noreferrer">查看观察范围与原图引用</a><a className="underline" href={`/projects/inspection/evidence/?projectId=${projectId}&evidenceSetId=${evidence.id}`} target="_blank" rel="noreferrer">查看完整识别证据</a></div>
  {!editable&&<p role="status" className="text-sm text-muted-foreground">{!model.canReview?"当前账号可查看，不能提交复核。":"当前研判不在可复核状态。"}</p>}
  {decisions.map((decision,index)=><fieldset disabled={!editable||pending} className="space-y-2 rounded border p-3" key={decision.candidateId??index}>
   <legend className="px-1 text-sm">{decision.candidateId?`线索 ${index+1}`:"本次分析范围"}</legend>
   {decision.candidateId&&<p className="break-all text-xs text-muted-foreground">{decision.candidateId} · 位置需核实</p>}
   <label className="block text-sm">处理决定<select aria-label={`线索 ${index+1} 处理决定`} className="ml-2 rounded border bg-background p-2" value={decision.action} onChange={e=>update(index,{action:e.target.value,issueId:undefined})}><option value="needs_review">请选择复核结论</option>{decision.candidateId&&<><option value="create">确认线索并建案</option><option value="update">关联既有案件</option></>}<option value="reject">驳回线索</option>{evidence.completeness==="complete"&&evidence.targetAlgorithmConfirmed&&<option value="no_issue">确认已分析范围无异常</option>}</select></label>
   {decision.action==="update"&&<label className="block text-sm">目标案件 ID<input type="number" min={1} className="ml-2 rounded border bg-background p-2" value={decision.issueId??""} onChange={e=>update(index,{issueId:e.target.value?Number(e.target.value):undefined})}/></label>}
   <label className="block text-sm">复核理由<textarea className="mt-1 block min-h-20 w-full rounded border bg-background p-2" value={decision.reason} onChange={e=>update(index,{reason:e.target.value})}/></label>
   {decision.missingInformation?.length? <p className="text-xs text-muted-foreground">原资料缺口：{decision.missingInformation.join("、")}</p>:null}
  </fieldset>)}
  {editable&&<><p className="text-xs text-muted-foreground">提交后保存新的人工修订并继续案件与报告步骤，不重新飞行或识别。驳回只表示不采纳线索，不代表未观察区域没有异常。</p><Button onClick={submit} disabled={pending||decisions.some(d=>d.action==="needs_review"||!d.reason.trim()||(d.action==="update"&&!d.issueId))}>{pending?"正在提交…":"提交整批复核并继续"}</Button></>}
  {error&&<p role="alert" className="text-sm text-destructive">{error}。如修订冲突或已过期，请刷新后查看最新状态。</p>}
 </CardContent></Card>;
}
