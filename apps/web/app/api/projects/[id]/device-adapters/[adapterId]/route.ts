import { NextRequest, NextResponse } from "next/server";

import { setDeviceAdapterEnabled } from "@/lib/device-adapters";

export async function PATCH(
  request: NextRequest,
  context: { params: Promise<{ id: string; adapterId: string }> }
) {
  try {
    const { id, adapterId } = await context.params;
    const body = await request.json() as { enabled?: unknown };
    if (typeof body.enabled !== "boolean") throw new Error("enabled is required");
    return NextResponse.json(await setDeviceAdapterEnabled(
      Number(id), Number(adapterId), body.enabled, request.headers.get("x-request-id")
    ));
  } catch {
    return NextResponse.json({ error: "Invalid or unauthorized adapter update" }, { status: 400 });
  }
}
