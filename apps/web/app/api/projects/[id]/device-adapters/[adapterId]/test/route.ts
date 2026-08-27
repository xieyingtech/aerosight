import { NextRequest, NextResponse } from "next/server";

import { testDeviceAdapterConnection } from "@/lib/device-adapters";

export async function POST(
  request: NextRequest,
  context: { params: Promise<{ id: string; adapterId: string }> }
) {
  try {
    const { id, adapterId } = await context.params;
    return NextResponse.json(await testDeviceAdapterConnection(
      Number(id), Number(adapterId), request.headers.get("x-request-id")
    ));
  } catch (error) {
    const code = error instanceof Error ? error.message : "DEVICE_ADAPTER_TEST_FAILED";
    const safeCode = new Set(["PROJECT_ACCESS_DENIED", "DEVICE_ADAPTER_NOT_FOUND"]).has(code)
      ? code
      : "DEVICE_ADAPTER_TEST_FAILED";
    return NextResponse.json({ error: safeCode }, { status: safeCode === "PROJECT_ACCESS_DENIED" ? 403 : 400 });
  }
}
