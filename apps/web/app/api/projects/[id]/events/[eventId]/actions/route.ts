import { NextResponse } from "next/server";
import { PERCEPTION_EVENT_ACTIONS,type PerceptionEventAction } from "@/lib/perception-event-actions-core";
import { handlePerceptionEvent } from "@/lib/perception-events";
const supported=new Set<string>(PERCEPTION_EVENT_ACTIONS);
export async function POST(request:Request,{params}:{params:Promise<{id:string;eventId:string}>}){
  const {id,eventId}=await params;const body=await request.json() as {action?:PerceptionEventAction;expectedVersion?:number;reason?:string;category?:string;issueId?:number};
  if(!body.action||!supported.has(body.action)||!Number.isInteger(body.expectedVersion)||!body.reason?.trim())return NextResponse.json({error:"EVENT_ACTION_INVALID"},{status:400});
  try{return NextResponse.json(await handlePerceptionEvent({projectId:Number(id),eventId,action:body.action,expectedVersion:body.expectedVersion!,reason:body.reason,category:body.category,issueId:body.issueId,requestId:request.headers.get("x-request-id")}));}
  catch(error){const code=error instanceof Error?error.message:"EVENT_ACTION_FAILED";return NextResponse.json({error:code},{status:code==="PROJECT_ACCESS_DENIED"?403:409});}
}
