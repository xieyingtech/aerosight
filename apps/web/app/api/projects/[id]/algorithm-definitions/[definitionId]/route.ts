import { NextResponse } from "next/server";

import { saveAlgorithmDefinition } from "@/lib/algorithm-definitions";

export async function PUT(
  request: Request,
  { params }: { params: Promise<{ id: string; definitionId: string }> }
) {
  const { id, definitionId } = await params;
  try {
    const body = await request.json() as { definition?: unknown; configuration?: unknown };
    return NextResponse.json(await saveAlgorithmDefinition(
      Number(id),
      Number(definitionId),
      body.definition,
      body.configuration,
      request.headers.get("x-request-id")
    ));
  } catch (error) {
    const code = error instanceof Error ? error.message : "ALGORITHM_DEFINITION_SAVE_FAILED";
    return NextResponse.json({ error: code }, { status: code === "PROJECT_ACCESS_DENIED" ? 403 : 400 });
  }
}
