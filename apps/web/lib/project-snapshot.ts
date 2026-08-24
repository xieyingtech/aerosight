import "server-only";

import { db } from "@/lib/db";
import {
  readProjectSituationSnapshot as readSnapshotCore,
  type ProjectSituationSnapshot
} from "@/lib/project-snapshot-core";

export type { ProjectSituationSnapshot };

export function readProjectSituationSnapshot(userId: number, projectId: number) {
  return readSnapshotCore(userId, projectId, () => db.connect());
}
