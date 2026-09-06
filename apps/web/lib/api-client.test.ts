import assert from "node:assert/strict";
import test from "node:test";
import { APIError, createAPIClient } from "./api-client.ts";

const response = (body: unknown, status = 200) => Response.json(body, { status });

test("writes share CSRF lookup, preserve headers and use same-origin credentials", async () => {
  let csrfCalls = 0;
  let writes = 0;
  const client = createAPIClient({ fetch: async (path, init) => {
    assert.equal(init?.credentials, "same-origin");
    assert.equal(init?.cache, "no-store");
    if (path === "/api/auth/csrf") { csrfCalls++; return response({ csrfToken: "csrf-value" }); }
    writes++;
    const headers = new Headers(init?.headers);
    assert.equal(headers.get("x-csrf-token"), "csrf-value");
    assert.equal(headers.get("content-type"), "application/json");
    assert.equal(headers.get("idempotency-key"), "operation-key");
    assert.equal(init?.redirect, "error");
    return response({ id: writes });
  } });
  await Promise.all([1, 2, 3].map(() => client.json("/api/projects", { method: "POST", credentials: "omit", headers: { "Content-Type": "application/json", "Idempotency-Key": "operation-key" }, body: "{}" })));
  assert.equal(csrfCalls, 1);
  assert.equal(writes, 3);
});

test("read transport preserves streaming and Range without fetching CSRF", async () => {
  let calls = 0;
  const client = createAPIClient({ fetch: async (path, init) => {
    calls++;
    assert.equal(path, "/api/projects/1/assets/2/content");
    assert.equal(new Headers(init?.headers).get("range"), "bytes=0-2");
    assert.equal(new Headers(init?.headers).has("x-csrf-token"), false);
    return new Response("abc", { status: 206 });
  } });
  const raw = await client.request("/api/projects/1/assets/2/content", { headers: { Range: "bytes=0-2" } });
  assert.equal(raw.bodyUsed, false);
  assert.equal(await raw.text(), "abc");
  assert.equal(calls, 1);
});

test("401 notifies protected callers while login errors stay local; writes never retry", async () => {
  let notifications = 0, requests = 0, csrfCalls = 0;
  const client = createAPIClient({ onUnauthorized: () => notifications++, fetch: async (path) => {
    if (path === "/api/auth/csrf") { csrfCalls++; return response({ csrfToken: "token" }); }
    requests++;
    return response({ error: "UNAUTHENTICATED" }, 401);
  } });
  await assert.rejects(client.json("/api/projects", { method: "POST" }), (error: unknown) => error instanceof APIError && error.status === 401 && error.code === "UNAUTHENTICATED");
  await assert.rejects(client.json("/api/auth/login", { method: "POST", auth: "optional" }));
  assert.equal(notifications, 1);
  assert.equal(requests, 2);
  assert.equal(csrfCalls, 2);
});

test("CSRF failure invalidates cache without replaying a mutation", async () => {
  let csrfCalls = 0, writes = 0;
  const client = createAPIClient({ fetch: async (path) => {
    if (path === "/api/auth/csrf") return response({ csrfToken: `token-${++csrfCalls}` });
    writes++;
    return response({ error: "CSRF_FAILED" }, 403);
  } });
  await assert.rejects(client.json("/api/projects", { method: "POST" }));
  assert.equal(writes, 1);
  await assert.rejects(client.json("/api/projects", { method: "POST" }));
  assert.equal(csrfCalls, 2);
});

test("aborting one CSRF waiter does not cancel other callers or submit the abandoned write", async () => {
  let release!: (value: Response) => void;
  let writes = 0;
  const client = createAPIClient({ fetch: async (path) => {
    if (path === "/api/auth/csrf") return new Promise<Response>((resolve) => { release = resolve; });
    writes++; return new Response(null, { status: 204 });
  } });
  const controller = new AbortController();
  const first = client.json("/api/projects", { method: "POST", signal: controller.signal });
  const second = client.json("/api/projects", { method: "POST" });
  controller.abort();
  await assert.rejects(first, { name: "AbortError" });
  release(response({ csrfToken: "token" }));
  assert.equal(await second, undefined);
  assert.equal(writes, 1);
});

test("external and normalized non-API paths never reach the network", async () => {
  const client = createAPIClient({ fetch: async () => { throw new Error("network must not run"); } });
  for (const path of ["https://evil.test/api/x", "//evil.test/api/x", "/api/../other", "/api/\\evil", "/other"]) {
    await assert.rejects(client.request(path), (error: unknown) => error instanceof APIError && error.code === "API_PATH_MUST_BE_SAME_ORIGIN");
  }
});

test("malformed CSRF and HTML errors produce safe typed errors", async () => {
  const csrf = createAPIClient({ fetch: async () => response({ csrfToken: null }) });
  await assert.rejects(csrf.json("/api/projects", { method: "POST" }), (error: unknown) => error instanceof APIError && error.code === "CSRF_RESPONSE_INVALID");
  const html = createAPIClient({ fetch: async () => new Response("<html>private proxy error</html>", { status: 502 }) });
  await assert.rejects(html.json("/api/projects"), (error: unknown) => error instanceof APIError && error.code === "HTTP_502");
});
