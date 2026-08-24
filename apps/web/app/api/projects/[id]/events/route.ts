import { NextResponse } from "next/server";
import { db } from "@/lib/db";
import { requireUser } from "@/lib/data";
import { canAccessProjectStream, createProjectEventStream } from "@/lib/project-stream";

export const dynamic = "force-dynamic";

export async function GET(request: Request, { params }: { params: Promise<{ id: string }> }) {
  const user = await requireUser();
  const { id } = await params;
  const projectId = Number(id);
  if (!Number.isSafeInteger(projectId) || projectId <= 0) {
    return NextResponse.json({ error: "PROJECT_NOT_FOUND" }, { status: 404 });
  }
  const client = await db.connect();
  const allowed = await canAccessProjectStream(client, user.id, projectId).finally(() => client.release());
  if (!allowed) return NextResponse.json({ error: "PROJECT_NOT_FOUND" }, { status: 404 });
  const url = new URL(request.url);
  const afterCursor = request.headers.get("Last-Event-ID") ?? url.searchParams.get("cursor");
  return new Response(createProjectEventStream({ userId: user.id, projectId, afterCursor, signal: request.signal }), {
    headers: {
      "Content-Type": "text/event-stream; charset=utf-8",
      "Cache-Control": "private, no-cache, no-transform",
      "Connection": "keep-alive",
      "X-Accel-Buffering": "no"
    }
  });
}
