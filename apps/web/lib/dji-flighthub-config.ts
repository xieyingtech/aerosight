import { z } from "zod";

export const DJI_FLIGHTHUB_CHINA_API_ORIGIN = "https://es-flight-api-cn.djigate.com";
export const DJI_FLIGHTHUB_CONSOLE_URL = "https://fh.dji.com";

const environmentSchema = z.object({
  DJI_FLIGHTHUB_ENABLED: z.enum(["true", "false"]).default("false"),
  DJI_FLIGHTHUB_API_BASE_URL: z.string().trim().default(DJI_FLIGHTHUB_CHINA_API_ORIGIN),
  DJI_FLIGHTHUB_HTTP_TIMEOUT_MS: z.coerce.number().int().min(500).max(30_000).default(8_000),
  DJI_FLIGHTHUB_MAX_RETRIES: z.coerce.number().int().min(0).max(3).default(2),
  DJI_FLIGHTHUB_MAX_PROJECT_PAGES: z.coerce.number().int().min(1).max(100).default(50),
  DJI_FLIGHTHUB_MAX_RESPONSE_BYTES: z.coerce.number().int().min(1_024).max(16_777_216).default(4_194_304),
});

export type FlightHubWebConfig = {
  enabled: boolean;
  apiBaseUrl: string;
  timeoutMs: number;
  maxRetries: number;
  maxProjectPages: number;
  maxResponseBytes: number;
};

function normalizeOfficialOrigin(rawValue: string) {
  let url: URL;
  try {
    url = new URL(rawValue);
  } catch {
    throw new Error("DJI_FLIGHTHUB_API_BASE_URL_INVALID");
  }
  if (
    url.protocol !== "https:" ||
    url.username ||
    url.password ||
    url.origin !== DJI_FLIGHTHUB_CHINA_API_ORIGIN ||
    (url.pathname !== "/" && url.pathname !== "") ||
    url.search ||
    url.hash
  ) {
    throw new Error("DJI_FLIGHTHUB_API_BASE_URL_NOT_ALLOWED");
  }
  return url.origin;
}

export function parseFlightHubWebConfig(
  environment: Record<string, string | undefined>
): FlightHubWebConfig {
  const parsed = environmentSchema.safeParse(environment);
  if (!parsed.success) {
    throw new Error("DJI_FLIGHTHUB_CONFIGURATION_INVALID");
  }
  return {
    enabled: parsed.data.DJI_FLIGHTHUB_ENABLED === "true",
    apiBaseUrl: normalizeOfficialOrigin(parsed.data.DJI_FLIGHTHUB_API_BASE_URL),
    timeoutMs: parsed.data.DJI_FLIGHTHUB_HTTP_TIMEOUT_MS,
    maxRetries: parsed.data.DJI_FLIGHTHUB_MAX_RETRIES,
    maxProjectPages: parsed.data.DJI_FLIGHTHUB_MAX_PROJECT_PAGES,
    maxResponseBytes: parsed.data.DJI_FLIGHTHUB_MAX_RESPONSE_BYTES,
  };
}
