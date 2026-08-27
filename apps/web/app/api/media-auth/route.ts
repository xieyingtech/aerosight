import { timingSafeEqual } from "node:crypto";
import { NextResponse } from "next/server";

import { MediaPlaybackTokenIssuer, type BrowserPlaybackProtocol } from "@/lib/live-stream-core";
import { getWebRuntimeConfig } from "@/lib/runtime-config";

function equalSecret(left: string, right: string | undefined) {
  if (!right) return false;
  const actual = Buffer.from(left);
  const expected = Buffer.from(right);
  return actual.length === expected.length && timingSafeEqual(actual, expected);
}

export async function POST(request: Request) {
  const body = await request.json().catch(() => null) as null | {
    user?: string; password?: string; token?: string; action?: string; path?: string;
    protocol?: string; query?: string;
  };
  if (!body?.action || !body.path) return NextResponse.json({ error: "MEDIA_AUTH_DENIED" }, { status: 401 });
  if (body.action === "publish") {
    const allowed = body.path.startsWith("demo/aerosight/")
      && equalSecret(body.user ?? "", process.env.MEDIA_PUBLISH_USER)
      && equalSecret(body.password ?? "", process.env.MEDIA_PUBLISH_PASSWORD);
    return allowed ? new NextResponse(null, { status: 204 })
      : NextResponse.json({ error: "MEDIA_AUTH_DENIED" }, { status: 401 });
  }
  if (body.action === "api") {
    const allowed = equalSecret(body.user ?? "", process.env.MEDIA_ADMIN_USER)
      && equalSecret(body.password ?? "", process.env.MEDIA_ADMIN_PASSWORD);
    return allowed ? new NextResponse(null, { status: 204 })
      : NextResponse.json({ error: "MEDIA_AUTH_DENIED" }, { status: 401 });
  }
  if (body.action !== "read" && body.action !== "playback") {
    return NextResponse.json({ error: "MEDIA_AUTH_DENIED" }, { status: 401 });
  }
  if (body.protocol !== "hls" && body.protocol !== "webrtc") {
    return NextResponse.json({ error: "MEDIA_AUTH_DENIED" }, { status: 401 });
  }
  const queryToken = new URLSearchParams(body.query ?? "").get("token");
  const token = body.token || queryToken;
  const claims = token ? new MediaPlaybackTokenIssuer(getWebRuntimeConfig().authSecret)
    .verify(token, { path: body.path, protocol: body.protocol as BrowserPlaybackProtocol }) : null;
  return claims ? new NextResponse(null, { status: 204 })
    : NextResponse.json({ error: "MEDIA_AUTH_DENIED" }, { status: 401 });
}
