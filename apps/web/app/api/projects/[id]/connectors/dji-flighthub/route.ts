import { NextRequest, NextResponse } from "next/server";
import { ZodError } from "zod";

import { FlightHubConnectionError } from "@/lib/dji-flighthub-connection-core";
import { createFlightHubConnection } from "@/lib/dji-flighthub-connections";
import { listFlightHubConnections, listFlightHubDiscoveryActivity } from "@/lib/dji-flighthub-lifecycle";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const noStoreHeaders = { "Cache-Control": "no-store, max-age=0" };

function errorStatus(error: FlightHubConnectionError) {
  if (error.safeCode === "access_denied" || error.safeCode === "scope_forbidden") return 403;
  if (error.safeCode === "credential_invalid") return 401;
  if (error.safeCode === "scope_not_found" || error.safeCode === "project_access_changed") return 409;
  if (error.safeCode === "duplicate_connection") return 409;
  if (error.safeCode === "connector_not_found") return 404;
  if (error.safeCode === "connector_disabled") return 409;
  if (error.safeCode === "rate_limited") return 429;
  if (error.safeCode === "request_timeout") return 504;
  return 503;
}

export async function GET(
  _request: NextRequest,
  context: { params: Promise<{ id: string }> }
) {
  try {
    const { id } = await context.params;
    const projectId = Number(id);
    const [connectors, activity] = await Promise.all([
      listFlightHubConnections(projectId),
      listFlightHubDiscoveryActivity(projectId),
    ]);
    return NextResponse.json({ connectors, ...activity }, { headers: noStoreHeaders });
  } catch (error) {
    if (error instanceof FlightHubConnectionError) {
      return NextResponse.json(
        { error: { code: error.safeCode } },
        { status: errorStatus(error), headers: noStoreHeaders }
      );
    }
    return NextResponse.json(
      { error: { code: "access_denied" } },
      { status: 403, headers: noStoreHeaders }
    );
  }
}

export async function POST(
  request: NextRequest,
  context: { params: Promise<{ id: string }> }
) {
  try {
    const { id } = await context.params;
    const connector = await createFlightHubConnection(
      Number(id),
      await request.json(),
      request.headers.get("x-request-id")
    );
    return NextResponse.json(connector, { status: 201, headers: noStoreHeaders });
  } catch (error) {
    if (error instanceof FlightHubConnectionError) {
      const headers: Record<string, string> = { ...noStoreHeaders };
      if (error.retryAfterSeconds !== undefined) headers["Retry-After"] = String(error.retryAfterSeconds);
      return NextResponse.json(
        { error: { code: error.safeCode } },
        { status: errorStatus(error), headers }
      );
    }
    if (error instanceof ZodError || error instanceof SyntaxError) {
      return NextResponse.json(
        { error: { code: "invalid_request" } },
        { status: 400, headers: noStoreHeaders }
      );
    }
    return NextResponse.json(
      { error: { code: "access_denied" } },
      { status: 403, headers: noStoreHeaders }
    );
  }
}
