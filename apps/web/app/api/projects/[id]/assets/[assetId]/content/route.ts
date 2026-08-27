import { parseMediaAccessAction } from "@/lib/media-access-core";
import { readAuthorizedMediaContent } from "@/lib/media-access";

export async function GET(
  request: Request,
  { params }: { params: Promise<{ id: string; assetId: string }> }
) {
  try {
    const { id, assetId } = await params;
    const url = new URL(request.url);
    const content = await readAuthorizedMediaContent({
      projectId: Number(id), assetId: Number(assetId),
      action: parseMediaAccessAction(url.searchParams.get("action")),
      expires: url.searchParams.get("expires") ?? "",
      signature: url.searchParams.get("signature") ?? ""
    });
    return new Response(content.body, {
      headers: {
        "Content-Type": content.contentType,
        "Content-Disposition": content.disposition,
        "Cache-Control": "private, no-store",
        "X-Content-Type-Options": "nosniff"
      }
    });
  } catch {
    return Response.json({ error: "Media unavailable" }, { status: 403 });
  }
}
