import { NextResponse } from "next/server";
import { createAgentSession } from "@/lib/agent-sessions";
export async function POST(request:Request,{params}:{params:Promise<{id:string}>}){const {id}=await params;try{return NextResponse.json(await createAgentSession(Number(id),request.headers.get("x-request-id")),{status:201})}catch(error){const code=error instanceof Error?error.message:"AGENT_SESSION_CREATE_FAILED";return NextResponse.json({error:code},{status:code==="PROJECT_ACCESS_DENIED"?403:400})}}
