import type { DeviceTreeNode } from "./device-tree-core";

export function flattenDeviceTree(nodes: DeviceTreeNode[]): DeviceTreeNode[] {
  return nodes.flatMap((node) => [node, ...flattenDeviceTree(node.children)]);
}

export function filterDeviceTree(nodes: DeviceTreeNode[], query: string): DeviceTreeNode[] {
  const normalizedQuery = query.trim().toLocaleLowerCase();
  if (!normalizedQuery) return nodes;

  return nodes.flatMap((node) => {
    const children = filterDeviceTree(node.children, normalizedQuery);
    const searchable = [node.name, node.category, node.typeName, node.typeKey, node.driverKey, node.vendor, node.model]
      .filter(Boolean)
      .join(" ")
      .toLocaleLowerCase();
    return searchable.includes(normalizedQuery) || children.length ? [{ ...node, children }] : [];
  });
}
