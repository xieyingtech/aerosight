import { timingSafeEqual } from "node:crypto";
import { NextResponse } from "next/server";

import { credentialAAD, decryptCredentialObject, type CredentialEnvelope } from "@/lib/credential-encryption";
import { query } from "@/lib/db";
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
  if (!body?.action) return NextResponse.json({ error: "MEDIA_AUTH_DENIED" }, { status: 401 });
  if (body.action === "api") {
    const allowed = equalSecret(body.user ?? "", process.env.MEDIA_ADMIN_USER)
      && equalSecret(body.password ?? "", process.env.MEDIA_ADMIN_PASSWORD);
    return allowed ? new NextResponse(null, { status: 204 })
      : NextResponse.json({ error: "MEDIA_AUTH_DENIED" }, { status: 401 });
  }
  if (!body.path) return NextResponse.json({ error: "MEDIA_AUTH_DENIED" }, { status: 401 });
  if (body.action === "publish") {
    let row: { projectId: number; adapterId: string; envelope: CredentialEnvelope } | null = null;
    try {
      row = body.path.startsWith("demo/aerosight/") ? (await query<{
        projectId: number; adapterId: string; envelope: CredentialEnvelope;
      }>(
        `select stream.project_id as "projectId", adapter.id as "adapterId",
                adapter.credential_envelope_json as envelope
           from live_streams stream
           join device_adapters adapter on adapter.id=stream.adapter_id and adapter.project_id=stream.project_id
          where stream.ingest_ref=$1 and stream.status in ('requested','starting','live','degraded')
          limit 1`, [body.path]
      )).rows[0] ?? null : null;
    } catch { row = null; }
    let allowed = false;
    if (row?.envelope) {
      try {
        const credentials = decryptCredentialObject<{ mediaPublishUser: string; mediaPublishPassword: string }>(
          row.envelope, getWebRuntimeConfig().authSecret,
          credentialAAD("device-adapter", row.adapterId, row.projectId)
        );
        allowed = equalSecret(body.user ?? "", credentials.mediaPublishUser)
          && equalSecret(body.password ?? "", credentials.mediaPublishPassword);
      } catch { allowed = false; }
    }
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
