import { z, ZodError } from "zod";
import { NextRequest, NextResponse } from "next/server";

import { readFlightHubLiveActionJob, submitFlightHubLiveAction } from "@/lib/dji-flighthub-live-actions";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";
const headers = { "Cache-Control": "private, no-store, max-age=0" };

function safeActionError(error: unknown) {
  if (error instanceof ZodError || error instanceof SyntaxError) return { code: "invalid_request", status: 400 };
  const message = error instanceof Error ? error.message : "";
  if (message === "FLIGHTHUB_LIVE_ACTION_NOT_FOUND") return { code: "action_not_found", status: 404 };
  if (message === "IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST") return { code: "idempotency_conflict", status: 409 };
  if (message.startsWith("FLIGHTHUB_LIVE_ACTION_")) return { code: message.toLowerCase(), status: 403 };
  return { code: "access_denied", status: 403 };
}

export async function POST(request: NextRequest,
  context: { params: Promise<{ id: string; connectorId: string }> }) {
  try {
    const { id, connectorId } = await context.params;
    const projectId = Number(id);
    const connectorInstanceId = Number(connectorId);
    const body = await request.json();
    if (!body || typeof body !== "object" || Array.isArray(body) || !Number.isInteger(projectId) || projectId <= 0
      || !Number.isInteger(connectorInstanceId) || connectorInstanceId <= 0) throw new SyntaxError("invalid request");
    return NextResponse.json(await submitFlightHubLiveAction(projectId, { ...body, connectorInstanceId },
      request.headers.get("x-request-id")), { status: 202, headers });
  } catch (error) {
    const safe = safeActionError(error);
    return NextResponse.json({ error: { code: safe.code } }, { status: safe.status, headers });
  }
}

export async function GET(request: NextRequest,
  context: { params: Promise<{ id: string; connectorId: string }> }) {
  try {
    const { id, connectorId } = await context.params;
    const projectId = Number(id);
    const connectorInstanceId = Number(connectorId);
    const jobId = z.string().uuid().parse(request.nextUrl.searchParams.get("jobId"));
    if (!Number.isInteger(projectId) || projectId <= 0 || !Number.isInteger(connectorInstanceId) || connectorInstanceId <= 0) {
      throw new SyntaxError("invalid request");
    }
    return NextResponse.json(await readFlightHubLiveActionJob(projectId, connectorInstanceId, jobId), { headers });
  } catch (error) {
    const safe = safeActionError(error);
    return NextResponse.json({ error: { code: safe.code } }, { status: safe.status, headers });
  }
}
