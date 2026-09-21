import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import { runInNewContext } from "node:vm";
import { AgentRealtimeCall, mergeRealtimeMessage, realtimeErrorMessage } from "./agent-realtime.ts";
import { APIError } from "./api-client.ts";

test("late transcription updates the original turn without duplication", () => {
  const assistant = { id: 2, role: "assistant", content: "查询中", toolCalls: [], createdAt: "" };
  const user = { ...assistant, id: 1, role: "user", content: "查设备" };
  let messages = mergeRealtimeMessage([assistant], user);
  messages = mergeRealtimeMessage(messages, { ...assistant, content: "查询完成" });
  assert.deepEqual(messages.map(m => [m.role, m.content]), [["user", "查设备"], ["assistant", "查询完成"]]);
  assert.match(realtimeErrorMessage(new APIError(400, "AI_REALTIME_CONNECT_FAILED")), /StepFun/);
  assert.match(realtimeErrorMessage(new DOMException("Denied", "NotAllowedError")), /权限/);
});

test("audio worklet resamples 48 kHz to 24 kHz PCM without forwarding device-rate audio", () => {
  let Processor: new () => { process: (inputs: Float32Array[][]) => boolean };
  const packets: ArrayBuffer[] = [];
  runInNewContext(readFileSync(new URL("../public/audio/agent-recorder.js", import.meta.url), "utf8"), {
    sampleRate: 48000, Int16Array,
    AudioWorkletProcessor: class { port = { postMessage: (data: ArrayBuffer) => packets.push(data) }; },
    registerProcessor: (_name: string, klass: typeof Processor) => { Processor = klass; },
  });
  const processor = new Processor!();
  processor.process([[new Float32Array(4800).fill(0.5)]]);
  assert.equal(packets.length, 1);
  assert.equal(packets[0].byteLength, 4800);
  assert.equal(new Int16Array(packets[0])[0], 16384);
});

test("hangup during permission prompt releases tracks granted later", async () => {
  const descriptors = ["window", "navigator", "AudioContext"].map(key => [key, Object.getOwnPropertyDescriptor(globalThis, key)] as const);
  let grant!: (media: unknown) => void;
  let stopped = 0, ended = 0, contextClosed = 0;
  const permission = new Promise(resolve => { grant = resolve; });
  class Context { resume() { return Promise.resolve(); } close() { contextClosed++; return Promise.resolve(); } }
  Object.defineProperty(globalThis, "window", { configurable: true, value: { isSecureContext: true, AudioContext: Context, AudioWorkletNode: class {} } });
  Object.defineProperty(globalThis, "AudioContext", { configurable: true, value: Context });
  Object.defineProperty(globalThis, "navigator", { configurable: true, value: { mediaDevices: { getUserMedia: () => permission } } });
  try {
    const call = new AgentRealtimeCall({ message() {}, status() {}, ready() {}, ended() { ended++; } });
    const preparing = call.prepare();
    call.stop();
    grant({ getTracks: () => [{ stop() { stopped++; } }] });
    await assert.rejects(preparing, { name: "AbortError" });
    assert.equal(stopped, 1);
    assert.equal(ended, 1);
    assert.equal(contextClosed, 1);
  } finally {
    for (const [key, descriptor] of descriptors) {
      if (descriptor) Object.defineProperty(globalThis, key, descriptor);
      else Reflect.deleteProperty(globalThis, key);
    }
  }
});
