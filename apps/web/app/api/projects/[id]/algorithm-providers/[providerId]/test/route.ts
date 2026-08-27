import { NextResponse } from "next/server";
import { testAlgorithmProviderEndpoint } from "@/lib/algorithm-providers";

export async function POST(_request: Request, { params }: { params: Promise<{ id: string; providerId: string }> }) {
  const { id, providerId } = await params;
  try { return NextResponse.json(await testAlgorithmProviderEndpoint(Number(id), Number(providerId))); }
  catch (error) { const code = error instanceof Error ? error.message : "ALGORITHM_PROVIDER_TEST_FAILED"; return NextResponse.json({ error: code }, { status: code === "PROJECT_ACCESS_DENIED" ? 403 : 400 }); }
}
