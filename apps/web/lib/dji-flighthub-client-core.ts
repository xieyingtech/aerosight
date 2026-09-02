export type FlightHubProject = {
  uuid: string;
  name: string;
  organizationUuid: string;
};

export type FlightHubJoinCodeInfo = {
  projectUuid: string;
  projectCode: string;
  projectName: string;
  organizationUuid: string;
  organizationCode: string;
  organizationName: string;
  userInOrganization: boolean;
  recommendedUserCallsign: string;
  recommendedDroneCallsign: string | null;
};

export type FlightHubSafeErrorCode =
  | "credential_invalid"
  | "scope_forbidden"
  | "scope_not_found"
  | "rate_limited"
  | "upstream_unavailable"
  | "request_timeout"
  | "response_too_large"
  | "schema_incompatible"
  | "project_page_limit"
  | "upstream_error";

export class FlightHubClientError extends Error {
  readonly safeCode: FlightHubSafeErrorCode;
  readonly retryable: boolean;
  readonly httpStatus?: number;
  readonly retryAfterSeconds?: number;

  constructor(
    safeCode: FlightHubSafeErrorCode,
    retryable: boolean,
    httpStatus?: number,
    retryAfterSeconds?: number
  ) {
    super(`DJI_FLIGHTHUB_${safeCode.toUpperCase()}`);
    this.name = "FlightHubClientError";
    this.safeCode = safeCode;
    this.retryable = retryable;
    this.httpStatus = httpStatus;
    this.retryAfterSeconds = retryAfterSeconds;
  }
}

export type FlightHubFetch = (
  input: string | URL | Request,
  init?: RequestInit
) => Promise<Response>;

export type FlightHubClientOptions = {
  apiBaseUrl: string;
  timeoutMs: number;
  maxRetries: number;
  maxProjectPages: number;
  maxResponseBytes: number;
  fetchImpl?: FlightHubFetch;
  requestId?: () => string;
  sleep?: (milliseconds: number) => Promise<void>;
};

type UpstreamEnvelope = {
  code: number;
  message?: unknown;
  data?: unknown;
};

const UUID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const PROJECT_PAGE_SIZE = 20;
const MAX_RETRY_AFTER_SECONDS = 10;

function defaultRequestId() {
  return crypto.randomUUID();
}

function defaultSleep(milliseconds: number) {
  return new Promise<void>((resolve) => setTimeout(resolve, milliseconds));
}

function isObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function parseRetryAfter(value: string | null) {
  if (!value) return undefined;
  const seconds = Number(value);
  if (Number.isFinite(seconds) && seconds >= 0) {
    return Math.min(Math.ceil(seconds), MAX_RETRY_AFTER_SECONDS);
  }
  const date = Date.parse(value);
  if (!Number.isFinite(date)) return undefined;
  return Math.min(Math.max(0, Math.ceil((date - Date.now()) / 1_000)), MAX_RETRY_AFTER_SECONDS);
}

function safeErrorForStatus(status: number, retryAfter: string | null) {
  if (status === 401) return new FlightHubClientError("credential_invalid", false, status);
  if (status === 403) return new FlightHubClientError("scope_forbidden", false, status);
  if (status === 404) return new FlightHubClientError("scope_not_found", false, status);
  if (status === 429) {
    return new FlightHubClientError("rate_limited", true, status, parseRetryAfter(retryAfter));
  }
  if (status >= 500) return new FlightHubClientError("upstream_unavailable", true, status);
  if (status >= 400) return new FlightHubClientError("upstream_error", false, status);
  return null;
}

function safeErrorForBusinessCode(code: number, status: number) {
  if (code === 0) return null;
  if (code === 200401) return new FlightHubClientError("credential_invalid", false, status);
  if (code === 200403) return new FlightHubClientError("scope_forbidden", false, status);
  if (code === 200404) return new FlightHubClientError("scope_not_found", false, status);
  if (code === 210429) return new FlightHubClientError("rate_limited", true, status);
  if ([200500, 210318, 210500, 210504].includes(code)) {
    return new FlightHubClientError("upstream_unavailable", true, status);
  }
  return new FlightHubClientError("upstream_error", false, status);
}

async function readBoundedJson(response: Response, maxResponseBytes: number): Promise<UpstreamEnvelope> {
  const declaredSize = Number(response.headers.get("content-length"));
  if (Number.isFinite(declaredSize) && declaredSize > maxResponseBytes) {
    throw new FlightHubClientError("response_too_large", false, response.status);
  }
  const bytes = new Uint8Array(await response.arrayBuffer());
  if (bytes.byteLength > maxResponseBytes) {
    throw new FlightHubClientError("response_too_large", false, response.status);
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(new TextDecoder().decode(bytes));
  } catch {
    throw new FlightHubClientError("schema_incompatible", false, response.status);
  }
  if (!isObject(parsed) || typeof parsed.code !== "number") {
    throw new FlightHubClientError("schema_incompatible", false, response.status);
  }
  return parsed as UpstreamEnvelope;
}

function parseProjectPage(payload: UpstreamEnvelope): FlightHubProject[] {
  if (!isObject(payload.data) || !Array.isArray(payload.data.list)) {
    throw new FlightHubClientError("schema_incompatible", false, 200);
  }
  return payload.data.list.map((value) => {
    if (
      !isObject(value) ||
      typeof value.uuid !== "string" ||
      !UUID_PATTERN.test(value.uuid) ||
      typeof value.name !== "string" ||
      !value.name.trim() ||
      typeof value.org_uuid !== "string" ||
      !UUID_PATTERN.test(value.org_uuid)
    ) {
      throw new FlightHubClientError("schema_incompatible", false, 200);
    }
    return {
      uuid: value.uuid.toLowerCase(),
      name: value.name.trim(),
      organizationUuid: value.org_uuid.toLowerCase(),
    };
  });
}

function parseJoinCodeInfo(payload: UpstreamEnvelope): FlightHubJoinCodeInfo {
  const value=payload.data;
  if (!isObject(value) || typeof value.project_uuid !== "string" || !UUID_PATTERN.test(value.project_uuid)
      || typeof value.project_id !== "string" || !value.project_id.trim() || typeof value.project_name !== "string" || !value.project_name.trim()
      || typeof value.organization_uuid !== "string" || !UUID_PATTERN.test(value.organization_uuid)
      || typeof value.organization_id !== "string" || !value.organization_id.trim()
      || typeof value.organization_name !== "string" || !value.organization_name.trim()
      || typeof value.is_user_in_organization !== "boolean" || typeof value.recommend_user_project_callsign !== "string"
      || !(value.recommend_association_drone_project_callsign===null||typeof value.recommend_association_drone_project_callsign==="string")) {
    throw new FlightHubClientError("schema_incompatible",false,200);
  }
  return {projectUuid:value.project_uuid.toLowerCase(),projectCode:value.project_id.trim(),projectName:value.project_name.trim(),
    organizationUuid:value.organization_uuid.toLowerCase(),organizationCode:value.organization_id.trim(),organizationName:value.organization_name.trim(),
    userInOrganization:value.is_user_in_organization,recommendedUserCallsign:value.recommend_user_project_callsign.trim(),
    recommendedDroneCallsign:value.recommend_association_drone_project_callsign?.trim()||null};
}

export class FlightHubProjectClient {
  private readonly options: FlightHubClientOptions;
  private readonly fetchImpl: FlightHubFetch;
  private readonly requestId: () => string;
  private readonly sleep: (milliseconds: number) => Promise<void>;

  constructor(options: FlightHubClientOptions) {
    if (new URL(options.apiBaseUrl).origin !== options.apiBaseUrl) {
      throw new Error("DJI_FLIGHTHUB_CLIENT_BASE_URL_INVALID");
    }
    this.options = options;
    this.fetchImpl = options.fetchImpl ?? fetch;
    this.requestId = options.requestId ?? defaultRequestId;
    this.sleep = options.sleep ?? defaultSleep;
  }

  private async get(token: string, path: string, projectUuid?: string) {
    if (!token.trim()) throw new FlightHubClientError("credential_invalid", false);
    const target = new URL(path, `${this.options.apiBaseUrl}/`);

    for (let attempt = 0; attempt <= this.options.maxRetries; attempt += 1) {
      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(), this.options.timeoutMs);
      let retryAfterSeconds: number | undefined;
      try {
        const headers = new Headers({
          "X-User-Token": token,
          "X-Request-Id": this.requestId(),
          "X-Language": "zh",
          Accept: "application/json",
        });
        if (projectUuid) headers.set("X-Project-Uuid", projectUuid);
        const response = await this.fetchImpl(target, {
          method: "GET",
          headers,
          signal: controller.signal,
          cache: "no-store",
          redirect: "error",
        });
        const statusError = safeErrorForStatus(response.status, response.headers.get("retry-after"));
        if (statusError) throw statusError;
        const payload = await readBoundedJson(response, this.options.maxResponseBytes);
        const businessError = safeErrorForBusinessCode(payload.code, response.status);
        if (businessError) throw businessError;
        return payload;
      } catch (error) {
        let safeError: FlightHubClientError;
        if (error instanceof FlightHubClientError) {
          safeError = error;
        } else if (controller.signal.aborted) {
          safeError = new FlightHubClientError("request_timeout", true);
        } else {
          safeError = new FlightHubClientError("upstream_unavailable", true);
        }
        retryAfterSeconds = safeError.retryAfterSeconds;
        if (!safeError.retryable || attempt === this.options.maxRetries) throw safeError;
      } finally {
        clearTimeout(timer);
      }
      const delayMs = retryAfterSeconds === undefined
        ? Math.min(250 * 2 ** attempt, 2_000)
        : retryAfterSeconds * 1_000;
      await this.sleep(delayMs);
    }
    throw new FlightHubClientError("upstream_unavailable", true);
  }

  async listProjects(token: string): Promise<FlightHubProject[]> {
    const projects: FlightHubProject[] = [];
    const seen = new Set<string>();
    for (let page = 1; page <= this.options.maxProjectPages; page += 1) {
      const query = new URLSearchParams({
        usage: "complete",
        sort_column: "create_time",
        sort_type: "desc",
        page: String(page),
        page_size: String(PROJECT_PAGE_SIZE),
      });
      const payload = await this.get(token, `/openapi/v2.0/project?${query}`);
      const pageProjects = parseProjectPage(payload);
      for (const project of pageProjects) {
        if (seen.has(project.uuid)) {
          throw new FlightHubClientError("schema_incompatible", false, 200);
        }
        seen.add(project.uuid);
        projects.push(project);
      }
      if (pageProjects.length < PROJECT_PAGE_SIZE) return projects;
    }
    throw new FlightHubClientError("project_page_limit", false, 200);
  }

  async getJoinCodeInfo(token:string,input:{projectCode:string;fastJoinCode:string;associationDroneSN?:string}):Promise<FlightHubJoinCodeInfo>{
    const identifier=/^[A-Za-z0-9._:-]{1,256}$/;
    if(!identifier.test(input.projectCode)||!identifier.test(input.fastJoinCode)||(input.associationDroneSN!==undefined&&!identifier.test(input.associationDroneSN))){
      throw new FlightHubClientError("scope_forbidden",false);
    }
    const query=new URLSearchParams({project_id:input.projectCode,project_fast_join_code:input.fastJoinCode});
    if(input.associationDroneSN)query.set("association_drone_device_sn",input.associationDroneSN);
    return parseJoinCodeInfo(await this.get(token,`/openapi/v2.0/projects/join-codes?${query}`));
  }
}
