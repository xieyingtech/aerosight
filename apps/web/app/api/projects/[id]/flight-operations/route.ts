import { NextResponse } from "next/server";

import {
  FlightHubFlightOperationsError,
  readFlightHubFlightOperations,
} from "@/lib/dji-flighthub-flight-operations";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const headers = { "Cache-Control": "private, no-store, max-age=0" };

export async function GET(_request: Request, context: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await context.params;
    return NextResponse.json(await readFlightHubFlightOperations(Number(id)), { headers });
  } catch (error) {
    const code = error instanceof FlightHubFlightOperationsError ? error.safeCode : "access_denied";
    return NextResponse.json({ error: { code } }, { status: code === "invalid_project" ? 400 : 403, headers });
  }
}
