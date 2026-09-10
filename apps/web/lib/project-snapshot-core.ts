import type { ProjectHealth } from "./dependency-health-core.ts";
import type { OperationDiagnostic } from "./operation-diagnostics-core.ts";
import type { ProjectedDeviceCapability } from "./device-action-projection.ts";

export type ProjectSnapshotChannel = {
  stableChannelId: string;
  capabilityCode?: string;
  channelKey: string;
  displayName: string;
  dataType: string;
  availability: string;
  availabilityReason: string | null;
  protocol: string | null;
};

export type ProjectSnapshotDevice = Record<string, unknown> & {
  id?: number;
  deviceTypeId?: string;
  name?: string;
  type?: string;
  status?: string;
  typeKey?: string;
  typeVersion?: string;
  typeName?: string;
  category?: string;
  driverKey?: string;
  driverVersion?: string;
  driverStatus?: string;
  statusReason?: string | null;
  lastSeenAt?: string | Date | null;
  pose?: Record<string, unknown> | null;
  capabilities?: ProjectedDeviceCapability[];
  channels?: ProjectSnapshotChannel[];
};

export type ProjectSituationSnapshot = {
  project: { id: number; name: string; teamId: number; dependencyHealth?: Record<string, unknown> };
  generatedAt: string;
  consistency: "repeatable-read";
  devices: ProjectSnapshotDevice[];
  tracks: Array<Record<string, unknown>>;
  activeTasks: Array<Record<string, unknown>>;
  taskSteps: Array<Record<string, unknown>>;
  algorithmRuns: Array<Record<string, unknown>>;
  liveStreams: Array<Record<string, unknown>>;
  realtimeChannels?: Array<Record<string, unknown>>;
  diagnostics?: OperationDiagnostic[];
  mediaPoints: Array<Record<string, unknown>>;
  suspectedConstruction: Array<Record<string, unknown>>;
  openIssues: Array<Record<string, unknown>>;
  /** @deprecated legacy perception records kept for history/replay compatibility */
  openAlerts: Array<Record<string, unknown>>;
  regions: Array<Record<string, unknown>>;
  freshness: { latestCapturedAt: string | null; isRealtime: boolean };
  availability: Record<string, "available" | "not-configured" | "degraded">;
  health?: ProjectHealth;
};
