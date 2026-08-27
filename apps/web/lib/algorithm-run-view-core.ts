export type AlgorithmRunViewRow = {
  id: string;
  status: string;
  inputSnapshot: Record<string, unknown>;
  canonicalResult: Record<string, unknown>;
  rawResultObjectKey: string | null;
  rawResultChecksumSha256: string | null;
  createdAt: Date;
  startedAt: Date | null;
  finishedAt: Date | null;
  errorCode: string | null;
  errorMessage: string | null;
};

export function buildAlgorithmRunDiagnostics(row: AlgorithmRunViewRow, permissions: ReadonlySet<string>) {
  const snapshot = row.inputSnapshot;
  const asset = objectValue(snapshot.inputAsset);
  const definition = objectValue(snapshot.definition);
  const diagnostics = Array.isArray(row.canonicalResult.mappingDiagnostics)
    ? row.canonicalResult.mappingDiagnostics.filter((value): value is string => typeof value === "string")
    : [];
  const end = row.finishedAt ?? new Date();
  const durationMs = row.startedAt ? Math.max(0, end.getTime() - row.startedAt.getTime()) : null;
  return {
    input: {
      assetId: numberValue(asset.assetId), assetVersion: numberValue(asset.version),
      checksumSha256: stringValue(asset.checksumSha256), mimeType: stringValue(asset.mimeType),
      parameters: objectValue(snapshot.parameters), context: objectValue(snapshot.context)
    },
    version: {
      definitionVersionId: numberValue(definition.definitionVersionId),
      modelOrProcess: stringValue(definition.modelOrProcess),
      mappingVersion: stringValue(definition.mappingVersion), providerType: stringValue(definition.providerType)
    },
    durationMs,
    diagnostics,
    rawResult: row.rawResultObjectKey ? { objectKey: row.rawResultObjectKey, checksumSha256: row.rawResultChecksumSha256 } : null,
    retryAllowed: ["failed", "timed_out"].includes(row.status) && permissions.has("algorithm:manage")
  };
}

function objectValue(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function stringValue(value: unknown) {
  return typeof value === "string" ? value : null;
}

function numberValue(value: unknown) {
  return typeof value === "number" ? value : null;
}
