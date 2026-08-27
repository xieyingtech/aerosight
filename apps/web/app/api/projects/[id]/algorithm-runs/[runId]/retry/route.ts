import { NextResponse } from "next/server";
import { retryAlgorithmRun } from "@/lib/algorithm-runs";

export async function POST(request: Request, { params }: { params: Promise<{ id: string; runId: string }> }) {
  const { id, runId } = await params;
  try {
    return NextResponse.json(await retryAlgorithmRun(Number(id), runId, request.headers.get("x-request-id")));
  } catch (error) {
    const code = error instanceof Error ? error.message : "ALGORITHM_RUN_RETRY_FAILED";
    return NextResponse.json({ error: code }, { status: code === "PROJECT_ACCESS_DENIED" ? 403 : 409 });
  }
}
