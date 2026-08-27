import assert from "node:assert/strict";
import test from "node:test";

import { assertSafeOutboundUrl, followSafeRedirects, isRestrictedAddress, type HostResolver } from "./outbound-url-policy.ts";

const publicResolver: HostResolver = async () => [{ address: "203.0.113.10", family: 4 }];
const policy = { allowedHosts: ["api.example.test", "*.models.example.test"], resolver: publicResolver };

test("only HTTPS allowlisted hosts with public DNS results are accepted", async () => {
  const target = await assertSafeOutboundUrl("https://api.example.test/v1/run", policy);
  assert.equal(target.url.hostname, "api.example.test");
  await assert.rejects(() => assertSafeOutboundUrl("http://api.example.test", policy), /HTTPS_REQUIRED/);
  await assert.rejects(() => assertSafeOutboundUrl("https://evil.example.test", policy), /HOST_NOT_ALLOWED/);
  await assert.rejects(() => assertSafeOutboundUrl("https://user:pass@api.example.test", policy), /CREDENTIALS_FORBIDDEN/);
});

test("loopback, link-local, metadata, private and mapped private addresses are restricted", () => {
  for (const address of ["127.0.0.1", "169.254.169.254", "10.0.0.3", "172.16.2.1", "192.168.1.1", "::1", "fe80::1", "fd00::1", "::ffff:127.0.0.1"]) {
    assert.equal(isRestrictedAddress(address), true, `${address} should be restricted`);
  }
  assert.equal(isRestrictedAddress("8.8.8.8"), false);
  assert.equal(isRestrictedAddress("2001:4860:4860::8888"), false);
});

test("a restricted DNS answer fails even when another answer is public", async () => {
  await assert.rejects(() => assertSafeOutboundUrl("https://api.example.test", {
    ...policy, resolver: async () => [{ address: "203.0.113.10", family: 4 }, { address: "10.0.0.2", family: 4 }]
  }), /ADDRESS_RESTRICTED/);
});

test("every redirect is allowlisted and DNS validated again", async () => {
  let resolves = 0;
  const resolver: HostResolver = async () => ++resolves === 1
    ? [{ address: "203.0.113.10", family: 4 }]
    : [{ address: "169.254.169.254", family: 4 }];
  let calls = 0;
  await assert.rejects(() => followSafeRedirects("https://api.example.test/start", { ...policy, resolver }, async () => {
    calls += 1;
    return { status: 302, location: "https://api.example.test/rebound" };
  }), /ADDRESS_RESTRICTED/);
  assert.equal(calls, 1);
  assert.equal(resolves, 2);
});

test("redirect to an unallowlisted host is rejected before transport", async () => {
  let calls = 0;
  await assert.rejects(() => followSafeRedirects("https://api.example.test/start", policy, async () => {
    calls += 1;
    return { status: 302, location: "https://metadata.invalid/latest" };
  }), /HOST_NOT_ALLOWED/);
  assert.equal(calls, 1);
});
