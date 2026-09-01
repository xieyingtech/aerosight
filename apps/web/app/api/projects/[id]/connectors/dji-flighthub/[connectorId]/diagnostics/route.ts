import { NextResponse } from "next/server";

import { FlightHubConnectionError } from "@/lib/dji-flighthub-connection-core";
import { readFlightHubConnectorDiagnostics } from "@/lib/dji-flighthub-diagnostics";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const headers = { "Cache-Control": "private, no-store, max-age=0" };

export async function GET(
  _request: Request,
  context: { params: Promise<{ id: string; connectorId: string }> }
) {
  try {
    const { id, connectorId } = await context.params;
    return NextResponse.json(
      await readFlightHubConnectorDiagnostics(Number(id), connectorId),
      { headers }
    );
  } catch (error) {
    const code = error instanceof FlightHubConnectionError ? error.safeCode : "access_denied";
    return NextResponse.json(
      { error: { code } },
      { status: code === "connector_not_found" ? 404 : 403, headers }
    );
  }
}
