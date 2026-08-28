import { NextResponse } from "next/server";
import { createAIProvider, listAIProviders } from "@/lib/ai-providers";

export async function GET() {
  try { return NextResponse.json(await listAIProviders()); }
  catch (error) { return NextResponse.json({ error: error instanceof Error ? error.message : "AI_PROVIDER_FAILED" }, { status: 403 }); }
}

export async function POST(request: Request) {
  try { return NextResponse.json(await createAIProvider(await request.json(), request.headers.get("x-request-id")), { status: 201 }); }
  catch (error) { return NextResponse.json({ error: error instanceof Error ? error.message : "AI_PROVIDER_FAILED" }, { status: 400 }); }
}
