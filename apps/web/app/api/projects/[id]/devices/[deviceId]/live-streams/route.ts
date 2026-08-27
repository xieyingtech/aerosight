import { NextResponse } from "next/server";

import { startLiveStream } from "@/lib/live-streams";
import { assertLiveControlRequest } from "@/lib/replay-policy";

export async function POST(request: Request, { params }: { params: Promise<{ id: string; deviceId: string }> }) {
  try {
    assertLiveControlRequest(request);
    const { id, deviceId } = await params;
    const body = await request.json().catch(() => ({})) as { streamKey?: string };
    return NextResponse.json(await startLiveStream(Number(id), Number(deviceId), { streamKey: body.streamKey }, request.headers.get("x-request-id")));
  } catch (error) {
    return NextResponse.json({ error: error instanceof Error ? error.message : "LIVE_STREAM_START_FAILED" }, { status: 409 });
  }
}
