import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const component = readFileSync(new URL("../components/dji-flighthub-wizard.tsx", import.meta.url), "utf8");

test("connector management renders capability matrix, evidence, watermarks, and read-only reprobe", () => {
  for (const label of ["能力与同步诊断", "验证证据", "型号 / 固件", "成功水位", "只读重新探测", "只会调用官方 GET 接口"]) {
    assert.match(component, new RegExp(label));
  }
  assert.match(component, /connectorDiagnosticHealth/);
  assert.match(component, /capabilityStatusLabels/);
  assert.match(component, /evidenceLevelLabels/);
});
