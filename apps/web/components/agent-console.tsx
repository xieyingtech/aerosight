"use client";

import { useEffect, useRef, useState } from "react";
import { ArrowUp, AudioLines, Bot, Check, History, Loader2, MessageSquare, PhoneOff, Plus, Sparkles } from "lucide-react";
import { apiJSON, apiFetch, APIError } from "@/lib/api-client";
import { readChatStream } from "@/lib/agent-chat-stream";
import { AgentRealtimeCall, mergeRealtimeMessage, realtimeErrorMessage } from "@/lib/agent-realtime";
import { Button } from "@/components/ui/button";
import { AgentQueryEvidence } from "@/components/agent-query-evidence";
import { ChatMarkdown } from "@/components/chat-markdown";
import { SiteHeaderActions } from "@/components/site-header";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import type { AgentSessionView } from "@/lib/web-api-types";

const suggestions = [
  { title: "项目态势", prompt: "帮我总结当前项目的整体态势，有哪些值得关注的情况？" },
  { title: "设备巡查", prompt: "查看当前项目的设备状态，哪些设备需要关注？" },
  { title: "任务进展", prompt: "当前项目的任务进展如何？有哪些异常或未完成的任务？" },
  { title: "案件梳理", prompt: "梳理当前项目的案件，按优先级总结需要处理的问题。" },
];
const titleOf = (session: AgentSessionView) => session.messages.find(message => message.role === "user")?.content || session.summary || "新对话";
function errorMessage(error: unknown) {
  if (!(error instanceof APIError)) return "连接失败，请检查网络后重试。";
  if (error.code.startsWith("AI_PROVIDER_")) return "AI 服务尚未就绪，请联系管理员检查 AI 服务配置后继续。";
  if (error.code === "AI_REQUEST_TIMEOUT") return "回复超时，请稍后继续提问。";
  if (error.code.startsWith("AI_UPSTREAM_")) return "AI 服务暂时无法回复，请稍后继续提问。";
  if (error.code === "AGENT_TOOL_STEP_LIMIT") return "本次查询已达到执行步数上限，可根据已有结果继续追问。";
  if (error.code.startsWith("AGENT_TOOL_")) return "本次工具查询未完成，请查看调用状态后重试。";
  if (error.status === 403) return "你没有在此项目中使用智能体的权限。";
  return "消息未能完成，请稍后重试。";
}

export function AgentConsole({ projectId, sessions: initialSessions }: { projectId: number; sessions: AgentSessionView[] }) {
  const [sessions, setSessions] = useState(initialSessions);
  const [activeId, setActiveId] = useState<number | null>(initialSessions[0]?.id ?? null);
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [pending, setPending] = useState<string | null>(null);
  const [liveSteps, setLiveSteps] = useState<Array<{ content: string; tools: Array<Record<string, unknown>> }>>([]);
  const [progress, setProgress] = useState("");
  const [voice, setVoice] = useState<"off" | "connecting" | "connected">("off");
  const [inputStatus, setInputStatus] = useState("");
  const [voiceStatus, setVoiceStatus] = useState("");
  const voiceCall = useRef<AgentRealtimeCall | null>(null);
  const mounted = useRef(true);
  const controller = useRef<AbortController | null>(null);
  useEffect(() => () => controller.current?.abort(), []);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; voiceCall.current?.dispose(); }; }, []);
  const [historyOpen, setHistoryOpen] = useState(false);
  const lock = useRef(false);
  const end = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLTextAreaElement>(null);
  const session = sessions.find(item => item.id === activeId);
  const messages = (session?.messages ?? []).filter(message => (voice === "connected" && message.role === "user") || message.content || (Array.isArray(message.toolCalls) && message.toolCalls.length));
  const occupied = busy || voice !== "off";
  const voiceButton = !draft.trim();
  // One user message starts a turn; its assistant steps share one identity.
  const messageGroups: Array<AgentSessionView["messages"]> = [];
  for (const message of messages) {
    const previous = messageGroups[messageGroups.length - 1];
    if (message.role === "assistant" && previous?.[0].role === "assistant") previous.push(message);
    else messageGroups.push([message]);
  }
  const base = `/api/projects/${projectId}/agent-sessions`;

  useEffect(() => {
    if (messages.length || pending) end.current?.scrollIntoView({ block: "nearest", behavior: busy ? "instant" : "smooth" });
    else end.current?.parentElement?.scrollTo({ top: 0 });
  }, [activeId, messages.length, session?.messages, pending, busy, liveSteps]);

  async function startVoice() {
    if (lock.current || draft.trim()) return;
    lock.current = true;
    setInputStatus(""); setVoice("connecting"); setVoiceStatus("正在连接实时语音…"); setError(null);
    let id = activeId;
    const call = new AgentRealtimeCall({
      message: message => {
        if (!mounted.current) return;
        setSessions(previous => previous.map(item => item.id === id ? { ...item, messages: mergeRealtimeMessage(item.messages, message) } : item));
      },
      status: status => { if (mounted.current) setVoiceStatus(status); },
      inputStatus: (status) => { if (mounted.current) setInputStatus(status); },
      ready: () => { if (mounted.current) setVoice("connected"); },
      ended: failure => {
        if (!mounted.current) return;
        voiceCall.current = null; lock.current = false; setVoice("off");
        setVoiceStatus("通话已结束，可以继续打字交流。");
        if (failure) setError(realtimeErrorMessage(failure));
        requestAnimationFrame(() => input.current?.focus());
      },
    });
    voiceCall.current = call;
    try {
      await call.prepare();
      if (!mounted.current || voiceCall.current !== call) { call.dispose(); return; }
      if (id === null) {
        const created = await apiJSON<{ id: number }>(base, { method: "POST" });
        id = created.id;
        if (!mounted.current || voiceCall.current !== call) { call.dispose(); return; }
        setSessions(previous => [{ id: created.id, status: "open", summary: null, createdAt: new Date().toISOString(), messages: [] }, ...previous]);
        setActiveId(id);
      }
      call.connect(`${base}/${id}/realtime`);
    } catch (failure) {
      call.dispose();
      if (!mounted.current || voiceCall.current !== call) return;
      voiceCall.current = null; lock.current = false; setVoice("off"); setVoiceStatus("");
      setError(realtimeErrorMessage(failure));
    }
  }

  function select(id: number | null) {
    if (lock.current) return;
    setActiveId(id); setDraft(""); setError(null); setVoiceStatus(""); setHistoryOpen(false);
    input.current?.focus();
  }

  async function send() {
    const content = draft.trim();
    if (!content || lock.current || (session && session.status !== "open")) return;
    lock.current = true; setBusy(true); setError(null); setPending(content); setDraft("");
    setLiveSteps([]); setProgress("正在发送问题…");
    const abort = new AbortController(); controller.current = abort;
    let id = activeId;
    let sent = false;
    try {
      if (id === null) {
        const created = await apiJSON<{ id: number }>(base, { method: "POST", signal: abort.signal });
        id = created.id;
        setSessions(previous => [{ id: created.id, status: "open", summary: null, createdAt: new Date().toISOString(), messages: [] }, ...previous]);
        setActiveId(id);
      }
      const response = await apiFetch(`${base}/${id}/messages`, { method: "POST", signal: abort.signal, headers: { "content-type": "application/json", Accept: "application/x-ndjson" }, body: JSON.stringify({ content }) });
      if (!response.ok) {
        const body = await response.json().catch(() => ({}));
        throw new APIError(response.status, body.error || `HTTP_${response.status}`);
      }
      await readChatStream(response, event => {
        if (event.type === "error") throw new APIError(400, String(event.data.code));
        if (event.type === "status") setProgress(String(event.data.message));
        if (event.type === "step") setLiveSteps(previous => [...previous, { content: "", tools: [] }]);
        if (event.type === "text") {
          setProgress("正在生成回复…");
          setLiveSteps(previous => previous.map((step, index) => index === previous.length - 1 ? { ...step, content: step.content + String(event.data.delta ?? "") } : step));
        }
        if (event.type === "tool") {
          setProgress(event.data.status === "running" ? "正在查询项目数据…" : "正在整理查询结果…");
          setLiveSteps(previous => previous.map((step, index) => {
            if (index !== previous.length - 1) return step;
            const found = step.tools.some(tool => tool.id === event.data.id);
            return { ...step, tools: found ? step.tools.map(tool => tool.id === event.data.id ? event.data : tool) : [...step.tools, event.data] };
          }));
        }
      });
      sent = true;
    } catch (error) {
      setError(abort.signal.aborted ? "已停止生成，已完成的查询保留在会话中。" : errorMessage(error));
      if (id === null) setDraft(content);
    } finally {
      // A failed AI reply may still have persisted the user's message.
      if (id !== null) {
        try {
          const refreshed = await apiJSON<AgentSessionView[]>(base);
          setSessions(refreshed);
          setLiveSteps([]);
          const persisted = refreshed.find(item => item.id === id)?.messages.some(message =>
            message.role === "user" && message.content === content && !messages.some(previous => previous.id === message.id));
          if (!sent && !persisted) setDraft(content);
        }
        catch { setError("暂时无法同步会话记录，请刷新页面确认消息状态后继续。"); }
      }
      if (!sent && id === null) setDraft(content);
      controller.current = null;
      setPending(null); setBusy(false); lock.current = false;
      requestAnimationFrame(() => input.current?.focus());
    }
  }

  return <section aria-label="项目智能体聊天" className="flex h-[calc(100dvh-6rem)] min-h-0 flex-col">
    <h1 className="sr-only">项目智能体</h1>
    <SiteHeaderActions>
      <Button size="icon" variant="ghost" title="新对话" aria-label="新对话" onClick={() => select(null)} disabled={occupied}><Plus className="size-5" /></Button>
      <DropdownMenu open={historyOpen} onOpenChange={setHistoryOpen}>
        <DropdownMenuTrigger asChild><Button size="icon" variant="ghost" title="历史会话" aria-label="历史会话" disabled={occupied}><History className="size-5" /></Button></DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-80 max-w-[calc(100vw-2rem)]" onCloseAutoFocus={event => { event.preventDefault(); input.current?.focus(); }}>
          <DropdownMenuLabel>历史会话</DropdownMenuLabel>
          <DropdownMenuSeparator />
          <div className="max-h-80 overflow-y-auto">
            {sessions.map(item => <DropdownMenuItem key={item.id} onSelect={() => select(item.id)} className="gap-3 px-3 py-3" aria-current={activeId === item.id ? "true" : undefined}>
              <MessageSquare className="size-4 shrink-0 text-muted-foreground" />
              <span className="min-w-0 flex-1"><span className="block truncate">{titleOf(item)}</span><span className="mt-1 block text-xs text-muted-foreground">{new Date(item.createdAt).toLocaleDateString("zh-CN", { month: "short", day: "numeric" })}</span></span>
              {activeId === item.id && <Check className="size-4 shrink-0 text-primary" />}
            </DropdownMenuItem>)}
            {!sessions.length && <p className="px-3 py-6 text-sm text-muted-foreground">发送消息后，对话会自动保存在这里。</p>}
          </div>
          <DropdownMenuSeparator />
          <p className="px-3 py-2 text-xs text-muted-foreground">仅显示你在当前项目的对话</p>
        </DropdownMenuContent>
      </DropdownMenu>
    </SiteHeaderActions>
      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-6 sm:px-8" role="log" aria-label="对话消息" aria-live="polite">
        {!messages.length && !pending && voice === "off" ? <div className="mx-auto flex min-h-full max-w-2xl flex-col justify-center py-1">
          <div className="mb-3 flex size-10 items-center justify-center rounded-2xl bg-primary/10 text-primary"><Sparkles className="size-5" /></div>
          <p className="mb-2 text-2xl font-semibold tracking-tight">今天想了解项目的什么？</p>
          <p className="max-w-lg text-sm leading-6 text-muted-foreground">和我聊聊设备、任务或案件。我会查询当前项目的数据，帮你梳理情况，并提供可追溯的依据。</p>
          <div className="mt-4 grid gap-2 sm:grid-cols-2">{suggestions.map(item => <button key={item.title} onClick={() => { setDraft(item.prompt); input.current?.focus(); }} className="rounded-lg p-3 text-left transition-colors hover:bg-muted/50"><span className="text-sm font-medium">{item.title}<span className="float-right text-muted-foreground">↗</span></span><span className="mt-1 block text-xs leading-5 text-muted-foreground">{item.prompt}</span></button>)}</div>
        </div> : <div className="mx-auto max-w-3xl space-y-6">
          {messageGroups.map(group => <article key={group[0].id} className={`flex gap-3 ${group[0].role === "user" ? "justify-end" : ""}`}>
            {group[0].role !== "user" && <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary"><Bot className="size-4" /></div>}
            <div className={`min-w-0 max-w-[88%] ${group[0].role === "user" ? "rounded-2xl rounded-tr-sm bg-primary px-4 py-3 text-primary-foreground" : "flex-1 pt-1"}`}>
              {group[0].role !== "user" && <p className="mb-2 text-xs opacity-60">项目智能体</p>}
              <div className="space-y-3">{group.map(message => <div key={message.id}>
                {message.content && (message.role === "assistant" ? <ChatMarkdown content={message.content} /> : <p className="whitespace-pre-wrap break-words text-sm leading-7">{message.content}</p>)}
                {voice === "connected" && message.role === "user" && !message.content && <p className="text-sm opacity-70">正在聆听并转写…</p>}
                <AgentQueryEvidence toolCalls={message.toolCalls} inline />
              </div>)}</div>
            </div>
          </article>)}
          {pending && <div className="flex justify-end"><p className="max-w-[88%] whitespace-pre-wrap break-words rounded-2xl rounded-tr-sm bg-primary px-4 py-3 text-sm leading-7 text-primary-foreground">{pending}</p></div>}
          {liveSteps.length > 0 && <article className="flex gap-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary"><Bot className="size-4" /></div>
            <div className="min-w-0 flex-1 pt-1">
              <p className="mb-2 text-xs text-muted-foreground">项目智能体</p>
              <div className="space-y-3">{liveSteps.map((step, index) => <div key={index}>
                {step.content && <ChatMarkdown content={step.content} />}
                <AgentQueryEvidence toolCalls={step.tools} inline />
              </div>)}</div>
            </div>
          </article>}
          {busy && <div role="status" className="flex items-center gap-3 text-sm text-muted-foreground"><Loader2 className="size-4 animate-spin" />{progress}<Button type="button" size="sm" variant="ghost" onClick={() => controller.current?.abort()}>停止生成</Button></div>}
        </div>}
        <div ref={end} />
      </div>
      <div className="shrink-0 px-4 pb-4 sm:px-8">
        <div className="mx-auto max-w-3xl">
          {error && <p role="alert" className="mb-3 rounded-lg bg-destructive/10 px-4 py-3 text-sm text-destructive">{error}</p>}
          {session && session.status !== "open" ? <p className="mb-3 text-sm text-muted-foreground">此对话已结束，可以新建对话继续交流。</p> : null}
          <form onSubmit={event => { event.preventDefault(); void send(); }} className="rounded-2xl border bg-muted/15 p-3 shadow-sm focus-within:border-primary/50 focus-within:ring-2 focus-within:ring-primary/10">
            {voice !== "off" ? <div className="flex min-h-16 items-center gap-3 px-1" role="status">
              {voice === "connecting" ? <Loader2 className="size-5 animate-spin text-primary" /> : <AudioLines className="size-5 animate-pulse text-primary" />}
              <div><p className="text-sm font-medium">{voice === "connecting" ? "正在接通实时对话" : "实时语音对话中"}</p><p className="mt-1 text-xs text-muted-foreground">{voiceStatus}</p>{voice === "connected" && <p role="status" className="mt-1 text-xs text-muted-foreground">{inputStatus}</p>}</div>
            </div> : <textarea ref={input} aria-label="发送给项目智能体" placeholder="询问项目情况，或继续追问…" value={draft} onChange={event => setDraft(event.target.value)} disabled={busy || Boolean(session && session.status !== "open")} rows={2} className="max-h-40 min-h-16 w-full resize-none bg-transparent px-1 py-1 text-sm leading-6 outline-none placeholder:text-muted-foreground disabled:opacity-60" onKeyDown={event => { if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing && event.keyCode !== 229) { event.preventDefault(); void send(); } }} />}
            <div className="flex items-center justify-between gap-2"><span className="px-1 text-[11px] text-muted-foreground">{voice !== "off" ? "可以随时说话打断 · 转写与查询显示在对话中" : voiceButton ? "输入文字，或点击声波开始实时对话" : "Enter 发送 · Shift + Enter 换行"}</span>
              {voice !== "off" ? <Button type="button" size="icon" variant="destructive" className="size-8 shrink-0 rounded-lg" aria-label="结束实时对话" title="结束实时对话" onClick={() => voiceCall.current?.stop()}><PhoneOff className="size-4" /></Button>
                : voiceButton ? <Button type="button" size="icon" className="size-8 shrink-0 rounded-lg" aria-label="开始实时语音对话" title="开始实时语音对话" disabled={busy || Boolean(session && session.status !== "open")} onClick={() => void startVoice()}><AudioLines className="size-4" /></Button>
                : <Button type="submit" size="icon" className="size-8 shrink-0 rounded-lg" aria-label="发送消息" disabled={busy || !draft.trim() || Boolean(session && session.status !== "open")}>{busy ? <Loader2 className="size-4 animate-spin" /> : <ArrowUp className="size-4" />}</Button>}
            </div>
          </form>
          {voice === "off" && voiceStatus && <p role="status" className="mt-2 text-center text-xs text-muted-foreground">{voiceStatus}</p>}
          <p className="mt-2 text-center text-[11px] text-muted-foreground">回答基于当前项目可访问的数据，重要结论请结合证据核实。</p>
        </div>
      </div>
  </section>;
}
