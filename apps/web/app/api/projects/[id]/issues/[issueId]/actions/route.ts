import { NextRequest, NextResponse } from "next/server";
import { issueMutationInputSchema, mutateIssue } from "@/lib/issue-collaboration";

function statusFor(error: unknown) {
  const message = error instanceof Error ? error.message : "ISSUE_UPDATE_FAILED";
  if (message === "PROJECT_ACCESS_DENIED") return 403;
  if (message === "ISSUE_NOT_FOUND") return 404;
  if (message === "ISSUE_VERSION_CONFLICT") return 409;
  return 400;
}

export async function POST(request: NextRequest, { params }: { params: Promise<{ id: string; issueId: string }> }) {
  try {
    const { id, issueId } = await params;
    const parsed = issueMutationInputSchema.parse(await request.json());
    const result = await mutateIssue(Number(id), Number(issueId), parsed, request.headers.get("x-request-id"));
    return NextResponse.json(result);
  } catch (error) {
    return NextResponse.json({ error: error instanceof Error ? error.message : "ISSUE_UPDATE_FAILED" }, { status: statusFor(error) });
  }
}
