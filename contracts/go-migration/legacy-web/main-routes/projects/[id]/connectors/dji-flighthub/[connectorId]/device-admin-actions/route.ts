import { NextResponse } from "next/server";
import { readFlightHubDeviceAdminJob, submitFlightHubDeviceAdminAction } from "@/lib/dji-flighthub-device-admin";
import { assertLiveControlRequest } from "@/lib/replay-policy";

export async function POST(request:Request,{params}:{params:Promise<{id:string;connectorId:string}>}){
  const {id,connectorId}=await params;const projectId=Number(id),connectorInstanceId=Number(connectorId);
  if(!Number.isSafeInteger(projectId)||projectId<=0||!Number.isSafeInteger(connectorInstanceId)||connectorInstanceId<=0)return NextResponse.json({error:"INPUT_INVALID"},{status:400});
  try{assertLiveControlRequest(request);const body=await request.json() as Record<string,unknown>;return NextResponse.json(await submitFlightHubDeviceAdminAction(projectId,{...body,connectorInstanceId},request.headers.get("x-request-id")),{status:202});}
  catch(error){return NextResponse.json({error:error instanceof Error?error.message:"FLIGHTHUB_DEVICE_ADMIN_FAILED"},{status:409});}
}
export async function GET(request:Request,{params}:{params:Promise<{id:string;connectorId:string}>}){
  const {id,connectorId}=await params;const jobId=new URL(request.url).searchParams.get("jobId")??"";
  if(!/^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(jobId))return NextResponse.json({error:"INPUT_INVALID"},{status:400});
  try{return NextResponse.json(await readFlightHubDeviceAdminJob(Number(id),Number(connectorId),jobId));}
  catch(error){return NextResponse.json({error:error instanceof Error?error.message:"FLIGHTHUB_DEVICE_ADMIN_FAILED"},{status:404});}
}
