import { NextResponse } from "next/server";

import { issueMediaAccess } from "@/lib/media-access";
import { parseMediaAccessAction } from "@/lib/media-access-core";

export async function GET(
  request: Request,
  { params }: { params: Promise<{ id: string; assetId: string }> }
) {
  try {
    const { id, assetId } = await params;
    const action = parseMediaAccessAction(new URL(request.url).searchParams.get("action"));
    return NextResponse.json(await issueMediaAccess(
      Number(id), Number(assetId), action, request.headers.get("x-request-id")
    ));
  } catch {
    return NextResponse.json({ error: "Unable to access media" }, { status: 403 });
  }
}
