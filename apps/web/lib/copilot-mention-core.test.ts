import assert from "node:assert/strict";
import test from "node:test";
import { hasActionableCopilotMention, shouldQueueCopilotMention } from "./copilot-mention-core.ts";

test("plain issue comment creates an actionable copilot mention", () => {
  assert.equal(hasActionableCopilotMention("@copilot 请根据原始证据解释这个案件"), true);
  assert.equal(hasActionableCopilotMention("麻烦 @Copilot 复核"), true);
});

test("quotes code and lookalike identifiers do not mention copilot", () => {
  for (const comment of [
    "> @copilot 上一次的回复", "`@copilot`", "```md\n@copilot\n```", "~~~\n@copilot\n~~~",
    "operator@copilot", "@copilot-bot", "copilot"
  ]) assert.equal(hasActionableCopilotMention(comment), false, comment);
});

test("quoted mention does not hide a later explicit mention", () => {
  assert.equal(hasActionableCopilotMention("> @copilot 旧内容\n\n@copilot 请重新分析"), true);
});

test("mention without current agent permission remains an ordinary comment", () => {
  assert.equal(shouldQueueCopilotMention("@copilot 请分析", new Set(["issue:handle"])), false);
  assert.equal(shouldQueueCopilotMention("@copilot 请分析", new Set(["issue:handle", "agent:use"])), true);
});
