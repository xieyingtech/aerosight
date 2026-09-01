const copilotMention = /(^|[^\p{L}\p{N}_])@copilot(?![\p{L}\p{N}_-])/iu;

function removeInlineCode(line: string) {
  let result = "";
  let inCode = false;
  for (let index = 0; index < line.length; index += 1) {
    if (line[index] === "`" && line[index - 1] !== "\\") {
      inCode = !inCode;
      continue;
    }
    if (!inCode) result += line[index];
  }
  return result;
}

export function hasActionableCopilotMention(markdown: string) {
  let fence: "`" | "~" | null = null;
  const visible: string[] = [];
  for (const line of markdown.split(/\r?\n/)) {
    const trimmed = line.trimStart();
    const opening = trimmed.startsWith("```") ? "`" : trimmed.startsWith("~~~") ? "~" : null;
    if (opening) {
      if (fence === opening) fence = null;
      else if (fence === null) fence = opening;
      continue;
    }
    if (fence || trimmed.startsWith(">")) continue;
    visible.push(removeInlineCode(line));
  }
  return copilotMention.test(visible.join("\n"));
}

export function shouldQueueCopilotMention(markdown: string, permissions: ReadonlySet<string>) {
  return permissions.has("agent:use") && hasActionableCopilotMention(markdown);
}

export function copilotJobIdempotencyKey(triggerType: "issue_mention" | "issue_assignment", activityId: number) {
  if (!Number.isSafeInteger(activityId) || activityId <= 0) throw new Error("COPILOT_TRIGGER_ACTIVITY_INVALID");
  return `${triggerType}:${activityId}:copilot`;
}
