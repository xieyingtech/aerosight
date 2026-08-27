export type DriverStatus = "active" | "disabled" | "retired";
export type DeviceTypeStatus = "active" | "retired";
export type DeviceStatus = "online" | "degraded" | "offline" | "unknown" | "unavailable";
export type CapabilityAvailability = "available" | "degraded" | "unavailable";
export type CapabilityRisk = "low" | "medium" | "high" | "critical";
export type DeviceStreamDataType = "video" | "audio" | "telemetry" | "sensor" | "events";

export type DeviceTypeSummary = {
  id: string;
  typeKey: string;
  version: number;
  displayName: string;
  category: string;
  vendor: string | null;
  model: string | null;
  driverKey: string;
  driverVersion: string;
  status: DeviceTypeStatus;
};

export type DeviceSummary = {
  id: number;
  projectId: number;
  name: string;
  legacyType: string;
  status: DeviceStatus;
  deviceType: DeviceTypeSummary;
  lastSeenAt: Date | null;
  updatedAt: Date;
};
