import assert from "node:assert/strict";
import test from "node:test";

import { diagnosticPresentation, type OperationDiagnostic } from "./operation-diagnostics-core.ts";

const diagnostic = (status: string, kind: OperationDiagnostic["kind"], reason: string): OperationDiagnostic => ({
  id: `${kind}:${status}`, kind, status, reason, severity: status === "degraded" ? "warning" : "error",
  title: "fixture", occurredAt: "2026-08-27T00:00:00Z"
});

test("timeout, NACK, offline and missing media stay explicit and actionable", () => {
  for (const fixture of [
    diagnostic("timed_out", "command", "DJI_REPLY_TIMEOUT"),
    diagnostic("nacked", "command", "DJI_RESULT_326108"),
    diagnostic("failed", "connection", "DJI_MQTT_CONNECTION_LOST"),
    diagnostic("degraded", "stream", "MEDIAMTX_INPUT_MISSING")
  ]) assert.equal(diagnosticPresentation(fixture).actionable, true);
});

test("an error can never be presented as physical success", () => {
  assert.throws(() => diagnosticPresentation(diagnostic("acknowledged", "command", "late reply")), /FALSE_SUCCESS/);
  assert.throws(() => diagnosticPresentation(diagnostic("live", "stream", "no tracks")), /FALSE_SUCCESS/);
});
