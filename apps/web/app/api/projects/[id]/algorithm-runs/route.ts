import { NextResponse } from "next/server";

import { startAlgorithmRun } from "@/lib/algorithm-runs";

export async function POST(request: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  try {
    return NextResponse.json(await startAlgorithmRun(Number(id), await request.json(), request.headers.get("x-request-id")), { status: 202 });
  } catch (error) {
    const code = error instanceof Error ? error.message : "ALGORITHM_RUN_CREATE_FAILED";
    return NextResponse.json({ error: code }, { status: code === "PROJECT_ACCESS_DENIED" ? 403 : 400 });
  }
}
