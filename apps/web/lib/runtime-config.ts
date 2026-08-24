import { z } from "zod";

const runtimeConfigSchema = z.object({
  DATABASE_URL: z.preprocess(
    (value) => value ?? "",
    z.string().trim().min(1, "DATABASE_URL is required")
  ),
  AUTH_SECRET: z.preprocess(
    (value) => value ?? "",
    z.string().min(16, "AUTH_SECRET must contain at least 16 characters")
  ),
  LOG_LEVEL: z.enum(["debug", "info", "warn", "error"]).default("info")
});

export type WebRuntimeConfig = {
  databaseUrl: string;
  authSecret: string;
  logLevel: "debug" | "info" | "warn" | "error";
};

export function parseWebRuntimeConfig(environment: Record<string, string | undefined>): WebRuntimeConfig {
  const parsed = runtimeConfigSchema.safeParse(environment);
  if (!parsed.success) {
    const details = parsed.error.issues.map((issue) => issue.message).join("; ");
    throw new Error(`Invalid AeroSight web configuration: ${details}`);
  }
  return {
    databaseUrl: parsed.data.DATABASE_URL,
    authSecret: parsed.data.AUTH_SECRET,
    logLevel: parsed.data.LOG_LEVEL
  };
}

let cachedConfig: WebRuntimeConfig | undefined;

export function getWebRuntimeConfig() {
  cachedConfig ??= parseWebRuntimeConfig(process.env);
  return cachedConfig;
}
