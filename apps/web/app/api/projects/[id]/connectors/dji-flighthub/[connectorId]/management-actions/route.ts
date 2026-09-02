import { NextResponse } from "next/server";
import { previewFlightHubProjectMemberWrite, readFlightHubManagementWriteJob,
  submitFlightHubProjectMemberWrite } from "@/lib/dji-flighthub-management-write";
import { assertLiveControlRequest } from "@/lib/replay-policy";

function scope(values: { id: string; connectorId: string }) {
  const projectId = Number(values.id), connectorInstanceId = Number(values.connectorId);
  if (!Number.isSafeInteger(projectId) || projectId <= 0 || !Number.isSafeInteger(connectorInstanceId) || connectorInstanceId <= 0) return null;
  return { projectId, connectorInstanceId };
}

export async function PUT(request: Request, { params }: { params: Promise<{ id: string; connectorId: string }> }) {
  const resolved = scope(await params);
  if (!resolved) return NextResponse.json({ error: "INPUT_INVALID" }, { status: 400 });
  try { return NextResponse.json(await previewFlightHubProjectMemberWrite(resolved.projectId, resolved.connectorInstanceId, await request.json())); }
  catch (error) { return NextResponse.json({ error: error instanceof Error ? error.message : "FLIGHTHUB_MANAGEMENT_WRITE_FAILED" }, { status: 409 }); }
}

export async function POST(request: Request, { params }: { params: Promise<{ id: string; connectorId: string }> }) {
  const resolved = scope(await params);
  if (!resolved) return NextResponse.json({ error: "INPUT_INVALID" }, { status: 400 });
  try { assertLiveControlRequest(request); const body = await request.json() as Record<string, unknown>;
    return NextResponse.json(await submitFlightHubProjectMemberWrite(resolved.projectId,
      { ...body, connectorInstanceId: resolved.connectorInstanceId }, request.headers.get("x-request-id")), { status: 202 }); }
  catch (error) { return NextResponse.json({ error: error instanceof Error ? error.message : "FLIGHTHUB_MANAGEMENT_WRITE_FAILED" }, { status: 409 }); }
}

export async function GET(request: Request, { params }: { params: Promise<{ id: string; connectorId: string }> }) {
  const resolved = scope(await params), jobId = new URL(request.url).searchParams.get("jobId") ?? "";
  if (!resolved || !/^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(jobId)) {
    return NextResponse.json({ error: "INPUT_INVALID" }, { status: 400 });
  }
  try { return NextResponse.json(await readFlightHubManagementWriteJob(resolved.projectId, resolved.connectorInstanceId, jobId)); }
  catch (error) { return NextResponse.json({ error: error instanceof Error ? error.message : "FLIGHTHUB_MANAGEMENT_WRITE_FAILED" }, { status: 404 }); }
}
