import assert from "node:assert/strict";
import test from "node:test";

import {
  assertProjectObjectKey,
  buildProjectObjectKey,
  MemoryS3CompatibleObjectStorage,
  sha256Hex,
  verifyStoredObject
} from "./object-storage-core.ts";

test("project object keys reject traversal, encoding, and another project prefix", () => {
  const valid = buildProjectObjectKey(17, "upload-1", "inspection frame.jpg");
  assert.equal(valid, "projects/17/uploads/upload-1/inspection-frame.jpg");
  assert.equal(assertProjectObjectKey(17, valid), valid);
  assert.throws(() => assertProjectObjectKey(17, "projects/18/uploads/upload-1/frame.jpg"), /PROJECT_MISMATCH/);
  assert.throws(() => assertProjectObjectKey(17, "projects/17/uploads/../secret"), /INVALID_OBJECT_KEY/);
  assert.throws(() => assertProjectObjectKey(17, "projects/17/uploads/%2e%2e/secret"), /INVALID_OBJECT_KEY/);
});

test("memory S3-compatible storage presigns upload and short-lived download", async () => {
  const now = new Date("2026-08-27T00:00:00.000Z");
  const storage = new MemoryS3CompatibleObjectStorage({ now: () => now });
  const key = buildProjectObjectKey(17, "upload-2", "frame.jpg");
  const checksumSha256 = sha256Hex("frame-content");
  const upload = await storage.createPresignedUpload({
    key,
    contentType: "image/jpeg",
    checksumSha256,
    expiresInSeconds: 300
  });
  assert.equal(storage.verifyPresignedUrl(upload.url, "PUT"), true);
  assert.deepEqual(upload.headers, {
    "content-type": "image/jpeg",
    "x-amz-checksum-sha256": checksumSha256
  });

  await storage.putObject({ key, body: "frame-content", contentType: "image/jpeg" });
  const download = await storage.createPresignedDownload({ key, expiresInSeconds: 60 });
  assert.equal(storage.verifyPresignedUrl(download.url, "GET"), true);
});

test("HEAD verification fails closed on size or checksum mismatch", async () => {
  const storage = new MemoryS3CompatibleObjectStorage();
  const key = buildProjectObjectKey(17, "upload-3", "frame.jpg");
  const stored = await storage.putObject({ key, body: "trusted-content", contentType: "image/jpeg" });

  await assert.rejects(
    verifyStoredObject(storage, { projectId: 17, key, sizeBytes: stored.sizeBytes + 1, checksumSha256: stored.checksumSha256 }),
    /OBJECT_SIZE_MISMATCH/
  );
  await assert.rejects(
    verifyStoredObject(storage, { projectId: 17, key, sizeBytes: stored.sizeBytes, checksumSha256: sha256Hex("forged") }),
    /OBJECT_CHECKSUM_MISMATCH/
  );
  assert.deepEqual(
    await verifyStoredObject(storage, {
      projectId: 17,
      key,
      sizeBytes: stored.sizeBytes,
      checksumSha256: stored.checksumSha256
    }),
    await storage.headObject(key)
  );
});
