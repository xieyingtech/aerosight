import { NextResponse } from "next/server";
import { triggerTaskRunAsCurrentUser } from "@/lib/task-triggers";

export async function POST(request: Request, { params }: { params: Promise<{ id: string; taskId: string }> }) {
  const { id, taskId } = await params;
  const projectId = Number(id);
  const numericTaskId = Number(taskId);
  if (!Number.isSafeInteger(projectId) || projectId <= 0 || !Number.isSafeInteger(numericTaskId) || numericTaskId <= 0) {
    return NextResponse.json({ error: "TASK_NOT_FOUND" }, { status: 404 });
  }
  try {
    const result = await triggerTaskRunAsCurrentUser({
      projectId,
      taskId: numericTaskId,
      invocation: await request.json(),
      requestId: request.headers.get("x-request-id")
    });
    return NextResponse.json(result, { status: result.replayed ? 200 : 201 });
  } catch (error) {
    const code = error instanceof Error ? error.message : "TASK_TRIGGER_FAILED";
    const status = code.includes("ACCESS_DENIED") || code.includes("PERMISSION") ? 403
      : code.includes("NOT_FOUND") ? 404
        : code.startsWith("TASK_TRIGGER_INPUT_") ? 400 : 409;
    return NextResponse.json({ error: code }, { status });
  }
}
