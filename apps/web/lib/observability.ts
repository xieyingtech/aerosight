import { randomUUID } from "node:crypto";

const CORRELATION_ID_PATTERN = /^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$/;
const SENSITIVE_KEY_PATTERN = /authorization|cookie|credential|password|secret|token|api[-_]?key|access[-_]?key|(^|[-_])sn($|[-_])|(^|[-_])sts($|[-_])|serial|sn[-_]?decrypt|mapping|object[-_]?key[-_]?prefix|signed[-_]?url|playback[-_]?url|publish[-_]?url|upstream.*(error|message|body)|response[-_]?body|raw[-_]?error/i;
const BEARER_VALUE = /\bBearer\s+[A-Za-z0-9._~+\/-]+=*/gi;
const QUERY_SECRET = /([?&](?:token|api[-_]?key|key|secret|signature|x-amz-credential|x-amz-signature|x-amz-security-token|security-token|credential)=)[^&\s]+/gi;
const JWT_VALUE = /\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b/g;
const LABELED_SECRET = /("?(?:x-user-token|access_key_id|access_key_secret|security_token|session_token|live_token|device_sn|encrypted_sns|sn|serial_number)"?\s*[:=]\s*"?)[^",\s}&]+/gi;
const DJI_SERIAL = /\b(?:7CT|1581F)[A-Z0-9]{8,}\b/g;

export type CorrelationContext = {
  requestId: string;
  eventId?: string;
};

export function correlationId(candidate?: string | null) {
  const value = candidate?.trim();
  return value && CORRELATION_ID_PATTERN.test(value) ? value : randomUUID();
}

function redactText(value: string) {
  return value
    .replace(BEARER_VALUE, "Bearer [REDACTED]")
    .replace(QUERY_SECRET, "$1[REDACTED]")
    .replace(JWT_VALUE, "[JWT_REDACTED]")
    .replace(LABELED_SECRET, "$1[REDACTED]")
    .replace(DJI_SERIAL, "[SN_REDACTED]");
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
