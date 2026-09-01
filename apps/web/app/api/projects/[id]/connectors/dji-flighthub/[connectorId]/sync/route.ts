import { NextRequest, NextResponse } from "next/server";

import { FlightHubConnectionError } from "@/lib/dji-flighthub-connection-core";
import { requestFlightHubSync } from "@/lib/dji-flighthub-lifecycle";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const noStoreHeaders = { "Cache-Control": "no-store, max-age=0" };

export async function POST(
  request: NextRequest,
  context: { params: Promise<{ id: string; connectorId: string }> }
) {
  try {
    const { id, connectorId } = await context.params;
    return NextResponse.json(
      await requestFlightHubSync(Number(id), connectorId, request.headers.get("x-request-id")),
      { status: 202, headers: noStoreHeaders }
    );
  } catch (error) {
    const code = error instanceof FlightHubConnectionError ? error.safeCode : "access_denied";
    const status = code === "access_denied" ? 403 : code === "connector_not_found" ? 404
      : code === "connector_disabled" ? 409 : 503;
    return NextResponse.json({ error: { code } }, { status, headers: noStoreHeaders });
  }
}
