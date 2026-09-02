export type DeviceTreeItem = {
  id: number;
  deviceTypeId: string;
  name: string;
  category: string;
  status: string;
  dataFreshness: string;
  statusReason: string | null;
  positionStatus: string;
  positionReason: string | null;
  positionSource: string;
  pose: { longitude: number; latitude: number; altitudeMeters: number | null; capturedAt: string; calibrationStatus: "calibrated" | "unverified" } | null;
  typeName: string;
  typeKey: string;
  driverKey: string;
  driverVersion: string;
  vendor: string | null;
  model: string | null;
  flightHubControl?: {
    connectorStatus: string;
    stateFresh: boolean;
    cameraFeatureEnabled: boolean;
    cameraFieldVerified: boolean;
    lensFeatureEnabled: boolean;
    lensFieldVerified: boolean;
    tcaState: "available" | "empty" | "stale" | "unavailable" | "missing";
    tcaCheckedAt: string | null;
    tcaItemCount: number | null;
  } | null;
  capabilities: { code: string; availability: string; reason: string | null; risk: "low" | "medium" | "high" | "critical"; authorized: boolean; actions: Array<import("./device-capability-actions").DeviceCapabilityAction & { enabled: boolean; unavailableReason: string | null }> }[];
  channels: { stableChannelId: string; name: string; dataType: string; availability: string }[];
};

export function applyFlightHubDevicePrerequisites(device: DeviceTreeItem): DeviceTreeItem {
  if (!device.flightHubControl) return {
    ...device,
    capabilities: device.capabilities.map((capability) => ["camera.change", "camera.lens.change"].includes(capability.code)
      ? { ...capability, availability: "unavailable", reason: "当前主路由不支持司空相机控制" } : capability)
  };
  const gateReason = (kind: "camera" | "lens") => {
    const control = device.flightHubControl;
    if (!control || control.connectorStatus !== "connected") return "司空连接器未连接";
    if (!control.stateFresh) return "设备状态已过期";
    if (kind === "camera" && !control.cameraFeatureEnabled) return "相机切换功能未启用";
    if (kind === "lens" && !control.lensFeatureEnabled) return "镜头切换功能未启用";
    if (kind === "camera" && !control.cameraFieldVerified) return "当前型号/固件尚未完成相机切换现场验收";
    if (kind === "lens" && !control.lensFieldVerified) return "当前型号/固件尚未完成镜头切换现场验收";
    return null;
  };
  return {
    ...device,
    capabilities: device.capabilities.map((capability) => {
      const reason = capability.code === "camera.change" ? gateReason("camera")
        : capability.code === "camera.lens.change" ? gateReason("lens") : null;
      return reason ? { ...capability, availability: "unavailable", reason } : capability;
    })
  };
}

export type DeviceTreeRelation = {
  fromDeviceId: number;
  toDeviceId: number;
  relationType: string;
};

export type DeviceTreeNode = DeviceTreeItem & {
  relationType: string | null;
  children: DeviceTreeNode[];
};

export function buildDeviceTree(devices: DeviceTreeItem[], relations: DeviceTreeRelation[]) {
  const byId = new Map(devices.map((device) => [device.id, device]));
  const parentByChild = new Map<number, DeviceTreeRelation>();
  for (const relation of relations) {
    if (byId.has(relation.fromDeviceId) && byId.has(relation.toDeviceId)
        && relation.fromDeviceId !== relation.toDeviceId && !parentByChild.has(relation.toDeviceId)) {
      parentByChild.set(relation.toDeviceId, relation);
    }
  }
  const build = (device: DeviceTreeItem, ancestry: Set<number>): DeviceTreeNode => {
    const nextAncestry = new Set(ancestry).add(device.id);
    const children = relations
      .filter((relation) => relation.fromDeviceId === device.id && !nextAncestry.has(relation.toDeviceId))
      .map((relation) => byId.get(relation.toDeviceId))
      .filter((child): child is DeviceTreeItem => Boolean(child))
      .map((child) => build(child, nextAncestry));
    return { ...device, relationType: parentByChild.get(device.id)?.relationType ?? null, children };
  };
  const roots = devices.filter((device) => !parentByChild.has(device.id)).map((device) => build(device, new Set()));
  const reachable = new Set<number>();
  const visit = (node: DeviceTreeNode) => { reachable.add(node.id); node.children.forEach(visit); };
  roots.forEach(visit);
  for (const device of devices) if (!reachable.has(device.id)) roots.push(build(device, new Set()));
  return roots;
}
