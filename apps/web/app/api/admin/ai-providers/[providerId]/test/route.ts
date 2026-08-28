import { NextResponse } from "next/server";
import { testAIProvider } from "@/lib/ai-providers";

export async function POST(request: Request, { params }: { params: Promise<{ providerId: string }> }) {
  const { providerId } = await params;
  try { return NextResponse.json(await testAIProvider(Number(providerId), request.headers.get("x-request-id"))); }
  catch (error) { return NextResponse.json({ error: error instanceof Error ? error.message : "AI_PROVIDER_TEST_FAILED" }, { status: 400 }); }
}
