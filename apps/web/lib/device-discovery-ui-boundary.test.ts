import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path: string) => readFileSync(new URL(`../../../${path}`, import.meta.url), "utf8");

test("device page owns discovery lifecycle without connector configuration fields", () => {
  const page = read("apps/web/app/(app)/projects/devices/page.tsx");
  const manager = read("apps/web/components/device-discovery-manager.tsx");
  const core = read("apps/web/lib/device-discovery-core.ts");
  assert.match(page, /DeviceDiscoveryManager/);
  for (const state of ["已纳管", "待确认", "冲突", "已忽略", "来源缺失"]) assert.match(core, new RegExp(state));
  for (const action of ["扫描", "确认纳管", "忽略", "重新匹配"]) assert.match(manager, new RegExp(action));
  assert.doesNotMatch(manager, /password|credential|secret|token|mqttEndpoint|apiPublicBaseUrl/i);
});

test("discovery writes remain permission gated and route binding is explicit", async () => {
const source=[await (await import("node:fs/promises")).readFile(new URL("../../../apps/server/internal/httpapi/device_discoveries_main.go", import.meta.url), "utf8"),await (await import("node:fs/promises")).readFile(new URL("../../../apps/server/internal/database/queries/discovery_main.sql", import.meta.url), "utf8")].join("\n");
assert.match(source, /device:configure/);
assert.match(source, /authorizeWrite/);
assert.match(source, /device_connector_bindings/);
assert.match(source, /CONNECTOR_DISABLED/);
assert.match(source, /status='standby'/);
assert.match(source, /discovery_status='managed'/);

});
