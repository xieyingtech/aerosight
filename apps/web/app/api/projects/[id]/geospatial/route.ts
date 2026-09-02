import { NextResponse } from "next/server";

import { FlightHubGeospatialError, readFlightHubGeospatial } from "@/lib/dji-flighthub-geospatial";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const headers = { "Cache-Control": "private, no-store, max-age=0" };

export async function GET(_request: Request, context: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await context.params;
    return NextResponse.json(await readFlightHubGeospatial(Number(id)), { headers });
  } catch (error) {
    const code = error instanceof FlightHubGeospatialError ? error.safeCode : "access_denied";
    return NextResponse.json({ error: { code } }, { status: code === "invalid_project" ? 400 : 403, headers });
  }
}
