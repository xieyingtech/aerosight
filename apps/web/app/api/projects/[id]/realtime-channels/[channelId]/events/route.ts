import { NextResponse } from "next/server";

import { requireUser } from "@/lib/data";
import { db } from "@/lib/db";
import { createRealtimeSubscriptionStream, resolveRealtimeSubscription } from "@/lib/realtime-subscriptions";

export const dynamic = "force-dynamic";

export async function GET(request: Request, { params }: {
  params: Promise<{ id: string; channelId: string }>;
}) {
  const user = await requireUser();
  const { id, channelId } = await params;
  const projectId = Number(id);
  if (!Number.isSafeInteger(projectId) || projectId <= 0 || !channelId || channelId.length > 512) {
    return NextResponse.json({ error: "REALTIME_CHANNEL_NOT_FOUND" }, { status: 404 });
  }
  const client = await db.connect();
  try {
    await resolveRealtimeSubscription(client, user.id, projectId, channelId);
  } catch {
    return NextResponse.json({ error: "REALTIME_CHANNEL_NOT_FOUND" }, { status: 404 });
  } finally {
    client.release();
  }
  const url = new URL(request.url);
  const afterCursor = request.headers.get("Last-Event-ID") ?? url.searchParams.get("cursor");
  return new Response(createRealtimeSubscriptionStream({
    userId: user.id, projectId, stableChannelId: channelId, afterCursor, signal: request.signal
  }), {
    headers: {
      "Content-Type": "text/event-stream; charset=utf-8",
      "Cache-Control": "private, no-cache, no-transform",
      "Connection": "keep-alive",
      "X-Accel-Buffering": "no"
    }
  });
}
