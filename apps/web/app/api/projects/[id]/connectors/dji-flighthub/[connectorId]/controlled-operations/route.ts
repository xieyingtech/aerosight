import { NextResponse } from "next/server";
import { readFlightHubControlledOperations } from "@/lib/dji-flighthub-controlled-operations";

export async function GET(_request: Request, { params }: { params: Promise<{ id: string; connectorId: string }> }) {
  const { id, connectorId } = await params;
  try { return NextResponse.json(await readFlightHubControlledOperations(Number(id), connectorId)); }
  catch (error) { return NextResponse.json({ error: error instanceof Error ? error.message : "FLIGHTHUB_CONTROLLED_OPERATIONS_FAILED" }, { status: 404 }); }
}
