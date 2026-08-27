import { NextResponse } from "next/server";
import { runEmergencyStopDrill } from "@/lib/mission-audit-traces";

export async function POST(request: Request, { params }: { params: Promise<{ id: string; runId: string }> }) {
  const { id, runId } = await params;
  const body = await request.json();
  return NextResponse.json(await runEmergencyStopDrill({
    projectId: Number(id), taskRunId: Number(runId), dryRun: body.dryRun === true,
    outcome: body.outcome, requestId: request.headers.get("x-request-id")
  }));
}
