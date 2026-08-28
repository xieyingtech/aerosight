export type AlgorithmCatalogRow = {
  definitionId: string;
  configurationSnapshotId: string;
  name: string;
  description: string | null;
  capabilityCode: string;
  providerType: string;
  providerStatus: string;
  executionMode: string;
  modelOrProcess: string;
  inputSchema: Record<string, unknown>;
  parametersSchema: Record<string, unknown>;
  outputSchema: Record<string, unknown>;
  displayMetadata: Record<string, unknown>;
};

export function buildAlgorithmCatalogEntry(row: AlgorithmCatalogRow) {
  return {
    id: row.definitionId,
    configurationSnapshotId: row.configurationSnapshotId,
    name: row.name,
    description: row.description,
    capabilityCode: row.capabilityCode,
    execution: { mode: row.executionMode, modelOrProcess: row.modelOrProcess },
    provider: { type: row.providerType, available: row.providerStatus === "active" },
    schemas: {
      input: row.inputSchema,
      parameters: row.parametersSchema,
      output: row.outputSchema
    },
    display: row.displayMetadata
  };
}
