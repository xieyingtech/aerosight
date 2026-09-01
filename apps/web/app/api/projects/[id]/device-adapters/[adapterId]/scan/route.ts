import { NextRequest, NextResponse } from "next/server";

import { requestFlightHubSync } from "@/lib/dji-flighthub-lifecycle";

export async function POST(request: NextRequest, context: { params: Promise<{ id: string; adapterId: string }> }) {
  try {
    const { id, adapterId } = await context.params;
    return NextResponse.json(await requestFlightHubSync(
      Number(id), adapterId, request.headers.get("x-request-id")
    ), { status: 202 });
  } catch (error) {
    const code = error instanceof Error ? error.message : "CONNECTOR_SCAN_FAILED";
    return NextResponse.json({ error: code }, { status: code.includes("access_denied") ? 403 : 409 });
  }
}
