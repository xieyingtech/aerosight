import { NextRequest, NextResponse } from "next/server";

import { setDeviceAdapterEnabled, updateDjiAdapterCredentials } from "@/lib/device-adapters";

export async function PATCH(
  request: NextRequest,
  context: { params: Promise<{ id: string; adapterId: string }> }
) {
  try {
    const { id, adapterId } = await context.params;
    const body = await request.json() as { enabled?: unknown; credentials?: unknown };
    if (body.credentials !== undefined) return NextResponse.json(await updateDjiAdapterCredentials(
      Number(id), Number(adapterId), body.credentials, request.headers.get("x-request-id")
    ));
    if (typeof body.enabled !== "boolean") throw new Error("enabled or credentials is required");
    return NextResponse.json(await setDeviceAdapterEnabled(
      Number(id), Number(adapterId), body.enabled, request.headers.get("x-request-id")
    ));
  } catch {
    return NextResponse.json({ error: "Invalid or unauthorized adapter update" }, { status: 400 });
  }
}
