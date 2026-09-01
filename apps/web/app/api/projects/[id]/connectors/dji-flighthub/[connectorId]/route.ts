import { NextRequest, NextResponse } from "next/server";

import { FlightHubConnectionError } from "@/lib/dji-flighthub-connection-core";
import { disconnectFlightHubConnection } from "@/lib/dji-flighthub-lifecycle";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const noStoreHeaders = { "Cache-Control": "no-store, max-age=0" };

export async function DELETE(
  request: NextRequest,
  context: { params: Promise<{ id: string; connectorId: string }> }
) {
  try {
    const { id, connectorId } = await context.params;
    return NextResponse.json(
      await disconnectFlightHubConnection(Number(id), connectorId, request.headers.get("x-request-id")),
      { headers: noStoreHeaders }
    );
  } catch (error) {
    const code = error instanceof FlightHubConnectionError ? error.safeCode : "access_denied";
    const status = code === "access_denied" ? 403 : code === "connector_not_found" ? 404 : 503;
    return NextResponse.json({ error: { code } }, { status, headers: noStoreHeaders });
  }
}
