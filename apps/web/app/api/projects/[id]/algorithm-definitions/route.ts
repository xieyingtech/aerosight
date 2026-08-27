import { NextResponse } from "next/server";

import { listAlgorithmCatalog } from "@/lib/algorithm-catalog";
import { createAlgorithmDefinition } from "@/lib/algorithm-definitions";

export async function GET(_request: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  try {
    return NextResponse.json({ definitions: await listAlgorithmCatalog(Number(id)) });
  } catch (error) {
    const code = error instanceof Error ? error.message : "ALGORITHM_CATALOG_FAILED";
    return NextResponse.json({ error: code }, { status: code === "PROJECT_ACCESS_DENIED" ? 403 : 400 });
  }
}

export async function POST(request: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  try {
    const body = await request.json() as { definition?: unknown; version?: unknown };
    return NextResponse.json(await createAlgorithmDefinition(Number(id), body.definition, body.version), { status: 201 });
  } catch (error) {
    const code = error instanceof Error ? error.message : "ALGORITHM_DEFINITION_FAILED";
    return NextResponse.json({ error: code }, { status: code === "PROJECT_ACCESS_DENIED" ? 403 : 400 });
  }
}
