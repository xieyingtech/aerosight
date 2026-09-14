import { parseDocument, stringify } from "yaml";

export type TaskSource = { format: "yaml" | "json"; source: string };
export function readTaskSource(value: TaskSource): Record<string, unknown> {
  let result: unknown;
  if (value.format === "json") result = JSON.parse(value.source);
  else {
    const document = parseDocument(value.source, { uniqueKeys: true });
    if (document.errors.length || document.warnings.length) throw new Error("YAML 格式无效，请先修正原始文本");
    result = document.toJS({ maxAliasCount: 0 });
  }
  if (!result || typeof result !== "object" || Array.isArray(result)) throw new Error("任务定义必须是对象");
  return result as Record<string, unknown>;
}
export function switchTaskFormat(value: TaskSource, format: TaskSource["format"]): TaskSource {
  if (format === value.format) return value;
  const definition = readTaskSource(value);
  return { format, source: format === "yaml" ? stringify(definition) : JSON.stringify(definition, null, 2) };
}
export function editTaskField(value: TaskSource, path: Array<string | number>, replacement: unknown): TaskSource {
  const definition = readTaskSource(value);
  if (value.format === "yaml") {
    const document = parseDocument(value.source, { uniqueKeys: true });
    document.setIn(path, replacement);
    return { ...value, source: document.toString() };
  }
  let parent: Record<string | number, unknown> = definition;
  for (const part of path.slice(0, -1)) {
    const next = parent[part];
    if (!next || typeof next !== "object") parent[part] = {};
    parent = parent[part] as Record<string | number, unknown>;
  }
  parent[path[path.length - 1]] = replacement;
  return { ...value, source: JSON.stringify(definition, null, 2) };
}
