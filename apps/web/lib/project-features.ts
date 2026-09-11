export type FeatureNode = { id: string; label: string; description?: string; children?: FeatureNode[] };
export type FeatureValues = Record<string, boolean>;
export type FeatureSettings = { projectId: number; tree: FeatureNode[]; values: FeatureValues };
export function featureLeaves(node: FeatureNode): string[] {
  return node.children?.length ? node.children.flatMap(featureLeaves) : [node.id];
}
export function featureSelection(node: FeatureNode, values: FeatureValues): "all" | "some" | "none" {
  const leaves = featureLeaves(node);
  const enabled = leaves.filter(id => values[id] === true).length;
  return enabled === leaves.length ? "all" : enabled === 0 ? "none" : "some";
}
export function featureChanges(saved: FeatureValues, draft: FeatureValues) {
  return Object.fromEntries(Object.keys(saved).filter(key => draft[key] !== saved[key])
    .map(key => [key, { expected: saved[key], enabled: draft[key] }]));
}
