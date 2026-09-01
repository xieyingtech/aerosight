import assert from "node:assert/strict";
import test from "node:test";

import { FlightHubClientError } from "./dji-flighthub-client-core.ts";
import {
  discoverFlightHubProjectsCore,
  FlightHubDiscoveryError,
  FlightHubDiscoveryRateLimiter,
  flightHubDiscoveryInputSchema,
  type FlightHubDiscoveryAudit,
} from "./dji-flighthub-discovery-core.ts";

const TOKEN = "token-must-never-be-observable";

function dependencies(overrides: Record<string, unknown> = {}) {
  const audits: FlightHubDiscoveryAudit[] = [];
  const logs: unknown[] = [];
  let upstreamCalls = 0;
  return {
    audits,
    logs,
    upstreamCalls: () => upstreamCalls,
    value: {
      projectId: 10,
      accessProjectId: 10,
      actorUserId: 20,
      role: "owner" as const,
      token: TOKEN,
      client: {
        listProjects: async () => {
          upstreamCalls += 1;
          return [{
            uuid: "00000000-0000-4000-8000-000000000001",
            name: "脱敏项目",
            organizationUuid: "00000000-0000-4000-8000-000000000010",
          }];
        },
      },
      rateLimiter: new FlightHubDiscoveryRateLimiter(),
      audit: async (summary: FlightHubDiscoveryAudit) => { audits.push(summary); },
      log: (summary: unknown) => { logs.push(summary); },
      ...overrides,
    },
  };
}

test("only owner and admin can discover FlightHub projects", async () => {
  for (const role of ["owner", "admin"] as const) {
    const fixture = dependencies({ role });
    const projects = await discoverFlightHubProjectsCore(fixture.value);
    assert.equal(projects.length, 1);
    assert.equal(fixture.upstreamCalls(), 1);
  }

  for (const role of ["member", null] as const) {
    const fixture = dependencies({ role });
    await assert.rejects(
      () => discoverFlightHubProjectsCore(fixture.value),
      (error) => error instanceof FlightHubDiscoveryError && error.safeCode === "access_denied"
    );
    assert.equal(fixture.upstreamCalls(), 0);
  }

  const crossProject = dependencies({ accessProjectId: 999 });
  await assert.rejects(
    () => discoverFlightHubProjectsCore(crossProject.value),
    (error) => error instanceof FlightHubDiscoveryError && error.safeCode === "access_denied"
  );
  assert.equal(crossProject.upstreamCalls(), 0);
});

test("project discovery rate limit is scoped by actor and AeroSight project", async () => {
  let now = 1_000;
  const limiter = new FlightHubDiscoveryRateLimiter(2, 60_000, () => now);
  const first = dependencies({ rateLimiter: limiter });
  await discoverFlightHubProjectsCore(first.value);
  await discoverFlightHubProjectsCore(first.value);
  await assert.rejects(
    () => discoverFlightHubProjectsCore(first.value),
    (error) => error instanceof FlightHubDiscoveryError &&
      error.safeCode === "rate_limited" && error.retryAfterSeconds === 60
  );
  assert.equal(first.upstreamCalls(), 2);

  const otherProject = dependencies({ rateLimiter: limiter, projectId: 11, accessProjectId: 11 });
  await discoverFlightHubProjectsCore(otherProject.value);
  const otherActor = dependencies({ rateLimiter: limiter, actorUserId: 21 });
  await discoverFlightHubProjectsCore(otherActor.value);

  now += 60_001;
  await discoverFlightHubProjectsCore(first.value);
});

test("discovery audit and logs contain only safe summaries", async () => {
  const success = dependencies();
  await discoverFlightHubProjectsCore(success.value);
  assert.deepEqual(success.audits, [{ status: "succeeded", projectCount: 1 }]);
  assert(!JSON.stringify([success.audits, success.logs]).includes(TOKEN));

  const failure = dependencies({
    client: {
      listProjects: async () => {
        throw new FlightHubClientError("credential_invalid", false, 401);
      },
    },
  });
  await assert.rejects(
    () => discoverFlightHubProjectsCore(failure.value),
    (error) => error instanceof FlightHubDiscoveryError &&
      error.safeCode === "credential_invalid" && !error.message.includes(TOKEN)
  );
  assert.deepEqual(failure.audits, [{
    status: "failed",
    projectCount: 0,
    errorCode: "credential_invalid",
  }]);
  assert(!JSON.stringify([failure.audits, failure.logs]).includes(TOKEN));
});

test("discovery input rejects SSRF fields and oversized credentials", () => {
  assert.deepEqual(flightHubDiscoveryInputSchema.parse({ token: "redacted" }), { token: "redacted" });
  assert.throws(() => flightHubDiscoveryInputSchema.parse({
    token: "redacted",
    baseUrl: "https://example.test",
  }));
  assert.throws(() => flightHubDiscoveryInputSchema.parse({ token: "x".repeat(16_385) }));
});
