import { NextRequest, NextResponse } from "next/server";
import { createTaskDraft, publishTaskDraft, updateTaskDraft } from "@/lib/task-versions";

export async function POST(request: NextRequest, { params }: { params: Promise<{ id: string; taskId: string }> }) {
  try {
    const { id,taskId } = await params;
    const projectId = Number(id);
    const task = Number(taskId);
    const body = await request.json();
    if (body.action === "create") return NextResponse.json(await createTaskDraft(projectId,task,request.headers.get("x-request-id")));
    if (body.action === "save") return NextResponse.json(await updateTaskDraft(projectId,task,Number(body.versionId),body.definition,request.headers.get("x-request-id")));
    if (body.action === "publish") return NextResponse.json(await publishTaskDraft(projectId,Number(body.versionId),request.headers.get("x-request-id")));
    return NextResponse.json({ error: "TASK_VERSION_ACTION_INVALID" }, { status: 400 });
  } catch (error) {
    const message = error instanceof Error ? error.message : "TASK_VERSION_UPDATE_FAILED";
    return NextResponse.json({ error: message }, { status: /PERMISSION|AUTH|ACCESS/.test(message) ? 403 : 400 });
  }
}
