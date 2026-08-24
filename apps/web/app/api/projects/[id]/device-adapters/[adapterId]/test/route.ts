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
  } catch {
    return NextResponse.json({ error: "Unable to test device adapter" }, { status: 400 });
  }
}
