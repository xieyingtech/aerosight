export class APIError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string) {
    super(code);
    this.name = "APIError";
    this.status = status;
    this.code = code;
  }
}

export type APIRequestOptions = RequestInit & { auth?: "required" | "optional" };
export type SessionUser = { id: number; name: string; email: string | null; phone: string | null; role: "user" | "admin" };

function validateAPIPath(path: string) {
  const base = "https://aerosight.invalid";
  const parsed = new URL(path, base);
  if (!path.startsWith("/api/") || /[\\\r\n]/.test(path) || parsed.origin !== base || !parsed.pathname.startsWith("/api/")) {
    throw new APIError(0, "API_PATH_MUST_BE_SAME_ORIGIN");
  }
}

function waitForShared<T>(promise: Promise<T>, signal?: AbortSignal | null): Promise<T> {
  if (!signal) return promise;
  signal.throwIfAborted();
  return new Promise<T>((resolve, reject) => {
    const abort = () => { signal.removeEventListener("abort", abort); reject(signal.reason); };
    signal.addEventListener("abort", abort, { once: true });
    promise.then(
      (value) => { signal.removeEventListener("abort", abort); resolve(value); },
      (error: unknown) => { signal.removeEventListener("abort", abort); reject(error); }
    );
  });
}

export function createAPIClient(options: { fetch?: typeof fetch; onUnauthorized?: () => void } = {}) {
  const transport: typeof fetch = options.fetch ?? ((input, init) => globalThis.fetch(input, init));
  const unauthorized = options.onUnauthorized;
  let csrfToken: string | null = null;
  let csrfPending: Promise<string> | null = null;
  let generation = 0;

  function invalidateCSRF() { generation++; csrfToken = null; csrfPending = null; }

  async function csrf(signal?: AbortSignal | null): Promise<string> {
    signal?.throwIfAborted();
    if (csrfToken) return csrfToken;
    if (!csrfPending) {
      const current = generation;
      csrfPending = (async () => {
        const response = await transport("/api/auth/csrf", { credentials: "same-origin", cache: "no-store", signal: AbortSignal.timeout(10_000) });
        if (!response.ok) throw new APIError(response.status, "CSRF_FETCH_FAILED");
        const body: unknown = await response.json().catch(() => { throw new APIError(502, "CSRF_RESPONSE_INVALID"); });
        if (!body || typeof body !== "object" || !("csrfToken" in body) || typeof body.csrfToken !== "string" || !body.csrfToken) {
          throw new APIError(502, "CSRF_RESPONSE_INVALID");
        }
        if (generation === current) csrfToken = body.csrfToken;
        return body.csrfToken;
      })().finally(() => { if (generation === current) csrfPending = null; });
    }
    return waitForShared(csrfPending, signal);
  }

  async function request(path: string, options: APIRequestOptions = {}): Promise<Response> {
    validateAPIPath(path);
    const { auth = "required", ...init } = options;
    const method = (init.method ?? "GET").toUpperCase();
    const headers = new Headers(init.headers);
    if (!["GET", "HEAD", "OPTIONS"].includes(method)) headers.set("X-CSRF-Token", await csrf(init.signal));
    init.signal?.throwIfAborted();
    const response = await transport(path, { ...init, method, headers, credentials: "same-origin", cache: "no-store", redirect: "error" });
    if (response.status === 401) {
      invalidateCSRF();
      if (auth === "required") unauthorized?.();
    }
    if (response.status === 403) invalidateCSRF();
    return response;
  }

  async function json<T>(path: string, init: APIRequestOptions = {}): Promise<T> {
    const response = await request(path, init);
    if (!response.ok) {
      let code = `HTTP_${response.status}`;
      if (response.headers.get("content-type")?.includes("application/json")) {
        const body: unknown = await response.json().catch(() => null);
        if (body && typeof body === "object" && "error" in body && typeof body.error === "string") code = body.error;
      }
      throw new APIError(response.status, code);
    }
    if (response.status === 204 || init.method?.toUpperCase() === "HEAD") return undefined as T;
    try { return await response.json() as T; }
    catch { init.signal?.throwIfAborted(); throw new APIError(502, "API_RESPONSE_INVALID"); }
  }

  return { request, json, invalidateCSRF };
}

export const apiClient = createAPIClient({ onUnauthorized: () => {
  if (typeof window !== "undefined") window.dispatchEvent(new Event("aerosight:unauthenticated"));
} });
export const apiFetch = apiClient.request;
export const apiJSON = apiClient.json;

export function getSession(signal?: AbortSignal) {
  return apiJSON<{ user: SessionUser }>("/api/auth/session", { signal, auth: "optional" });
}

export async function login(username: string, password: string) {
  const result = await apiJSON<{ user: SessionUser }>("/api/auth/login", {
    method: "POST", auth: "optional", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ username, password })
  });
  apiClient.invalidateCSRF();
  return result;
}

export async function logout() {
  await apiJSON<void>("/api/auth/logout", { method: "POST", auth: "optional" });
  apiClient.invalidateCSRF();
}
