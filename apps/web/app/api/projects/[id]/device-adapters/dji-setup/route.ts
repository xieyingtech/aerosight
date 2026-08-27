import { NextRequest, NextResponse } from "next/server";

import { createDjiAdapterSetup } from "@/lib/device-adapters";

export async function POST(request: NextRequest, context: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await context.params;
    const adapter = await createDjiAdapterSetup(
      Number(id),
      await request.json(),
      request.headers.get("x-request-id")
    );
    return NextResponse.json(adapter, { status: 201 });
  } catch (error) {
    const code = error instanceof Error ? error.message : "DJI_ADAPTER_SETUP_FAILED";
    const issues = error instanceof Error && "issues" in error
      ? (error as Error & { issues: unknown }).issues
      : undefined;
    const safeCode = code === "NETWORK_PROFILE_INVALID" || code === "PROJECT_ACCESS_DENIED"
      ? code
      : "DJI_ADAPTER_SETUP_INVALID";
    return NextResponse.json(
      { error: safeCode, issues },
      { status: safeCode === "PROJECT_ACCESS_DENIED" ? 403 : 400 }
    );
  }
}
