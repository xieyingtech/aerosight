import { NextRequest,NextResponse } from "next/server";
import { recordIssueFeedback } from "@/lib/issue-feedback";

export async function POST(request: NextRequest,{ params }: { params: Promise<{ id: string;issueId: string }> }) {
  try {
    const { id,issueId } = await params;
    return NextResponse.json(await recordIssueFeedback(Number(id),Number(issueId),await request.json(),request.headers.get("x-request-id")));
  } catch (error) {
    const message = error instanceof Error ? error.message : "ISSUE_FEEDBACK_FAILED";
    const status = message === "ISSUE_VERSION_CONFLICT" ? 409 : /PERMISSION|ACCESS/.test(message) ? 403 : /NOT_FOUND/.test(message) ? 404 : 400;
    return NextResponse.json({ error: message },{ status });
  }
}
