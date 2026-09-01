import { NextRequest, NextResponse } from "next/server";
import { ZodError } from "zod";

import { FlightHubConnectionError } from "@/lib/dji-flighthub-connection-core";
import { updateFlightHubToken } from "@/lib/dji-flighthub-lifecycle";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const noStoreHeaders = { "Cache-Control": "no-store, max-age=0" };

export async function PUT(
  request: NextRequest,
  context: { params: Promise<{ id: string; connectorId: string }> }
) {
  try {
    const { id, connectorId } = await context.params;
    return NextResponse.json(await updateFlightHubToken(
      Number(id), connectorId, await request.json(), request.headers.get("x-request-id")
    ), { headers: noStoreHeaders });
  } catch (error) {
    if (error instanceof ZodError || error instanceof SyntaxError) {
      return NextResponse.json({ error: { code: "invalid_request" } }, { status: 400, headers: noStoreHeaders });
    }
    const code = error instanceof FlightHubConnectionError ? error.safeCode : "access_denied";
    const status = code === "access_denied" || code === "scope_forbidden" ? 403
      : code === "credential_invalid" ? 401
      : code === "connector_not_found" ? 404
      : code === "rate_limited" ? 429
      : code === "request_timeout" ? 504
      : code === "project_access_changed" ? 409 : 503;
    return NextResponse.json({ error: { code } }, { status, headers: noStoreHeaders });
  }
}
