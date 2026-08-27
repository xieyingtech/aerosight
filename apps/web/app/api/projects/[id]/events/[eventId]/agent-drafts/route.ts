import {NextResponse} from "next/server";
import {generateOnDemandAlertDraft} from "@/lib/on-demand-alert-drafts";
export async function POST(request:Request,{params}:{params:Promise<{id:string;eventId:string}>}){const {id,eventId}=await params;try{return NextResponse.json(await generateOnDemandAlertDraft(Number(id),eventId,request.headers.get("x-request-id")),{status:201})}catch(error){const code=error instanceof Error?error.message:"AGENT_DRAFT_FAILED";return NextResponse.json({error:code},{status:code==="PROJECT_ACCESS_DENIED"?403:503})}}
