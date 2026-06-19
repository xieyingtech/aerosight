import zh from "../messages/zh_cn.json";

type Messages = typeof zh;

export const messages: Messages = zh;

export function t(path: string, values?: Record<string, string | number>) {
  const value = path.split(".").reduce<unknown>((current, key) => {
    if (current && typeof current === "object" && key in current) {
      return (current as Record<string, unknown>)[key];
    }
    return undefined;
  }, messages);

  let text = typeof value === "string" ? value : path;
  for (const [key, replacement] of Object.entries(values ?? {})) {
    text = text.replaceAll(`{${key}}`, String(replacement));
  }
  return text;
}
