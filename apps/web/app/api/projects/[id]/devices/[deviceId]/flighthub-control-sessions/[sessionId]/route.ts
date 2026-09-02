import { NextResponse } from "next/server";

import { heartbeatFlightHubControlSession, releaseFlightHubControlSession } from "@/lib/dji-flighthub-control-sessions";

const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const safeErrors = new Set([
  "FLIGHTHUB_CONTROL_HEARTBEAT_REJECTED", "FLIGHTHUB_CONTROL_SESSION_NOT_FOUND",
  "FLIGHTHUB_CONTROL_OPERATION_RATE_LIMITED"
]);

export async function PATCH(request: Request, { params }: { params: Promise<{ id: string; deviceId: string; sessionId: string }> }) {
  const route = await params;
  const projectId = Number(route.id);
  const deviceId = Number(route.deviceId);
  const body = await request.json().catch(() => null) as Record<string, unknown> | null;
  if (!Number.isSafeInteger(projectId) || projectId <= 0 || !Number.isSafeInteger(deviceId) || deviceId <= 0
      || !uuidPattern.test(route.sessionId) || !body || (body.action !== "heartbeat" && body.action !== "release")) {
    return NextResponse.json({ error: "FLIGHTHUB_CONTROL_SESSION_INPUT_INVALID" }, { status: 400 });
  }
  try {
    const result = body.action === "heartbeat"
      ? await heartbeatFlightHubControlSession(projectId, deviceId, route.sessionId)
      : await releaseFlightHubControlSession(projectId, deviceId, route.sessionId, request.headers.get("x-request-id"));
    return NextResponse.json(result, { status: body.action === "release" ? 202 : 200 });
  } catch (error) {
    const candidate = error instanceof Error ? error.message : "";
    const message = safeErrors.has(candidate) ? candidate : "FLIGHTHUB_CONTROL_SESSION_FAILED";
    return NextResponse.json({ error: message }, { status: message.includes("RATE") ? 429 : 409 });
  }
}
