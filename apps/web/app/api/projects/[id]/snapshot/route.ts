import { NextResponse } from "next/server";
import { requireUser } from "@/lib/data";
import { readProjectSituationSnapshot } from "@/lib/project-snapshot";

export async function GET(_request: Request, { params }: { params: Promise<{ id: string }> }) {
  const user = await requireUser();
  const { id } = await params;
  const projectId = Number(id);
  if (!Number.isSafeInteger(projectId) || projectId <= 0) {
    return NextResponse.json({ error: "PROJECT_NOT_FOUND" }, { status: 404 });
  }
  const snapshot = await readProjectSituationSnapshot(user.id, projectId);
  if (!snapshot) return NextResponse.json({ error: "PROJECT_NOT_FOUND" }, { status: 404 });
  return NextResponse.json(snapshot, {
    headers: { "Cache-Control": "private, no-store", "X-AeroSight-Snapshot": "repeatable-read" }
  });
}
