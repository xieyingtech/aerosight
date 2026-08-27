import { NextResponse } from "next/server";
import { getMissionAuditTrace } from "@/lib/mission-audit-traces";

export async function GET(_request: Request, { params }: { params: Promise<{ id: string; runId: string }> }) {
  const { id, runId } = await params;
  return NextResponse.json(await getMissionAuditTrace(Number(id), Number(runId)));
}
