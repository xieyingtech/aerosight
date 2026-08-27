import { NextResponse } from "next/server";

import { startLiveStream } from "@/lib/live-streams";
import { assertLiveControlRequest, ReplayControlForbiddenError } from "@/lib/replay-policy";

export async function POST(
  request: Request,
  { params }: { params: Promise<{ id: string; deviceId: string }> }
) {
  try {
    assertLiveControlRequest(request);
    const { id, deviceId } = await params;
    const body = await request.json().catch(() => ({})) as { streamKey?: string; taskRunId?: number };
    return NextResponse.json(await startLiveStream(
      Number(id), Number(deviceId), body, request.headers.get("x-request-id")
    ), { status: 201 });
  } catch (error) {
    if (error instanceof ReplayControlForbiddenError) {
      return NextResponse.json({ error: error.code }, { status: 409 });
    }
    return NextResponse.json({ error: "Unable to start live stream" }, { status: 400 });
  }
}
