import { NextResponse } from "next/server";
import { requireUser } from "@/lib/data";
import { parseReplayQuery } from "@/lib/project-replay-core";
import { readProjectReplay } from "@/lib/project-replay";

export async function GET(request: Request, { params }: { params: Promise<{ id: string }> }) {
  const user = await requireUser();
  const { id } = await params;
  const projectId = Number(id);
  if (!Number.isSafeInteger(projectId) || projectId <= 0) return NextResponse.json({ error: "PROJECT_NOT_FOUND" }, { status: 404 });
  let input;
  try { input = parseReplayQuery(new URL(request.url)); }
  catch (error) { return NextResponse.json({ error: error instanceof Error ? error.message : "INVALID_REPLAY_QUERY" }, { status: 400 }); }
  const replay = await readProjectReplay(user.id, projectId, input);
  if (!replay) return NextResponse.json({ error: "PROJECT_NOT_FOUND" }, { status: 404 });
  return NextResponse.json(replay, { headers: { "Cache-Control": "private, no-store", "X-AeroSight-Mode": "replay" } });
}
