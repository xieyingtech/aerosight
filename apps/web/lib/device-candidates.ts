import type { GeoPoint } from "./mission-preflight.ts";

export type CandidateDevice = {
  id: number;
  type: string;
  connectionStatus: "online" | "degraded" | "offline" | "unknown";
  batteryPercent?: number;
  position?: GeoPoint;
  occupiedByTaskRunId?: number;
  capabilities: Record<string, Record<string, unknown>>;
};

export type DeviceRequirement = {
  deviceType: string;
  minimumBatteryPercent: number;
  routeStart: GeoPoint;
  capabilities: Array<{ code: string; constraints?: Record<string, unknown> }>;
};

export type RankedDevice = {
  deviceId: number;
  eligible: boolean;
  score: number;
  distanceMeters?: number;
  features: Record<string, number | string | boolean>;
  exclusionReasons: string[];
};

export type DeviceSelection = {
  status: "selected" | "blocked";
  selectedDeviceId?: number;
  overridden: boolean;
  requiresPreflight: boolean;
  candidates: RankedDevice[];
};

function distanceMeters(a: GeoPoint, b: GeoPoint) {
  const radians = (value: number) => value * Math.PI / 180;
  const latitudeDelta = radians(b[1] - a[1]);
  const longitudeDelta = radians(b[0] - a[0]);
  const value = Math.sin(latitudeDelta / 2) ** 2
    + Math.cos(radians(a[1])) * Math.cos(radians(b[1])) * Math.sin(longitudeDelta / 2) ** 2;
  return 6_371_000 * 2 * Math.atan2(Math.sqrt(value), Math.sqrt(1 - value));
}

function constraintSatisfied(actual: unknown, expected: unknown): boolean {
  if (typeof expected === "number") return typeof actual === "number" && actual >= expected;
  if (Array.isArray(expected)) return Array.isArray(actual) && expected.every((item) => actual.includes(item));
  return actual === expected;
}

function capabilityMismatch(device: CandidateDevice, requirement: DeviceRequirement) {
  for (const required of requirement.capabilities) {
    const actual = device.capabilities[required.code];
    if (!actual) return `CAPABILITY_MISSING:${required.code}`;
    for (const [key, expected] of Object.entries(required.constraints ?? {})) {
      if (!constraintSatisfied(actual[key], expected)) return `CAPABILITY_CONSTRAINT:${required.code}.${key}`;
    }
  }
  return null;
}

export function rankDeviceCandidates(devices: CandidateDevice[], requirement: DeviceRequirement): RankedDevice[] {
  return devices.map((device) => {
    const exclusionReasons: string[] = [];
    if (device.type !== requirement.deviceType) exclusionReasons.push("DEVICE_TYPE_MISMATCH");
    if (device.connectionStatus === "offline" || device.connectionStatus === "unknown") exclusionReasons.push("DEVICE_NOT_CONNECTED");
    if (device.batteryPercent === undefined) exclusionReasons.push("BATTERY_UNKNOWN");
    else if (device.batteryPercent < requirement.minimumBatteryPercent) exclusionReasons.push("BATTERY_BELOW_MINIMUM");
    if (device.occupiedByTaskRunId !== undefined) exclusionReasons.push("DEVICE_OCCUPIED");
    const mismatch = capabilityMismatch(device, requirement);
    if (mismatch) exclusionReasons.push(mismatch);

    const distance = device.position ? distanceMeters(device.position, requirement.routeStart) : undefined;
    if (distance === undefined) exclusionReasons.push("POSITION_UNKNOWN");
    const connectionScore = device.connectionStatus === "online" ? 30 : device.connectionStatus === "degraded" ? 10 : 0;
    const batteryScore = Math.max(0, Math.min(40, (device.batteryPercent ?? 0) * 0.4));
    const proximityScore = distance === undefined ? 0 : Math.max(0, 30 - distance / 1000);
    const eligible = exclusionReasons.length === 0;
    return {
      deviceId: device.id,
      eligible,
      score: eligible ? Math.round((connectionScore + batteryScore + proximityScore) * 100) / 100 : 0,
      ...(distance === undefined ? {} : { distanceMeters: Math.round(distance) }),
      features: {
        connectionStatus: device.connectionStatus,
        batteryPercent: device.batteryPercent ?? "unknown",
        occupied: device.occupiedByTaskRunId !== undefined,
        distanceMeters: distance === undefined ? "unknown" : Math.round(distance),
        capabilityCount: Object.keys(device.capabilities).length
      },
      exclusionReasons
    };
  }).sort((left, right) => Number(right.eligible) - Number(left.eligible) || right.score - left.score || left.deviceId - right.deviceId);
}

export function selectMissionDevice(
  devices: CandidateDevice[],
  requirement: DeviceRequirement,
  overrideDeviceId?: number
): DeviceSelection {
  const candidates = rankDeviceCandidates(devices, requirement);
  if (overrideDeviceId !== undefined) {
    const overridden = candidates.find((candidate) => candidate.deviceId === overrideDeviceId);
    if (!overridden?.eligible) return { status: "blocked", overridden: true, requiresPreflight: true, candidates };
    return {
      status: "selected", selectedDeviceId: overrideDeviceId, overridden: true,
      requiresPreflight: true, candidates
    };
  }
  const selected = candidates.find((candidate) => candidate.eligible);
  return selected
    ? { status: "selected", selectedDeviceId: selected.deviceId, overridden: false, requiresPreflight: true, candidates }
    : { status: "blocked", overridden: false, requiresPreflight: true, candidates };
}
