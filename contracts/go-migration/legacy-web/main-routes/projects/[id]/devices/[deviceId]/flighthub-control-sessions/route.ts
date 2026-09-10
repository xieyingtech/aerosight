import { NextResponse } from "next/server";

import { createFlightHubControlSession } from "@/lib/dji-flighthub-control-sessions";

const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const safeErrors = new Set([
  "FLIGHTHUB_CONTROL_SELECTION_INVALID", "FLIGHTHUB_CONTROL_SCOPE_MISMATCH",
  "FLIGHTHUB_CONTROL_CONNECTOR_UNAVAILABLE", "FLIGHTHUB_CONTROL_NOT_ENABLED",
  "FLIGHTHUB_CONTROL_DEVICE_STALE", "FLIGHTHUB_CONTROL_SAFETY_POLICY_STALE",
  "FLIGHTHUB_CONTROL_APPROVAL_REQUIRED", "FLIGHTHUB_CONTROL_SESSION_CONFLICT",
  "IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST"
]);

export async function POST(request: Request, { params }: { params: Promise<{ id: string; deviceId: string }> }) {
  const route = await params;
  const projectId = Number(route.id);
  const deviceId = Number(route.deviceId);
  const body = await request.json().catch(() => null) as Record<string, unknown> | null;
  if (!Number.isSafeInteger(projectId) || projectId <= 0 || !Number.isSafeInteger(deviceId) || deviceId <= 0
      || !body || !Number.isSafeInteger(body.connectorInstanceId) || (body.connectorInstanceId as number) <= 0
      || typeof body.approvalRequestId !== "string" || !uuidPattern.test(body.approvalRequestId)
      || !Number.isSafeInteger(body.safetyPolicyVersionId) || (body.safetyPolicyVersionId as number) <= 0
      || typeof body.idempotencyKey !== "string" || body.idempotencyKey.length < 8 || body.idempotencyKey.length > 200) {
    return NextResponse.json({ error: "FLIGHTHUB_CONTROL_SESSION_INPUT_INVALID" }, { status: 400 });
  }
  try {
    const session = await createFlightHubControlSession({
      projectId, deviceId, connectorInstanceId: body.connectorInstanceId as number, controls: body.controls,
      approvalRequestId: body.approvalRequestId, safetyPolicyVersionId: body.safetyPolicyVersionId as number,
      idempotencyKey: body.idempotencyKey, requestId: request.headers.get("x-request-id")
    });
    return NextResponse.json(session, { status: session.reused ? 200 : 202 });
  } catch (error) {
    const candidate = error instanceof Error ? error.message : "";
    const message = safeErrors.has(candidate) ? candidate : "FLIGHTHUB_CONTROL_SESSION_FAILED";
    const status = message.includes("CONFLICT") ? 409 : message.includes("APPROVAL") ? 403 : 400;
    return NextResponse.json({ error: message }, { status });
  }
}
