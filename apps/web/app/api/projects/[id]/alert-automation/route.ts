import { NextResponse } from "next/server";

import { saveAlertAutomationPolicy, setAutomaticAiEnabled } from "@/lib/alert-automation-settings";

export async function POST(request: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const projectId = Number(id);
  const body = await request.json() as { action?: string; enabled?: boolean; mode?: unknown };
  try {
    const result = body.action === "toggle"
      ? await setAutomaticAiEnabled(projectId, body.enabled === true, request.headers.get("x-request-id"))
      : body.action === "save"
        ? await saveAlertAutomationPolicy(projectId, body, request.headers.get("x-request-id"))
        : null;
    if (!result) return NextResponse.json({ error: "ALERT_AUTOMATION_ACTION_INVALID" }, { status: 400 });
    return NextResponse.json(result);
  } catch (error) {
    const code = error instanceof Error ? error.message : "ALERT_AUTOMATION_FAILED";
    return NextResponse.json({ error: code }, { status: code.includes("REQUIRED") ? 400 : 403 });
  }
}
