import { APIError } from "./api-client.ts";
import type { AgentSessionView } from "./web-api-types";

export type RealtimeMessage = AgentSessionView["messages"][number];
type RealtimeCallbacks = {
  message: (message: RealtimeMessage) => void;
  status: (status: string) => void;
  ready: () => void;
  ended: (error?: unknown) => void;
};
export function mergeRealtimeMessage(messages: RealtimeMessage[], message: RealtimeMessage) {
  const found = messages.some(item => item.id === message.id);
  return (found ? messages.map(item => item.id === message.id ? message : item) : [...messages, message]).sort((a, b) => a.id - b.id);
}

export function realtimeErrorMessage(error: unknown): string {
  if (error instanceof DOMException) {
    if (error.name === "NotAllowedError") return "未获得语音权限。请允许浏览器使用麦克风后重试。";
    if (error.name === "NotFoundError") return "没有找到可用的音频输入设备。";
    if (error.name === "NotReadableError") return "音频输入设备正被占用，请关闭其他录音应用后重试。";
  }
  const code = error instanceof APIError ? error.code : error instanceof Error ? error.message : "";
  const messages: Record<string, string> = {
    AI_REALTIME_UNSUPPORTED: "当前浏览器不支持实时语音，请使用支持音频输入的浏览器通过 HTTPS 或 localhost 打开。",
    AI_REALTIME_PROVIDER_REQUIRED: "请在默认 AI Provider 中选择实时语音协议并填写实时模型，再开始对话。",
    AI_REALTIME_CONNECT_FAILED: "未能连接实时语音，请检查 Provider 地址、模型、API 密钥及服务权限。",
    AI_REALTIME_UPSTREAM_FAILED: "实时语音服务返回错误，请稍后重试。",
    AI_REALTIME_DISCONNECTED: "实时语音连接已中断，已完成的对话仍会保留。",
    AI_REALTIME_TIMEOUT: "本次实时通话已结束，请新建对话继续。",
    AI_REALTIME_ALREADY_CONNECTED: "已有一通实时对话，请先结束另一个窗口中的通话。",
    AI_REALTIME_AUDIO_BLOCKED: "浏览器未能播放语音，请检查音频权限后重新连接。",
    AI_REALTIME_AUDIO_BACKLOG: "网络或音频播放延迟过大，通话已结束，请重新连接。",
    PROJECT_ACCESS_DENIED: "你没有在此项目中使用智能体的权限。",
    AGENT_TOOL_STEP_LIMIT: "本轮查询已达到工具调用上限，请新建对话继续。",
  };
  return messages[code] ?? "实时语音连接失败，请检查网络和音频设备后重试。";
}

// Audio and transport only. Messages are supplied to AgentConsole's existing
// message renderer, with exactly the same IDs and DTO as persisted text chat.
export class AgentRealtimeCall {
  private context: AudioContext | null = null;
  private media: MediaStream | null = null;
  private recorder: AudioWorkletNode | null = null;
  private socket: WebSocket | null = null;
  private closed = false;
  private stopping = false;
  private ready = false;
  private nextTime = 0;
  private offsets = new Map<string, number>();
  private interruptedItems = new Set<string>();
  private currentAudioId: string | null = null;
  private playing: Array<{ source: AudioBufferSourceNode; id: string; start: number; duration: number; offset: number }> = [];
  private timer: ReturnType<typeof setTimeout> | null = null;
  private finished = false;

  private callbacks: RealtimeCallbacks;
  constructor(callbacks: RealtimeCallbacks) { this.callbacks = callbacks; }

  // Called directly from the voice button so audio permission and playback
  // initialization belong to a user gesture. A late permission grant after
  // hangup must immediately release its tracks.
  async prepare() {
    if (!window.isSecureContext || !navigator.mediaDevices?.getUserMedia || !window.AudioContext || !window.AudioWorkletNode) throw new Error("AI_REALTIME_UNSUPPORTED");
    this.callbacks.status("请允许浏览器使用音频输入设备…");
    this.timer = setTimeout(() => this.finish(new DOMException("Permission timed out", "NotAllowedError")), 30000);
    this.context = new AudioContext({ sampleRate: 24000 });
    const resumed = this.context.resume();
    // Request permission in the same user gesture, even if the embedded
    // browser defers AudioContext.resume until it has audio-input permission.
    const media = await navigator.mediaDevices.getUserMedia({ audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true, autoGainControl: true } });
    if (this.closed) { media.getTracks().forEach(track => track.stop()); throw new DOMException("Cancelled", "AbortError"); }
    this.media = media;
    await resumed;
    if (this.closed) throw new DOMException("Cancelled", "AbortError");
    this.callbacks.status("音频设备已就绪，正在连接实时语音服务…");
    await this.context.audioWorklet.addModule("/audio/agent-recorder.js");
    if (this.closed) throw new DOMException("Cancelled", "AbortError");
    const source = this.context.createMediaStreamSource(media);
    this.recorder = new AudioWorkletNode(this.context, "agent-recorder");
    // The worklet emits silence to its output; input is sent only after ready.
    source.connect(this.recorder).connect(this.context.destination);
    this.recorder.port.onmessage = ({ data }: MessageEvent<ArrayBuffer>) => {
      if (!this.ready || this.stopping || this.socket?.readyState !== WebSocket.OPEN) return;
      if (this.socket.bufferedAmount > 240000) { this.finish(new Error("AI_REALTIME_AUDIO_BACKLOG")); return; }
      this.socket.send(data);
    };
    media.getTracks().forEach(track => { track.onended = () => { if (!this.closed) this.finish(new Error("AI_REALTIME_DISCONNECTED")); }; });
  }

  connect(path: string) {
    if (this.closed) return;
    const url = new URL(path, window.location.origin);
    if (url.origin !== window.location.origin || !url.pathname.startsWith("/api/")) throw new Error("API_PATH_MUST_BE_SAME_ORIGIN");
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    const socket = new WebSocket(url);
    this.socket = socket;
    this.armTimeout(25000);
    socket.onmessage = ({ data }) => {
      if (this.closed) return;
      try {
        const event = JSON.parse(String(data)) as { type: string; data: Record<string, unknown> };
        if (!this.stopping) this.armTimeout(45000);
        switch (event.type) {
          case "ready": this.ready = true; this.callbacks.ready(); this.callbacks.status("已接通，直接说话即可"); break;
          case "message": this.callbacks.message(event.data as RealtimeMessage); break;
          case "status": if (!this.stopping) this.callbacks.status(String(event.data.message)); break;
          case "audio": if (!this.stopping) this.play(String(event.data.itemId), String(event.data.delta)); break;
          case "interrupted": {
            const played = this.interrupt();
            if (played) socket.send(JSON.stringify({ type: "truncate", ...played }));
            this.callbacks.status("正在聆听…");
            break;
          }
          case "error": this.finish(new APIError(400, String(event.data.code))); break;
          case "done": this.finish(); break;
        }
      } catch (error) { this.finish(error); }
    };
    socket.onerror = () => this.finish(new Error("AI_REALTIME_CONNECT_FAILED"));
    socket.onclose = () => { if (!this.closed) this.finish(new Error("AI_REALTIME_DISCONNECTED")); };
  }

  private armTimeout(ms: number) {
    if (this.timer) clearTimeout(this.timer);
    this.timer = setTimeout(() => this.finish(new Error("AI_REALTIME_DISCONNECTED")), ms);
  }

  private play(id: string, encoded: string) {
    const ctx = this.context;
    if (!ctx || !encoded || this.interruptedItems.has(id)) return;
    this.currentAudioId = id;
    if (ctx.state !== "running") throw new Error("AI_REALTIME_AUDIO_BLOCKED");
    const bytes = Uint8Array.from(atob(encoded), char => char.charCodeAt(0));
    if (!bytes.length || bytes.length % 2) return;
    const view = new DataView(bytes.buffer);
    const buffer = ctx.createBuffer(1, bytes.length / 2, 24000);
    const channel = buffer.getChannelData(0);
    for (let i = 0; i < channel.length; i++) channel[i] = view.getInt16(i * 2, true) / 32768;
    const start = Math.max(ctx.currentTime + 0.02, this.nextTime);
    if (start - ctx.currentTime > 60) throw new Error("AI_REALTIME_AUDIO_BACKLOG");
    const source = ctx.createBufferSource();
    source.buffer = buffer;
    source.connect(ctx.destination);
    const offset = this.offsets.get(id) ?? 0;
    this.offsets.set(id, offset + buffer.duration);
    const entry = { source, id, start, duration: buffer.duration, offset };
    this.playing.push(entry);
    source.onended = () => {
      this.playing = this.playing.filter(item => item !== entry);
      source.disconnect();
    };
    source.start(start);
    this.nextTime = start + buffer.duration;
  }

  private interrupt() {
    if (this.currentAudioId) this.interruptedItems.add(this.currentAudioId);
    this.currentAudioId = null;
    const now = this.context?.currentTime ?? 0;
    const active = this.playing.find(item => item.start <= now && item.start + item.duration > now) ?? this.playing[0];
    const position = active ? { itemId: active.id, audioEndMs: Math.floor((active.offset + Math.max(0, Math.min(active.duration, now - active.start))) * 1000) } : null;
    for (const item of this.playing) { item.source.onended = null; item.source.stop(); item.source.disconnect(); }
    this.playing = [];
    this.nextTime = 0;
    return position;
  }

  stop() {
    if (this.closed || this.stopping) return;
    this.stopping = true;
    this.media?.getTracks().forEach(track => { track.onended = null; track.stop(); });
    this.interrupt();
    if (this.socket?.readyState === WebSocket.OPEN && this.ready) {
      this.callbacks.status("正在结束通话并保存记录…");
      this.socket.send(JSON.stringify({ type: "stop" }));
      this.armTimeout(8000);
    } else this.finish();
  }

  private finish(error?: unknown) {
    if (this.finished) return;
    this.finished = true;
    this.dispose();
    this.callbacks.ended(error);
  }

  dispose() {
    this.closed = true;
    if (this.timer) clearTimeout(this.timer);
    this.media?.getTracks().forEach(track => { track.onended = null; track.stop(); });
    this.recorder?.disconnect();
    if (this.recorder) this.recorder.port.onmessage = null;
    this.interrupt();
    this.socket?.close();
    void this.context?.close().catch(() => {});
  }
}
