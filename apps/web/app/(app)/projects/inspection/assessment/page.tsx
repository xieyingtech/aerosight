"use client";
import {Page} from "@/components/page";
import {StaticAPIPage,positiveParam,uuidParam} from "@/components/static-api-page";
import {InspectionAssessmentPanel,type AssessmentModel} from "@/components/inspection-assessment-panel";
export default function AssessmentPage(){
 return <StaticAPIPage<AssessmentModel> endpoint={q=>{const project=positiveParam(q),id=uuidParam(q,"assessmentId");return project&&id?`/api/projects/${project}/inspection/assessments/${id}`:null;}}>{(model,q,reload)=><Page title="巡检研判与复核" description="查看证据范围、模型原文和人工修订"><InspectionAssessmentPanel key={`${model.id}:${model.revision}`} projectId={positiveParam(q)!} model={model} onChanged={reload}/></Page>}</StaticAPIPage>;
}
