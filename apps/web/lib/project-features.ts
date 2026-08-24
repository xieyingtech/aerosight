import "server-only";

import { query } from "@/lib/db";

export type ProjectFeatureFlags = {
  deviceCommands: boolean;
  operationsOverview: boolean;
  objectStorage: boolean;
  externalAlgorithms: boolean;
  automaticAi: boolean;
  dependencyHealth: Record<string, unknown>;
};

export const DEFAULT_PROJECT_FEATURE_FLAGS: ProjectFeatureFlags = Object.freeze({
  deviceCommands: false,
  operationsOverview: false,
  objectStorage: false,
  externalAlgorithms: false,
  automaticAi: false,
  dependencyHealth: {}
});

export async function getProjectFeatureFlags(projectId: number): Promise<ProjectFeatureFlags> {
  const result = await query<ProjectFeatureFlags>(
    `select coalesce(flags.device_commands_enabled, false) as "deviceCommands",
            coalesce(flags.operations_overview_enabled, false) as "operationsOverview",
            coalesce(flags.object_storage_enabled, false) as "objectStorage",
            coalesce(flags.external_algorithms_enabled, false) as "externalAlgorithms",
            coalesce(flags.automatic_ai_enabled, false) as "automaticAi",
            coalesce(flags.dependency_health_json, '{}'::jsonb) as "dependencyHealth"
       from projects project
       left join project_feature_flags flags on flags.project_id = project.id
      where project.id = $1`,
    [projectId]
  );
  const flags = result.rows[0];
  if (!flags) throw new Error("PROJECT_NOT_FOUND");
  return flags;
}
