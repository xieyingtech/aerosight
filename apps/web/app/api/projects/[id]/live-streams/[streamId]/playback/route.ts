import { NextResponse } from "next/server";

import { getLiveStreamPlayback } from "@/lib/live-streams";

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ id: string; streamId: string }> }
) {
  try {
    const { id, streamId } = await params;
    return NextResponse.json(await getLiveStreamPlayback(Number(id), Number(streamId)));
  } catch {
    return NextResponse.json({ error: "Unable to access live stream" }, { status: 403 });
  }
}
