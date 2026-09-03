import assert from "node:assert/strict";
import test from "node:test";

import {
  FlightHubClientError,
  FlightHubProjectClient,
  type FlightHubFetch,
} from "./dji-flighthub-client-core.ts";
import {
  DJI_FLIGHTHUB_CHINA_API_ORIGIN,
  parseFlightHubWebConfig,
} from "./dji-flighthub-config.ts";

const PROJECT_UUID = "00000000-0000-4000-8000-000000000001";
const ORG_UUID = "00000000-0000-4000-8000-000000000010";

function jsonResponse(body: unknown, init: ResponseInit = {}) {
  return new Response(JSON.stringify(body), {
    status: init.status ?? 200,
    headers: { "content-type": "application/json", ...init.headers },
  });
}

function client(fetchImpl: FlightHubFetch, overrides: Partial<ConstructorParameters<typeof FlightHubProjectClient>[0]> = {}) {
  return new FlightHubProjectClient({
    apiBaseUrl: DJI_FLIGHTHUB_CHINA_API_ORIGIN,
    timeoutMs: 1_000,
    maxRetries: 2,
    maxProjectPages: 3,
    maxResponseBytes: 32_768,
    requestId: () => "00000000-0000-4000-8000-000000000099",
    sleep: async () => {},
    fetchImpl,
    ...overrides,
  });
}

test("FlightHub deployment config only accepts the China official HTTPS origin", () => {
  const config = parseFlightHubWebConfig({});
  assert.equal(config.apiBaseUrl, DJI_FLIGHTHUB_CHINA_API_ORIGIN);
  for (const forbidden of [
    "http://es-flight-api-cn.djigate.com",
    "https://es-flight-api-cn.djigate.com/openapi",
    "https://user:pass@es-flight-api-cn.djigate.com",
    "https://example.test",
  ]) {
    assert.throws(
      () => parseFlightHubWebConfig({ DJI_FLIGHTHUB_API_BASE_URL: forbidden }),
      /NOT_ALLOWED/
    );
  }
});

test("project discovery paginates with minimal headers and no token in the URL", async () => {
  const calls: Array<{ url: URL; headers: Headers }> = [];
  const fetchImpl: FlightHubFetch = async (input, init) => {
    const url = new URL(input.toString());
    const headers = new Headers(init?.headers);
    calls.push({ url, headers });
    const page = Number(url.searchParams.get("page"));
    const list = page === 1
      ? Array.from({ length: 20 }, (_, index) => ({
          name: `脱敏项目${index}`,
          uuid: `00000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
          org_uuid: ORG_UUID,
        }))
      : [{ name: "尾页项目", uuid: "00000000-0000-4000-8000-000000000099", org_uuid: ORG_UUID }];
    return jsonResponse({ code: 0, message: "", data: { list } });
  };

  const projects = await client(fetchImpl).listProjects("token-only-in-header");
  assert.equal(projects.length, 21);
  assert.equal(calls.length, 2);
  for (const call of calls) {
    assert.equal(call.url.origin, DJI_FLIGHTHUB_CHINA_API_ORIGIN);
    assert.equal(call.url.pathname, "/openapi/v2.0/project");
    assert.equal(call.url.searchParams.get("usage"), "complete");
    assert.equal(call.url.searchParams.get("page_size"), "20");
    assert(!call.url.toString().includes("token-only-in-header"));
    assert.equal(call.headers.get("X-User-Token"), "token-only-in-header");
    assert.equal(call.headers.get("X-Project-Uuid"), null);
    assert.equal(call.headers.get("X-Language"), "zh");
  }
});

test("project discovery retries bounded 429 and 5xx responses with safe errors", async () => {
  const delays: number[] = [];
  let attempts = 0;
  const fetchImpl: FlightHubFetch = async () => {
    attempts += 1;
    if (attempts === 1) return jsonResponse({ code: 210429, message: "limited", data: {} }, { status: 429, headers: { "Retry-After": "3" } });
    if (attempts === 2) return jsonResponse({ code: 210500, message: "unavailable", data: {} }, { status: 503 });
    return jsonResponse({ code: 0, message: "", data: { list: [] } });
  };
  const projectClient = client(fetchImpl, { sleep: async (delay) => { delays.push(delay); } });
  assert.deepEqual(await projectClient.listProjects("redacted"), []);
  assert.equal(attempts, 3);
  assert.deepEqual(delays, [3_000, 500]);
});

test("authentication and schema failures are normalized without leaking upstream details", async () => {
  const secret = "sensitive-token-value";
  const authClient = client(async () => jsonResponse(
    { code: 200401, message: `invalid ${secret}`, data: {} },
    { status: 200 }
  ));
  await assert.rejects(
    () => authClient.listProjects(secret),
    (error) => error instanceof FlightHubClientError &&
      error.safeCode === "credential_invalid" &&
      !error.message.includes(secret)
  );

  const malformedClient = client(async () => jsonResponse({ code: 0, data: { list: "bad" } }));
  await assert.rejects(
    () => malformedClient.listProjects("redacted"),
    (error) => error instanceof FlightHubClientError && error.safeCode === "schema_incompatible"
  );
});

test("response size, duplicate pages, page ceilings, and request timeouts fail closed", async () => {
  const oversized = client(
    async () => jsonResponse({ code: 0, data: { list: [], padding: "x".repeat(2_000) } }),
    { maxResponseBytes: 128 }
  );
  await assert.rejects(
    () => oversized.listProjects("redacted"),
    (error) => error instanceof FlightHubClientError && error.safeCode === "response_too_large"
  );

  const fullPage = Array.from({ length: 20 }, (_, index) => ({
    name: `项目${index}`,
    uuid: `00000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
    org_uuid: ORG_UUID,
  }));
  const duplicate = client(async () => jsonResponse({ code: 0, data: { list: fullPage } }));
  await assert.rejects(
    () => duplicate.listProjects("redacted"),
    (error) => error instanceof FlightHubClientError && error.safeCode === "schema_incompatible"
  );

  let pageIndex = 0;
  const ceiling = client(async () => {
    pageIndex += 1;
    return jsonResponse({
      code: 0,
      data: {
        list: fullPage.map((project, index) => ({
          ...project,
          uuid: `00000000-0000-4000-${String(8000 + pageIndex).padStart(4, "0")}-${String(index + 1).padStart(12, "0")}`,
        })),
      },
    });
  }, { maxProjectPages: 2 });
  await assert.rejects(
    () => ceiling.listProjects("redacted"),
    (error) => error instanceof FlightHubClientError && error.safeCode === "project_page_limit"
  );

  const timeout = client(async (_input, init) => new Promise<Response>((_resolve, reject) => {
    init?.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
  }), { timeoutMs: 5, maxRetries: 0 });
  await assert.rejects(
    () => timeout.listProjects("redacted"),
    (error) => error instanceof FlightHubClientError && error.safeCode === "request_timeout"
  );
});

test("known project identifiers are normalized", async () => {
  const projectClient = client(async () => jsonResponse({
    code: 0,
    message: "",
    data: { list: [{ name: " 脱敏项目 ", uuid: PROJECT_UUID.toUpperCase(), org_uuid: ORG_UUID.toUpperCase() }] },
  }));
  assert.deepEqual(await projectClient.listProjects("redacted"), [{
    uuid: PROJECT_UUID,
    name: "脱敏项目",
    organizationUuid: ORG_UUID,
  }]);
});

test("join code lookup is a bounded global GET and returns a typed result", async () => {
  const calls:Array<{url:URL;headers:Headers}>=[];
  const projectClient=client(async(input,init)=>{
    calls.push({url:new URL(input.toString()),headers:new Headers(init?.headers)});
    return jsonResponse({code:0,message:"",data:{project_uuid:PROJECT_UUID,project_id:"PROJECT-1",project_name:"脱敏项目",
      organization_uuid:ORG_UUID,organization_id:"ORG-1",organization_name:"脱敏组织",is_user_in_organization:true,
      recommend_user_project_callsign:"测试用户",recommend_association_drone_project_callsign:"脱敏飞机"}});
  });
  const result=await projectClient.getJoinCodeInfo("token-only-in-header",{projectCode:"PROJECT-1",fastJoinCode:"JOIN-1",associationDroneSN:"AIRCRAFT-1"});
  assert.equal(result.projectUuid,PROJECT_UUID);
  assert.equal(result.organizationUuid,ORG_UUID);
  assert.equal(calls[0].url.pathname,"/openapi/v2.0/projects/join-codes");
  assert.equal(calls[0].url.searchParams.get("project_fast_join_code"),"JOIN-1");
  assert.equal(calls[0].headers.get("X-User-Token"),"token-only-in-header");
  assert.equal(calls[0].headers.get("X-Project-Uuid"),null);
  assert(!calls[0].url.toString().includes("token-only-in-header"));
  await assert.rejects(()=>projectClient.getJoinCodeInfo("redacted",{projectCode:"../bad",fastJoinCode:"JOIN-1"}),
    (error)=>error instanceof FlightHubClientError&&error.safeCode==="scope_forbidden");
});
