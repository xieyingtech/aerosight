import { NextResponse } from "next/server";
import { updateAlgorithmProvider } from "@/lib/algorithm-providers";

export async function PATCH(request: Request, { params }: { params: Promise<{ id: string; providerId: string }> }) {
  const { id, providerId } = await params;
  try { return NextResponse.json(await updateAlgorithmProvider(Number(id), Number(providerId), await request.json())); }
  catch (error) { const code = error instanceof Error ? error.message : "ALGORITHM_PROVIDER_FAILED"; return NextResponse.json({ error: code }, { status: code === "PROJECT_ACCESS_DENIED" ? 403 : 400 }); }
}
