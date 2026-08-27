export type GeoPoint = [longitude: number, latitude: number, altitudeMeters?: number];

export type ComplianceValue = { reference: string; validUntil?: Date };
export type PreflightSeverity = "pass" | "warning" | "hard_failure";

export type SafetyPolicy = {
  policyVersionId: string;
  projectBoundary: GeoPoint[];
  restrictedAreas: GeoPoint[][];
  maxAltitudeMeters: number;
  maxSpeedMetersPerSecond: number;
  minimumBatteryPercent: number;
  allowedWindows?: Array<{ weekdays: number[]; startMinute: number; endMinute: number }>;
  requiredCompliance: string[];
  optionalCompliance?: string[];
  exemptions?: Array<{ field: string; reason: string; validUntil?: Date }>;
};

export type MissionPreflightInput = {
  route: GeoPoint[];
  plannedSpeedMetersPerSecond: number;
  batteryPercent: number;
  plannedStartAt: Date;
  compliance: Record<string, ComplianceValue | undefined>;
};

export type PreflightCheck = {
  code: string;
  severity: PreflightSeverity;
  message: string;
  evidence?: Record<string, unknown>;
};

export type MissionPreflightResult = {
  policyVersionId: string;
  allowed: boolean;
  checks: PreflightCheck[];
};

const orientation = (a: GeoPoint, b: GeoPoint, c: GeoPoint) =>
  Math.sign((b[0] - a[0]) * (c[1] - a[1]) - (b[1] - a[1]) * (c[0] - a[0]));

function onSegment(point: GeoPoint, a: GeoPoint, b: GeoPoint) {
  return orientation(a, b, point) === 0
    && point[0] >= Math.min(a[0], b[0]) && point[0] <= Math.max(a[0], b[0])
    && point[1] >= Math.min(a[1], b[1]) && point[1] <= Math.max(a[1], b[1]);
}

function segmentsIntersect(a: GeoPoint, b: GeoPoint, c: GeoPoint, d: GeoPoint) {
  const abC = orientation(a, b, c);
  const abD = orientation(a, b, d);
  const cdA = orientation(c, d, a);
  const cdB = orientation(c, d, b);
  return (abC !== abD && cdA !== cdB)
    || (abC === 0 && onSegment(c, a, b)) || (abD === 0 && onSegment(d, a, b))
    || (cdA === 0 && onSegment(a, c, d)) || (cdB === 0 && onSegment(b, c, d));
}

export function pointInPolygon(point: GeoPoint, polygon: GeoPoint[]) {
  if (polygon.some((vertex, index) => onSegment(point, vertex, polygon[(index + 1) % polygon.length]))) return true;
  let inside = false;
  for (let index = 0, previous = polygon.length - 1; index < polygon.length; previous = index++) {
    const a = polygon[index];
    const b = polygon[previous];
    if ((a[1] > point[1]) !== (b[1] > point[1])
      && point[0] < ((b[0] - a[0]) * (point[1] - a[1])) / (b[1] - a[1]) + a[0]) inside = !inside;
  }
  return inside;
}

function routeIntersectsPolygon(route: GeoPoint[], polygon: GeoPoint[]) {
  if (route.some((point) => pointInPolygon(point, polygon))) return true;
  for (let routeIndex = 1; routeIndex < route.length; routeIndex += 1) {
    for (let edgeIndex = 0; edgeIndex < polygon.length; edgeIndex += 1) {
      if (segmentsIntersect(route[routeIndex - 1], route[routeIndex], polygon[edgeIndex], polygon[(edgeIndex + 1) % polygon.length])) return true;
    }
  }
  return false;
}

function check(code: string, severity: PreflightSeverity, message: string, evidence?: Record<string, unknown>): PreflightCheck {
  return { code, severity, message, ...(evidence ? { evidence } : {}) };
}

export function evaluateMissionPreflight(policy: SafetyPolicy, input: MissionPreflightInput): MissionPreflightResult {
  const checks: PreflightCheck[] = [];
  const outside = input.route.findIndex((point) => !pointInPolygon(point, policy.projectBoundary));
  checks.push(outside >= 0
    ? check("PROJECT_BOUNDARY", "hard_failure", "航线超出项目边界", { waypointIndex: outside })
    : check("PROJECT_BOUNDARY", "pass", "航线位于项目边界内"));

  const restrictedIndex = policy.restrictedAreas.findIndex((area) => routeIntersectsPolygon(input.route, area));
  checks.push(restrictedIndex >= 0
    ? check("RESTRICTED_AREA", "hard_failure", "航线进入禁限区域", { restrictedAreaIndex: restrictedIndex })
    : check("RESTRICTED_AREA", "pass", "航线未进入禁限区域"));

  const altitude = Math.max(...input.route.map((point) => point[2] ?? 0));
  checks.push(altitude > policy.maxAltitudeMeters
    ? check("MAX_ALTITUDE", "hard_failure", "计划高度超过策略上限", { actual: altitude, limit: policy.maxAltitudeMeters })
    : check("MAX_ALTITUDE", "pass", "计划高度符合策略"));
  checks.push(input.plannedSpeedMetersPerSecond > policy.maxSpeedMetersPerSecond
    ? check("MAX_SPEED", "hard_failure", "计划速度超过策略上限", { actual: input.plannedSpeedMetersPerSecond, limit: policy.maxSpeedMetersPerSecond })
    : check("MAX_SPEED", "pass", "计划速度符合策略"));

  const batterySeverity: PreflightSeverity = input.batteryPercent < policy.minimumBatteryPercent
    ? "hard_failure" : input.batteryPercent < policy.minimumBatteryPercent + 10 ? "warning" : "pass";
  checks.push(check("BATTERY", batterySeverity,
    batterySeverity === "hard_failure" ? "电量低于起飞下限" : batterySeverity === "warning" ? "电量接近起飞下限" : "电量充足",
    { actual: input.batteryPercent, minimum: policy.minimumBatteryPercent }));

  if (policy.allowedWindows?.length) {
    const weekday = input.plannedStartAt.getUTCDay();
    const minute = input.plannedStartAt.getUTCHours() * 60 + input.plannedStartAt.getUTCMinutes();
    const inWindow = policy.allowedWindows.some((window) => window.weekdays.includes(weekday)
      && minute >= window.startMinute && minute <= window.endMinute);
    checks.push(check("TIME_WINDOW", inWindow ? "pass" : "hard_failure", inWindow ? "计划时间符合策略" : "计划时间不在允许窗口"));
  }

  for (const field of policy.requiredCompliance) {
    const value = input.compliance[field];
    const exemption = policy.exemptions?.find((candidate) => candidate.field === field
      && (!candidate.validUntil || candidate.validUntil >= input.plannedStartAt));
    if (exemption) {
      checks.push(check(`COMPLIANCE_${field}`, "warning", "已应用版本化合规豁免", { reason: exemption.reason }));
    } else if (!value?.reference || (value.validUntil && value.validUntil < input.plannedStartAt)) {
      checks.push(check(`COMPLIANCE_${field}`, "hard_failure", "缺少有效的必需合规字段"));
    } else {
      checks.push(check(`COMPLIANCE_${field}`, "pass", "合规字段有效"));
    }
  }
  for (const field of policy.optionalCompliance ?? []) {
    if (!input.compliance[field]?.reference) checks.push(check(`COMPLIANCE_${field}`, "warning", "建议补充合规字段"));
  }

  return { policyVersionId: policy.policyVersionId, allowed: !checks.some((item) => item.severity === "hard_failure"), checks };
}
