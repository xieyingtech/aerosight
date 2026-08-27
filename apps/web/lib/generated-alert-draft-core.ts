import { createHash } from "node:crypto";
import type { z } from "zod";
import { agentEvidenceReferenceSchema } from "./agent-draft-tools-core.ts";

export type EvidenceReference = z.infer<typeof agentEvidenceReferenceSchema>;

export function evidenceVersionHash(references: readonly EvidenceReference[]) {
  const canonical=[...references].sort((left,right)=>`${left.type}:${left.id}`.localeCompare(`${right.type}:${right.id}`))
    .map((reference)=>`${reference.type}:${reference.id}:${reference.version}`).join("\n");
  return createHash("sha256").update(canonical).digest("hex");
}

export function assessGeneratedDraftFreshness(saved:readonly EvidenceReference[],current:readonly EvidenceReference[]){
  const currentVersions=new Map(current.map((reference)=>[`${reference.type}:${reference.id}`,reference.version]));
  const revisions=saved.flatMap((reference)=>{const currentVersion=currentVersions.get(`${reference.type}:${reference.id}`);return currentVersion===reference.version?[]:[{type:reference.type,id:reference.id,savedVersion:reference.version,currentVersion:currentVersion??null}]});
  return {stale:revisions.length>0,revisions};
}
