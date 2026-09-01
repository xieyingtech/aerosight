import { NextRequest, NextResponse } from "next/server";

import { updateDiscoveryStatus } from "@/lib/device-discoveries";

export async function PATCH(request: NextRequest, context: { params: Promise<{ id: string; identityId: string }> }) {
  try {
    const { id, identityId } = await context.params;
    const body = await request.json() as { action?: unknown };
    if (body.action !== "ignore" && body.action !== "review" && body.action !== "rematch") {
      throw new Error("DISCOVERY_ACTION_INVALID");
    }
    return NextResponse.json(await updateDiscoveryStatus(
      Number(id), Number(identityId), body.action, request.headers.get("x-request-id")
    ));
  } catch (error) {
    const code = error instanceof Error ? error.message : "DISCOVERY_UPDATE_FAILED";
    return NextResponse.json({ error: code }, { status: code === "PROJECT_ACCESS_DENIED" ? 403 : 400 });
  }
}
