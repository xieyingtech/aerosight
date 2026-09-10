import { NextResponse } from "next/server";
import { readFlightHubLiveMedia } from "@/lib/dji-flighthub-live-media";

export const dynamic = "force-dynamic";
export async function GET(_request: Request,{ params }: { params: Promise<{ id: string }> }) {
  try { const { id } = await params; return NextResponse.json(await readFlightHubLiveMedia(Number(id)),
    { headers: { "Cache-Control":"private, no-store, max-age=0" } }); }
  catch { return NextResponse.json({ error:{ code:"access_denied" } },{ status:403,
    headers:{ "Cache-Control":"private, no-store, max-age=0" } }); }
}
