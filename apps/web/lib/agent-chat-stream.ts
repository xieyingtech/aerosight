export type ChatEvent = { type: string; data: Record<string, unknown> };

// NDJSON frames may cross arbitrary UTF-8/network chunk boundaries.
export async function readChatStream(response: Response, onEvent: (event: ChatEvent) => void) {
  if (!response.body || !response.headers.get("content-type")?.includes("application/x-ndjson")) throw new Error("CHAT_STREAM_INVALID");
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let terminal = false;
  const consume = (line: string) => {
    if (!line.trim()) return;
    const event = JSON.parse(line) as ChatEvent;
    if (!event || typeof event.type !== "string" || !event.data || typeof event.data !== "object") throw new Error("CHAT_STREAM_INVALID");
    if (terminal) throw new Error("CHAT_STREAM_INVALID");
    onEvent(event);
    terminal = event.type === "done" || event.type === "error";
  };
  try {
    while (true) {
      const { done, value } = await reader.read();
      buffer += decoder.decode(value, { stream: !done });
      let newline: number;
      while ((newline = buffer.indexOf("\n")) >= 0) {
        consume(buffer.slice(0, newline));
        buffer = buffer.slice(newline + 1);
      }
      if (buffer.length > 1_000_000) throw new Error("CHAT_STREAM_INVALID");
      if (done) break;
    }
    consume(buffer);
    if (!terminal) throw new Error("CHAT_STREAM_INTERRUPTED");
  } finally { await reader.cancel().catch(() => {}); reader.releaseLock(); }
}
