import { NextResponse } from "next/server";
import { createAlgorithmProvider, listAlgorithmProviders } from "@/lib/algorithm-providers";

export async function GET(_request: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  try { return NextResponse.json(await listAlgorithmProviders(Number(id))); }
  catch (error) { return NextResponse.json({ error: error instanceof Error ? error.message : "ALGORITHM_PROVIDER_FAILED" }, { status: 403 }); }
}
export async function POST(request: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  try { return NextResponse.json(await createAlgorithmProvider(Number(id), await request.json()), { status: 201 }); }
  catch (error) { const code = error instanceof Error ? error.message : "ALGORITHM_PROVIDER_FAILED"; return NextResponse.json({ error: code }, { status: code === "PROJECT_ACCESS_DENIED" ? 403 : 400 }); }
}
