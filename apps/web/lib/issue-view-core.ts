export function issueEvidenceSummary(input: {
  detections: Array<Record<string, unknown>>;
  assets: Array<Record<string, unknown>>;
}) {
  const located = input.detections.filter((item) => item.geometry != null);
  return {
    hasMapLocation: located.length > 0,
    locationLabel: located.length > 0 ? `${located.length} 条检测具有地图位置` : "尚未关联地理位置",
    hasEvidence: input.detections.length > 0 || input.assets.length > 0,
    evidenceLabel: input.detections.length > 0 || input.assets.length > 0
      ? `检测（${input.detections.length} 条），媒体（${input.assets.length} 个）` : "暂无关联证据",
    detectionCount: input.detections.length,
    assetCount: input.assets.length,
    completeEvidence: input.detections.length > 0 && input.assets.length > 0
  };
}
export function issuePriorityLabel(priority: string) {
  return ({ low: "低", medium: "中", high: "高", critical: "紧急" } as Record<string, string>)[priority] ?? priority;
}
