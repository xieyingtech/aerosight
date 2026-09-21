import { test } from "node:test";
import assert from "node:assert/strict";
import { readChatStream } from "./agent-chat-stream.ts";

function response(text: string) {
  const bytes = new TextEncoder().encode(text);
  return new Response(new ReadableStream({start(controller) {
    for (const byte of bytes) controller.enqueue(new Uint8Array([byte]));
    controller.close();
  }}), {headers: {"content-type": "application/x-ndjson"}});
}
test("chat stream decodes split UTF-8 and delivers events in order", async () => {
  const events: string[] = [];
  await readChatStream(response('{"type":"text","data":{"delta":"查询设备"}}\n{"type":"done","data":{}}'), event => events.push(String(event.data.delta ?? event.type)));
  assert.deepEqual(events, ["查询设备", "done"]);
});
test("chat stream rejects EOF without terminal event", async () => {
  await assert.rejects(readChatStream(response('{"type":"text","data":{"delta":"部分回复"}}\n'), () => {}), /CHAT_STREAM_INTERRUPTED/);
});
test("chat stream propagates server errors and rejects malformed frames", async () => {
  await assert.rejects(readChatStream(response('{"type":"error","data":{"code":"AI_REQUEST_TIMEOUT"}}\n'), event => { throw new Error(String(event.data.code)); }), /AI_REQUEST_TIMEOUT/);
  await assert.rejects(readChatStream(response('{bad}\n'), () => {}), SyntaxError);
});
