import { NextRequest, NextResponse } from "next/server";
import { ZodError } from "zod";

import { discoverFlightHubProjects } from "@/lib/dji-flighthub-discovery";
import { FlightHubDiscoveryError } from "@/lib/dji-flighthub-discovery-core";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const noStoreHeaders = { "Cache-Control": "no-store, max-age=0" };

function errorStatus(error: FlightHubDiscoveryError) {
  if (error.safeCode === "access_denied" || error.safeCode === "scope_forbidden") return 403;
  if (error.safeCode === "credential_invalid") return 401;
  if (error.safeCode === "rate_limited") return 429;
  if (error.safeCode === "request_timeout") return 504;
  if (error.safeCode === "scope_not_found") return 404;
  return 502;
}

export async function POST(
  request: NextRequest,
  context: { params: Promise<{ id: string }> }
) {
  try {
    const { id } = await context.params;
    const projects = await discoverFlightHubProjects(
      Number(id),
      await request.json(),
      request.headers.get("x-request-id")
    );
    return NextResponse.json({ projects }, { headers: noStoreHeaders });
  } catch (error) {
    if (error instanceof FlightHubDiscoveryError) {
      const headers: Record<string, string> = { ...noStoreHeaders };
      if (error.retryAfterSeconds !== undefined) {
        headers["Retry-After"] = String(error.retryAfterSeconds);
      }
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
