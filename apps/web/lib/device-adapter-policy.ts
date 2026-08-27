import { z } from "zod";

const sensitiveKey = /authorization|cookie|credential|password|secret|token|api[-_]?key/i;

export const deviceAdapterInputSchema = z.object({
  name: z.string().trim().min(1).max(100),
  adapterType: z.enum(["simulator", "dji", "ros2", "mqtt", "mavlink", "rtsp", "gb28181"]),
  vendor: z.string().trim().max(100).optional(),
  protocolVersion: z.string().trim().min(1).max(50).default("1"),
  secretRef: z.string().trim().regex(/^[a-z][a-z0-9+.-]*:\/\/.+/i).optional(),
  config: z.record(z.string(), z.unknown()).default({})
});

export const djiAdapterSetupInputSchema = z.object({
  name: z.string().trim().min(1).max(100),
  mode: z.enum(["lan", "public"]),
  mqttEndpoint: z.string().trim().min(1).max(500),
  apiPublicBaseUrl: z.string().trim().min(1).max(500),
  websocketPublicUrl: z.string().trim().min(1).max(500),
  mediaIngestBaseUrl: z.string().trim().min(1).max(500),
  mediaPlaybackBaseUrl: z.string().trim().min(1).max(500),
  tlsRequired: z.boolean(),
  mqttAnonymous: z.boolean().default(false),
  secretRef: z.string().trim().regex(/^[a-z][a-z0-9+.-]*:\/\/.+/i),
  gatewaySerials: z.array(z.string().trim().min(1).max(100)).min(1).max(100)
});

export type DeviceAdapterInput = z.infer<typeof deviceAdapterInputSchema>;
export type DjiAdapterSetupInput = z.infer<typeof djiAdapterSetupInputSchema>;

export function canManageDeviceAdapters(role: "owner" | "admin" | "member" | null) {
  return role === "owner" || role === "admin";
}

export function assertSupportedDeviceAdapterType(adapterType: DeviceAdapterInput["adapterType"]) {
  if (adapterType !== "simulator" && adapterType !== "dji") {
    throw new Error(`ADAPTER_TYPE_NOT_SUPPORTED:${adapterType}`);
  }
}

export function assertNoInlineSecrets(value: unknown, path = "config") {
  if (!value || typeof value !== "object") return;
  if (Array.isArray(value)) {
    value.forEach((item, index) => assertNoInlineSecrets(item, `${path}[${index}]`));
    return;
  }
  for (const [key, item] of Object.entries(value as Record<string, unknown>)) {
    if (sensitiveKey.test(key)) {
      throw new Error(`INLINE_SECRET_NOT_ALLOWED:${path}.${key}`);
    }
    assertNoInlineSecrets(item, `${path}.${key}`);
  }
}

export function publicDeviceAdapter<T extends { secretRef: string | null }>(adapter: T) {
  const { secretRef, ...safe } = adapter;
  return { ...safe, hasSecret: secretRef !== null };
}
