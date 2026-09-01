export type DevicePositionFacts = {
  dataFreshness?: unknown;
  positionStatus?: unknown;
  positionReason?: unknown;
  positionSource?: unknown;
  pose?: { longitude?: unknown; latitude?: unknown; capturedAt?: unknown; calibrationStatus?: unknown } | null;
};

export type DevicePositionPresentation = {
  state: "available" | "unverified" | "invalid" | "missing" | "stale";
  label: string;
  reason: string;
  source: string;
  capturedAt: string | null;
  coordinate: string | null;
};

function finiteCoordinate(value: unknown) {
  if (value === null || value === undefined || value === "" || typeof value === "boolean") return null;
  const number = Number(value);
  return Number.isFinite(number) ? number : null;
}

export function presentDevicePosition(device: DevicePositionFacts): DevicePositionPresentation {
  const longitude = finiteCoordinate(device.pose?.longitude);
  const latitude = finiteCoordinate(device.pose?.latitude);
  const hasPosition = longitude !== null && latitude !== null;
  const freshness = String(device.dataFreshness ?? "unknown");
  const rawStatus = String(device.positionStatus ?? (hasPosition ? "available" : "missing"));
  const rawReason = String(device.positionReason ?? "");
  const source = String(device.positionSource ?? "unknown");
  const capturedAt = device.pose?.capturedAt ? String(device.pose.capturedAt) : null;
  const coordinate = hasPosition ? `${longitude.toFixed(6)}, ${latitude.toFixed(6)}` : null;
  if (!hasPosition) {
    if (rawStatus === "invalid") return { state: "invalid", label: "位置无效", reason: rawReason || "上游坐标无效", source, capturedAt, coordinate: null };
    return { state: "missing", label: "暂无位置", reason: rawReason || "尚未收到有效坐标", source, capturedAt, coordinate: null };
  }
  if (freshness === "stale" || freshness === "expired") {
    return { state: "stale", label: "位置已过期", reason: rawReason || `数据新鲜度：${freshness}`, source, capturedAt, coordinate };
  }
  if (rawStatus === "invalid") {
    return { state: "invalid", label: "当前位置无效", reason: rawReason || "显示上次有效位置", source, capturedAt, coordinate };
  }
  if (rawStatus === "unverified" || device.pose?.calibrationStatus === "unverified") {
    return { state: "unverified", label: "位置未校准", reason: rawReason || "坐标基准待现场验收，不用于控制预检", source, capturedAt, coordinate };
  }
  return { state: "available", label: "位置可用", reason: rawReason || "坐标已标准化", source, capturedAt, coordinate };
}
