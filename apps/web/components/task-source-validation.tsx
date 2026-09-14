"use client";

import { useRef, useState } from "react";
import { apiFetch } from "@/lib/api-client";
import type { TaskSource } from "@/lib/task-source-editor";
import { Button } from "@/components/ui/button";

type Result = {canPublish?:boolean;error?:string;issues?:Array<{code:string;stepKey?:string;fields?:Array<{path:string;constraint:string}>}>};
export function TaskSourceValidation({projectId,value,disabled=false}:{projectId:number;value:TaskSource;disabled?:boolean}) {
 const signature=JSON.stringify([projectId,value.format,value.source]);
 const current=useRef(signature);current.current=signature;
 const [pending,setPending]=useState(false);
 const [result,setResult]=useState<{signature:string;body:Result}|null>(null);
 async function validate(){
  const submitted=signature;setPending(true);setResult(null);
  try {
   const response=await apiFetch(`/api/projects/${projectId}/tasks/validate`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({sourceFormat:value.format,source:value.source})});
   const body=await response.json() as Result;
   if(!response.ok)throw new Error(body.error??"任务校验失败");
   if(current.current===submitted)setResult({signature:submitted,body});
  }catch(error){if(current.current===submitted)setResult({signature:submitted,body:{error:error instanceof Error?error.message:"任务校验失败"}});}
  finally{setPending(false);}
 }
 const shown=result?.signature===signature?result.body:null;
 return <div className="space-y-2">
  <Button type="button" variant="outline" disabled={disabled||pending} onClick={validate}>{pending?"正在校验…":"校验当前草稿"}</Button>
  {shown&&<div role="status" className="space-y-1 text-sm">
   {shown.error?<p>{shown.error}</p>:shown.canPublish?<p>当前草稿校验通过；发布和运行前仍会重新检查权限与资源。</p>:<p>当前草稿暂不可发布：</p>}
   {shown.issues?.map((issue,index)=><div key={index}><p>{issue.stepKey?`步骤 ${issue.stepKey}：`:""}{issue.code}</p>{issue.fields?.map((field,i)=><p className="font-mono text-xs" key={i}>{field.path} · {field.constraint}</p>)}</div>)}
  </div>}
 </div>;
}
