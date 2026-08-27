import assert from "node:assert/strict";
import test from "node:test";
import { assessGeneratedDraftFreshness, evidenceVersionHash, type EvidenceReference } from "./generated-alert-draft-core.ts";

const saved:EvidenceReference[]=[{type:"event",id:"evt-1",version:"state:2",observedAt:"2026-08-27T00:00:00.000Z",quality:"verified"},{type:"asset",id:"4",version:"sha256:abc",observedAt:"2026-08-27T00:00:00.000Z",quality:"verified"}];

test("generated draft provenance hashes model evidence versions deterministically",()=>{
  assert.equal(evidenceVersionHash(saved),evidenceVersionHash([...saved].reverse()));
});

test("evidence revision marks an old generated draft stale",()=>{
  const current=structuredClone(saved);current[0].version="state:3";
  const freshness=assessGeneratedDraftFreshness(saved,current);
  assert.equal(freshness.stale,true);assert.deepEqual(freshness.revisions,[{type:"event",id:"evt-1",savedVersion:"state:2",currentVersion:"state:3"}]);
  assert.equal(assessGeneratedDraftFreshness(saved,saved).stale,false);
});
