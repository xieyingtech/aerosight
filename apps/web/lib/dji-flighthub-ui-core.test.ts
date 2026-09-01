import assert from "node:assert/strict";
import test from "node:test";

import {
  defaultFlightHubProjectSelection,
  discoveryStatusLabel,
  flightHubErrorMessage,
  flightHubReadOnlyCapabilities,
  flightHubUnavailableActions,
} from "./dji-flighthub-ui-core.ts";

const project = (uuid: string) => ({ uuid, name: uuid, organizationUuid: "00000000-0000-4000-8000-000000000010" });

test("project discovery auto-selects only an unambiguous single project", () => {
  assert.equal(defaultFlightHubProjectSelection([]), "");
  assert.equal(defaultFlightHubProjectSelection([project("one")]), "one");
  assert.equal(defaultFlightHubProjectSelection([project("one"), project("two")]), "");
});

test("safe UI messages cover invalid token, revoked project and rate limiting", () => {
  assert.match(flightHubErrorMessage("credential_invalid"), /Token/);
  assert.match(flightHubErrorMessage("project_access_changed"), /重新验证/);
  assert.match(flightHubErrorMessage("rate_limited"), /频繁/);
  assert.doesNotMatch(flightHubErrorMessage("Bearer secret-token"), /secret-token/);
});

test("FlightHub UI exposes only read-only capabilities and conflict review", () => {
  assert.deepEqual(flightHubReadOnlyCapabilities, ["目录读取", "状态读取"]);
  assert.deepEqual(flightHubUnavailableActions, ["任务下发", "返航", "机场调试", "直播控制"]);
  assert.equal(discoveryStatusLabel("conflicted"), "身份冲突");
});
