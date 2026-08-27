import "server-only";

import { buildAlgorithmCatalogEntry, type AlgorithmCatalogRow } from "@/lib/algorithm-catalog-core";
import { requireCurrentProjectPermission } from "@/lib/data";
import { query } from "@/lib/db";

export async function listAlgorithmCatalog(projectId: number) {
  await requireCurrentProjectPermission(projectId, "project:view");
  const result = await query<AlgorithmCatalogRow>(
    `select definition.id as "definitionId", version.id as "versionId", version.version,
            definition.name, definition.description, definition.capability_code as "capabilityCode",
            provider.provider_type as "providerType", provider.status as "providerStatus",
            version.execution_mode as "executionMode", version.model_or_process as "modelOrProcess",
            version.input_requirements_json as "inputSchema",
            version.parameters_schema_json as "parametersSchema",
            version.output_schema_json as "outputSchema",
            version.display_metadata_json as "displayMetadata"
       from algorithm_definitions definition
       join algorithm_definition_versions version
         on version.id=definition.current_published_version_id and version.project_id=definition.project_id
       join algorithm_providers provider
         on provider.id=definition.provider_id and provider.project_id=definition.project_id
      where definition.project_id=$1 and version.status='published'
      order by definition.name`,
    [projectId]
  );
  return result.rows.map(buildAlgorithmCatalogEntry);
}
