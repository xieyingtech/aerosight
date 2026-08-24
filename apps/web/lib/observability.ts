import { randomUUID } from "node:crypto";

const CORRELATION_ID_PATTERN = /^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$/;
const SENSITIVE_KEY_PATTERN = /authorization|cookie|credential|password|secret|token|api[-_]?key/i;
const SENSITIVE_TEXT_PATTERNS = [
  /\bBearer\s+[A-Za-z0-9._~+\/-]+=*/gi,
  /([?&](?:token|api_key|key|secret)=)[^&\s]+/gi
];

export type CorrelationContext = {
  requestId: string;
  eventId?: string;
};

export function correlationId(candidate?: string | null) {
  const value = candidate?.trim();
  return value && CORRELATION_ID_PATTERN.test(value) ? value : randomUUID();
}

function redactText(value: string) {
  return SENSITIVE_TEXT_PATTERNS.reduce(
    (current, pattern) => current.replace(pattern, (_match, prefix) => prefix ? `${prefix}[REDACTED]` : "Bearer [REDACTED]"),
    value
  );
}

export function redactSensitive(value: unknown, seen = new WeakSet<object>()): unknown {
  if (typeof value === "string") return redactText(value);
  if (value === null || typeof value !== "object") return value;
  if (seen.has(value)) return "[CIRCULAR]";
  seen.add(value);

  if (Array.isArray(value)) return value.map((item) => redactSensitive(item, seen));
  if (value instanceof Error) {
    return { name: value.name, message: redactText(value.message) };
  }

  return Object.fromEntries(
    Object.entries(value).map(([key, item]) => [
      key,
      SENSITIVE_KEY_PATTERN.test(key) ? "[REDACTED]" : redactSensitive(item, seen)
    ])
  );
}

export function structuredLog(
  level: "debug" | "info" | "warn" | "error",
  message: string,
  fields: Record<string, unknown> = {}
) {
  const payload = JSON.stringify({
    level,
    message,
    timestamp: new Date().toISOString(),
    ...redactSensitive(fields) as Record<string, unknown>
  });
  const output = level === "error" ? console.error : level === "warn" ? console.warn : console.info;
  output(payload);
}
