export type DiscoveryStatus = "managed" | "discovered" | "conflicted" | "ignored" | "missing";

export type DeviceDiscovery = {
  id: string;
  connectorId: string;
  connectorName: string;
  connectorKey: string;
  externalDeviceId: string;
  externalDeviceType: string | null;
  parentExternalId: string | null;
  status: DiscoveryStatus;
  suggestedTypeKey: string | null;
  suggestedTypeName: string | null;
  matchConfidence: number | null;
  deviceId: number | null;
  lastSeenAt: string;
};

export type DeviceTypeOption = { id: string; typeKey: string; displayName: string; category: string };
export type DiscoveryConnector = { id: string; name: string; connectorKey: string; status: string; canScan: boolean };

export const DISCOVERY_STATUS_LABELS: Record<DiscoveryStatus, string> = {
  managed: "已纳管",
  discovered: "待确认",
  conflicted: "冲突",
  ignored: "已忽略",
  missing: "来源缺失",
};

export function canConfirmDiscovery(status: DiscoveryStatus) {
  return status === "discovered";
}

export function filterDiscoveries(
  discoveries: DeviceDiscovery[],
  filters: { status?: DiscoveryStatus | "all"; connectorId?: string; query?: string }
) {
  const query = filters.query?.trim().toLocaleLowerCase() ?? "";
  return discoveries.filter((item) => {
    if (filters.status && filters.status !== "all" && item.status !== filters.status) return false;
    if (filters.connectorId && filters.connectorId !== "all" && item.connectorId !== filters.connectorId) return false;
    if (!query) return true;
    return [item.externalDeviceId, item.externalDeviceType, item.connectorName, item.suggestedTypeName, item.suggestedTypeKey]
      .some((value) => value?.toLocaleLowerCase().includes(query));
  });
}
