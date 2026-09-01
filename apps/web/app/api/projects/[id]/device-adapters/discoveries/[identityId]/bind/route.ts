import { NextRequest, NextResponse } from "next/server";

import { bindDiscoveredDeviceRecord } from "@/lib/device-discoveries";

export async function POST(
  request: NextRequest,
  context: { params: Promise<{ id: string; identityId: string }> }
) {
  try {
    const { id, identityId } = await context.params;
    return NextResponse.json(await bindDiscoveredDeviceRecord(
      Number(id), Number(identityId), await request.json(), request.headers.get("x-request-id")
    ));
  } catch (error) {
    const code = error instanceof Error ? error.message : "DEVICE_BINDING_FAILED";
		return NextResponse.json({ error: code }, { status: code === "PROJECT_ACCESS_DENIED" ? 403 : code === "CONNECTOR_DISABLED" ? 409 : 400 });
  }
}
