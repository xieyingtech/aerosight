import { NextResponse } from "next/server";
import type { MissionAction } from "@/lib/mission-workbench-core";
import { controlMissionRun } from "@/lib/task-runs";

const supported = new Set<MissionAction>(["pause", "resume", "cancel", "emergency_stop", "approve"]);

export async function POST(request: Request, { params }: { params: Promise<{ id: string; runId: string }> }) {
  const { id, runId } = await params;
  const body = await request.json() as { action?: MissionAction; expectedVersion?: number; reason?: string };
  if (!body.action || !supported.has(body.action) || !Number.isInteger(body.expectedVersion) || !body.reason?.trim()) {
    return NextResponse.json({ error: "MISSION_CONTROL_INPUT_INVALID" }, { status: 400 });
  }
  try {
    const result = await controlMissionRun({ projectId: Number(id), taskRunId: Number(runId),
      action: body.action, expectedVersion: body.expectedVersion!, reason: body.reason });
    return NextResponse.json(result);
  } catch (error) {
    const code = error instanceof Error ? error.message : "MISSION_CONTROL_FAILED";
    return NextResponse.json({ error: code }, { status: code === "PROJECT_ACCESS_DENIED" ? 403 : 409 });
  }
}
