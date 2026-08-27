import { NextResponse } from "next/server";

import { stopLiveStream } from "@/lib/live-streams";
import { assertLiveControlRequest, ReplayControlForbiddenError } from "@/lib/replay-policy";

export async function POST(
  request: Request,
  { params }: { params: Promise<{ id: string; streamId: string }> }
) {
  try {
    assertLiveControlRequest(request);
    const { id, streamId } = await params;
    return NextResponse.json(await stopLiveStream(
      Number(id), Number(streamId), request.headers.get("x-request-id")
    ));
  } catch (error) {
    if (error instanceof ReplayControlForbiddenError) {
      return NextResponse.json({ error: error.code }, { status: 409 });
    }
    return NextResponse.json({ error: "Unable to stop live stream" }, { status: 400 });
  }
}
