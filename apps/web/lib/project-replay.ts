import "server-only";
import { db } from "@/lib/db";
import { readProjectReplay as readCore, type ReplayQuery } from "@/lib/project-replay-core";

export function readProjectReplay(userId: number, projectId: number, input: ReplayQuery) {
  return readCore(userId, projectId, input, () => db.connect());
}
