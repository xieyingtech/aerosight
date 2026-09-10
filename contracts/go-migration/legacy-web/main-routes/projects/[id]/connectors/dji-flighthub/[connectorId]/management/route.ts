import { NextResponse } from "next/server";

import { lookupFlightHubJoinCode, readFlightHubManagement } from "@/lib/dji-flighthub-management";

function identifiers(id:string,connectorId:string){
  const projectId=Number(id);
  return Number.isSafeInteger(projectId)&&projectId>0&&/^\d+$/.test(connectorId)?projectId:null;
}

export async function GET(_request:Request,{params}:{params:Promise<{id:string;connectorId:string}>}){
  const {id,connectorId}=await params,projectId=identifiers(id,connectorId);
  if(projectId===null)return NextResponse.json({error:"INPUT_INVALID"},{status:400});
  try{return NextResponse.json(await readFlightHubManagement(projectId,connectorId));}
  catch(error){return NextResponse.json({error:error instanceof Error?error.message:"FLIGHTHUB_MANAGEMENT_FAILED"},{status:403});}
}

export async function POST(request:Request,{params}:{params:Promise<{id:string;connectorId:string}>}){
  const {id,connectorId}=await params,projectId=identifiers(id,connectorId);
  if(projectId===null)return NextResponse.json({error:"INPUT_INVALID"},{status:400});
  try{return NextResponse.json(await lookupFlightHubJoinCode(projectId,connectorId,await request.json()));}
  catch(error){return NextResponse.json({error:error instanceof Error?error.message:"FLIGHTHUB_MANAGEMENT_LOOKUP_FAILED"},{status:403});}
}
