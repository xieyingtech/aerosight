export function issueEvidenceSummary(input: {
  detections: Array<Record<string, unknown>>;
  assets: Array<Record<string, unknown>>;
}) {
  const located = input.detections.filter((item) => item.geometry != null);
  return {
    hasMapLocation: located.length > 0,
    locationLabel: located.length > 0 ? `${located.length} 条检测具有地图位置` : "仅影像级证据，暂无可靠地图位置",
    detectionCount: input.detections.length,
    assetCount: input.assets.length,
    completeEvidence: input.detections.length > 0 && input.assets.length > 0
  };
}
