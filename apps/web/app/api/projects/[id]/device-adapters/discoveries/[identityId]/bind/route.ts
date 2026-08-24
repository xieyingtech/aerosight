import { NextRequest, NextResponse } from "next/server";

import { bindDiscoveredDevice } from "@/lib/device-adapters";

export async function POST(
  request: NextRequest,
  context: { params: Promise<{ id: string; identityId: string }> }
) {
  try {
    const { id, identityId } = await context.params;
    return NextResponse.json(await bindDiscoveredDevice(
      Number(id), Number(identityId), await request.json(), request.headers.get("x-request-id")
    ));
  } catch {
    return NextResponse.json({ error: "Invalid or unauthorized device binding" }, { status: 400 });
  }
}
