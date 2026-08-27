export type PerceptionEventEvidenceInput = {
  event: Record<string, unknown>;
  detections: Array<Record<string, unknown>>;
  feedback: Array<Record<string, unknown>>;
};

export function buildPerceptionEventEvidence(input: PerceptionEventEvidenceInput) {
  const detections = input.detections.map((detection) => ({
    id: detection.id,
    label: detection.label,
    confidence: detection.confidence,
    locationQuality: detection.locationQuality ?? "unavailable",
    geographicGeometry: detection.geographicGeometry ?? null,
    horizontalErrorMeters: detection.horizontalErrorMeters ?? null,
    projectionMethod: detection.projectionMethod ?? "image-only",
    pixelGeometry: detection.pixelGeometry,
    modelOrProcess: detection.modelOrProcess,
    modelVersion: detection.modelVersion,
    mappingVersion: detection.mappingVersion,
    inputAssetId: detection.inputAssetId,
    assetVersion: detection.assetVersion,
    assetChecksumSha256: detection.assetChecksumSha256,
    mimeType: detection.mimeType,
    capturedAt: detection.capturedAt
  }));
  const mapped = detections.filter((item) => item.geographicGeometry !== null);
  const event: Record<string, unknown> & { title: string; disclaimer: string; hasMapLocation: boolean; locationSummary: string } = {
    ...input.event,
    title: "疑似违建",
    disclaimer: "该结果为算法生成的巡检线索，不构成法律意义上的违建认定。",
    hasMapLocation: mapped.length > 0,
    locationSummary: mapped.length > 0 ? `${mapped.length} 条检测具有可用地理位置` : "位置不可用，仅展示影像内标注"
  };
  return {
    event,
    detections,
    feedback: input.feedback
  };
}
