export type DeviceTreeItem = {
  id: number;
  name: string;
  category: string;
  status: string;
  dataFreshness: string;
  statusReason: string | null;
  typeName: string;
  typeKey: string;
  driverKey: string;
  driverVersion: string;
  vendor: string | null;
  model: string | null;
  capabilities: { code: string; availability: string; reason: string | null; risk: string }[];
  channels: { stableChannelId: string; name: string; dataType: string; availability: string }[];
};

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
