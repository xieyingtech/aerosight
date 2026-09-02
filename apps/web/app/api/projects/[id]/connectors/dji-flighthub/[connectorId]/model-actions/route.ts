import { z, ZodError } from "zod";
import { NextRequest, NextResponse } from "next/server";

import { previewFlightHubModelDelete, readFlightHubModelDeleteJob,
  submitFlightHubModelDelete } from "@/lib/dji-flighthub-model-actions";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";
const headers = { "Cache-Control": "private, no-store, max-age=0" };

function safeActionError(error: unknown) {
  if (error instanceof ZodError || error instanceof SyntaxError) return { code: "invalid_request", status: 400 };
  const message = error instanceof Error ? error.message : "";
  if (message === "FLIGHTHUB_MODEL_DELETE_NOT_FOUND") return { code: "action_not_found", status: 404 };
  if (message === "IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST") return { code: "idempotency_conflict", status: 409 };
  if (message === "FLIGHTHUB_MODEL_DELETE_PREVIEW_CONFLICT") return { code: "preview_conflict", status: 409 };
  if (message.startsWith("FLIGHTHUB_MODEL_DELETE_")) return { code: message.toLowerCase(), status: 403 };
  return { code: "access_denied", status: 403 };
}

function ids(id: string, connectorId: string) {
  const projectId = Number(id);
  const connectorInstanceId = Number(connectorId);
  if (!Number.isInteger(projectId) || projectId <= 0 || !Number.isInteger(connectorInstanceId)
    || connectorInstanceId <= 0) throw new SyntaxError("invalid request");
  return { projectId, connectorInstanceId };
}

export async function POST(request: NextRequest,
  context: { params: Promise<{ id: string; connectorId: string }> }) {
  try {
    const params = await context.params;
    const { projectId, connectorInstanceId } = ids(params.id, params.connectorId);
    const body = await request.json();
    if (!body || typeof body !== "object" || Array.isArray(body)) throw new SyntaxError("invalid request");
    return NextResponse.json(await submitFlightHubModelDelete(projectId, { ...body, connectorInstanceId },
      request.headers.get("x-request-id")), { status: 202, headers });
  } catch (error) {
    const safe = safeActionError(error);
    return NextResponse.json({ error: { code: safe.code } }, { status: safe.status, headers });
  }
}

export async function GET(request: NextRequest,
  context: { params: Promise<{ id: string; connectorId: string }> }) {
  try {
    const params = await context.params;
    const { projectId, connectorInstanceId } = ids(params.id, params.connectorId);
    const jobId = request.nextUrl.searchParams.get("jobId");
    if (jobId) return NextResponse.json(await readFlightHubModelDeleteJob(projectId, connectorInstanceId,
      z.string().uuid().parse(jobId)), { headers });
    const targetResourceId = z.coerce.number().int().positive().parse(request.nextUrl.searchParams.get("targetResourceId"));
    const action = z.enum(["model-delete", "model-resource-delete"]).parse(request.nextUrl.searchParams.get("action"));
    return NextResponse.json(await previewFlightHubModelDelete(projectId, connectorInstanceId, targetResourceId, action),
      { headers });
  } catch (error) {
    const safe = safeActionError(error);
    return NextResponse.json({ error: { code: safe.code } }, { status: safe.status, headers });
  }
}
