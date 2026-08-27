import assert from "node:assert/strict";
import test from "node:test";

import {
  completeMediaUpload,
  MemoryMediaIngestionRepository,
  startMediaUpload
} from "./media-ingestion-core.ts";
import { MemoryS3CompatibleObjectStorage, sha256Hex } from "./object-storage-core.ts";

const completionTime = new Date("2026-08-27T00:02:00.000Z");

async function startFixture(
  repository: MemoryMediaIngestionRepository,
  storage: MemoryS3CompatibleObjectStorage,
  body: string,
  logicalKey = "missions/42/frame"
) {
  return startMediaUpload(repository, storage, {
    projectId: 17,
    teamId: 5,
    actorUserId: 9,
    logicalKey,
    fileName: "frame.jpg",
    kind: "image",
    mimeType: "image/jpeg",
    sizeBytes: Buffer.byteLength(body),
    checksumSha256: sha256Hex(body),
    now: new Date("2026-08-27T00:00:00.000Z")
  });
}

test("interrupted upload stays unpublished and can complete on retry", async () => {
  const repository = new MemoryMediaIngestionRepository();
  const storage = new MemoryS3CompatibleObjectStorage();
  const started = await startFixture(repository, storage, "frame-content");

  await assert.rejects(
    completeMediaUpload(repository, storage, {
      projectId: 17,
      uploadId: started.intent.id,
      now: new Date("2026-08-27T00:01:00.000Z")
    }),
    /OBJECT_NOT_FOUND/
  );
  assert.equal(repository.assets.size, 0);
  assert.equal((await repository.getUploadIntent(17, started.intent.id))?.status, "pending");

  await storage.putObject({ key: started.intent.objectKey, body: "frame-content", contentType: "image/jpeg" });
  const asset = await completeMediaUpload(repository, storage, {
    projectId: 17,
    uploadId: started.intent.id,
    now: new Date("2026-08-27T00:02:00.000Z")
  });
  assert.equal(asset.status, "available");
});

test("repeated completion publishes one asset", async () => {
  const repository = new MemoryMediaIngestionRepository();
  const storage = new MemoryS3CompatibleObjectStorage();
  const started = await startFixture(repository, storage, "frame-content");
  await storage.putObject({ key: started.intent.objectKey, body: "frame-content", contentType: "image/jpeg" });

  const first = await completeMediaUpload(repository, storage, {
    projectId: 17, uploadId: started.intent.id, now: completionTime
  });
  const replay = await completeMediaUpload(repository, storage, {
    projectId: 17, uploadId: started.intent.id, now: completionTime
  });
  assert.equal(replay.id, first.id);
  assert.equal(repository.assets.size, 1);
});

test("checksum failure marks the intent failed and never publishes the asset", async () => {
  const repository = new MemoryMediaIngestionRepository();
  const storage = new MemoryS3CompatibleObjectStorage();
  const started = await startFixture(repository, storage, "expected");
  await storage.putObject({ key: started.intent.objectKey, body: "tampered", contentType: "image/jpeg" });

  await assert.rejects(
    completeMediaUpload(repository, storage, { projectId: 17, uploadId: started.intent.id, now: completionTime }),
    /OBJECT_CHECKSUM_MISMATCH/
  );
  assert.equal(repository.assets.size, 0);
  assert.equal((await repository.getUploadIntent(17, started.intent.id))?.status, "failed");
});

test("new logical asset version cannot rewrite a published evidence snapshot", async () => {
  const repository = new MemoryMediaIngestionRepository();
  const storage = new MemoryS3CompatibleObjectStorage();
  const firstUpload = await startFixture(repository, storage, "version-one");
  await storage.putObject({ key: firstUpload.intent.objectKey, body: "version-one", contentType: "image/jpeg" });
  const first = await completeMediaUpload(repository, storage, {
    projectId: 17, uploadId: firstUpload.intent.id, now: completionTime
  });
  const evidence = repository.linkEvidence({
    projectId: 17,
    targetType: "report",
    targetId: "report-9",
    assetId: first.id,
    published: true
  });

  const secondUpload = await startFixture(repository, storage, "version-two");
  await storage.putObject({ key: secondUpload.intent.objectKey, body: "version-two", contentType: "image/jpeg" });
  const second = await completeMediaUpload(repository, storage, {
    projectId: 17, uploadId: secondUpload.intent.id, now: completionTime
  });

  assert.equal(second.version, 2);
  assert.equal(second.supersedesAssetId, first.id);
  assert.equal(evidence.assetId, first.id);
  assert.equal(evidence.assetVersion, 1);
  assert.equal(evidence.assetChecksumSha256, sha256Hex("version-one"));
});

test("an upload intent is invisible from another project scope", async () => {
  const repository = new MemoryMediaIngestionRepository();
  const storage = new MemoryS3CompatibleObjectStorage();
  const started = await startFixture(repository, storage, "frame-content");
  assert.equal(await repository.getUploadIntent(18, started.intent.id), null);
  await assert.rejects(
    completeMediaUpload(repository, storage, { projectId: 18, uploadId: started.intent.id }),
    /UPLOAD_INTENT_NOT_FOUND/
  );
});
