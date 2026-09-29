import assert from "node:assert/strict";
import test from "node:test";
import { defaultVideoChannel } from "./device-video-channel.ts";
import type { ProjectSnapshotChannel } from "./project-snapshot-core.ts";

const channel = (channelKey: string, availability = "available"): ProjectSnapshotChannel => ({
  channelKey, availability, stableChannelId: channelKey, displayName: channelKey,
  dataType: "video", availabilityReason: null, protocol: null,
});

test("payload camera takes priority over Vision Assist regardless of lexical order", () => {
  assert.equal(defaultVideoChannel([channel("176-0-0"), channel("81-0-0")])?.channelKey, "81-0-0");
  assert.equal(defaultVideoChannel([channel("176-0-0")])?.channelKey, "176-0-0");
  assert.equal(defaultVideoChannel([channel("165-0-7")])?.channelKey, "165-0-7");
  assert.equal(defaultVideoChannel([channel("81-0-0", "unavailable"), channel("176-0-0")])?.channelKey, "176-0-0");
});
