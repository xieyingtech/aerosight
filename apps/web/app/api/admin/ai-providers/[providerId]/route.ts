import { NextResponse } from "next/server";
import { deleteAIProvider, updateAIProvider } from "@/lib/ai-providers";

export async function PATCH(request: Request, { params }: { params: Promise<{ providerId: string }> }) {
  const { providerId } = await params;
  try { return NextResponse.json(await updateAIProvider(Number(providerId), await request.json(), request.headers.get("x-request-id"))); }
  catch (error) { return NextResponse.json({ error: error instanceof Error ? error.message : "AI_PROVIDER_FAILED" }, { status: 400 }); }
}

export async function DELETE(request: Request, { params }: { params: Promise<{ providerId: string }> }) {
  const { providerId } = await params;
  try { return NextResponse.json(await deleteAIProvider(Number(providerId), request.headers.get("x-request-id"))); }
  catch (error) { return NextResponse.json({ error: error instanceof Error ? error.message : "AI_PROVIDER_FAILED" }, { status: 400 }); }
}
