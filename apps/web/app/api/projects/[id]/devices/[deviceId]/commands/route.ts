import { NextResponse } from "next/server";
import { requireCurrentProjectPermission } from "@/lib/data";
import { assertLiveControlRequest, ReplayControlForbiddenError } from "@/lib/replay-policy";

export async function POST(request: Request, { params }: { params: Promise<{ id: string; deviceId: string }> }) {
  const { id } = await params;
  await requireCurrentProjectPermission(Number(id), "mission:operate");
  try { assertLiveControlRequest(request); }
  catch (error) {
    if (error instanceof ReplayControlForbiddenError) return NextResponse.json({ error: error.code }, { status: 409 });
    throw error;
  }
  return NextResponse.json({ error: "DEVICE_COMMANDS_NOT_ENABLED" }, { status: 503 });
}
