import { NextResponse } from "next/server";

import { submitDeviceCommand } from "@/lib/device-commands";
import { assertLiveControlRequest } from "@/lib/replay-policy";

export async function POST(request: Request, { params }: { params: Promise<{ id: string; deviceId: string }> }) {
  const { id, deviceId } = await params;
  const projectId = Number(id);
  const parsedDeviceId = Number(deviceId);
  const body = await request.json() as Record<string, unknown>;
  if (!Number.isInteger(projectId) || projectId <= 0 || !Number.isInteger(parsedDeviceId) || parsedDeviceId <= 0
      || typeof body.capabilityCode !== "string" || !body.capabilityCode.trim()
      || typeof body.commandKey !== "string" || !body.commandKey.trim()
      || !body.parameters || typeof body.parameters !== "object" || Array.isArray(body.parameters)
      || typeof body.idempotencyKey !== "string" || body.idempotencyKey.length < 8 || body.idempotencyKey.length > 128
      || typeof body.reason !== "string" || !body.reason.trim()
      || (body.approvalRequestId !== undefined && body.approvalRequestId !== null
          && (typeof body.approvalRequestId !== "string"
              || !/^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(body.approvalRequestId)))
      || (body.safetyPolicyVersionId !== undefined && body.safetyPolicyVersionId !== null
          && (!Number.isSafeInteger(body.safetyPolicyVersionId) || (body.safetyPolicyVersionId as number) <= 0))
      || (body.deadlineSeconds !== undefined && (typeof body.deadlineSeconds !== "number" || !Number.isFinite(body.deadlineSeconds)))) {
    return NextResponse.json({ error: "DEVICE_COMMAND_INPUT_INVALID" }, { status: 400 });
  }
  try {
    assertLiveControlRequest(request);
    const result = await submitDeviceCommand({
      projectId, deviceId: parsedDeviceId, capabilityCode: body.capabilityCode,
      commandKey: body.commandKey, parameters: body.parameters as Record<string, unknown>,
      idempotencyKey: body.idempotencyKey, confirmation: typeof body.confirmation === "string" ? body.confirmation : null,
      reason: body.reason, deadlineSeconds: typeof body.deadlineSeconds === "number" ? body.deadlineSeconds : undefined,
      approvalRequestId: typeof body.approvalRequestId === "string" ? body.approvalRequestId : null,
      safetyPolicyVersionId: typeof body.safetyPolicyVersionId === "number" ? body.safetyPolicyVersionId : null,
      requestId: request.headers.get("x-request-id")
    });
    return NextResponse.json(result);
  } catch (error) {
    const code = error instanceof Error ? error.message : "DEVICE_COMMAND_FAILED";
    const forbidden = code.includes("DENIED") || code.includes("NOT_GRANTED");
    return NextResponse.json({ error: code }, { status: forbidden ? 403 : 409 });
  }
}
