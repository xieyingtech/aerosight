import assert from "node:assert/strict";
import test from "node:test";

import { assertMediaActionAllowed, MediaAccessSigner, safeDownloadName } from "./media-access-core.ts";

test("project member may preview but sensitive download needs handling permission", () => {
  assert.doesNotThrow(() => assertMediaActionAllowed({
    action: "preview", role: "member", permissions: new Set(), sensitive: true
  }));
  assert.throws(() => assertMediaActionAllowed({
    action: "download", role: "member", permissions: new Set(), sensitive: true
  }), /SENSITIVE_MEDIA_DOWNLOAD_DENIED/);
  assert.doesNotThrow(() => assertMediaActionAllowed({
    action: "download", role: "member", permissions: new Set(["event:handle"]), sensitive: true
  }));
});

test("media URL expires and cannot cross project or change action", () => {
  let now = new Date("2026-08-27T00:00:00Z");
  const signer = new MediaAccessSigner("0123456789abcdef", () => now);
  const access = signer.issue({ projectId: 17, assetId: 42, action: "preview", ttlSeconds: 30 });
  assert(!access.url.includes("storage") && !access.url.includes("secret"));
  const parsed = new URL(access.url, "https://aerosight.test");
  const token = { expires: parsed.searchParams.get("expires")!, signature: parsed.searchParams.get("signature")! };
  assert.equal(signer.verify({ projectId: 17, assetId: 42, action: "preview", ...token }), true);
  assert.equal(signer.verify({ projectId: 18, assetId: 42, action: "preview", ...token }), false);
  assert.equal(signer.verify({ projectId: 17, assetId: 42, action: "download", ...token }), false);
  now = new Date("2026-08-27T00:01:00Z");
  assert.equal(signer.verify({ projectId: 17, assetId: 42, action: "preview", ...token }), false);
});

test("download names cannot inject response headers", () => {
  assert.equal(safeDownloadName("frame\r\nattachment.exe", "asset-42"), "frame-attachment.exe");
});
