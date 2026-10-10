"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { ArrowUp, AudioLines, Bot, Check, History, Loader2, MessageSquare, PhoneOff, Plus, Sparkles, PanelsTopLeft } from "lucide-react";
import { apiJSON, apiFetch, APIError } from "@/lib/api-client";
import { readChatStream } from "@/lib/agent-chat-stream";
import { AgentRealtimeCall, mergeRealtimeMessage, realtimeErrorMessage } from "@/lib/agent-realtime";
import { Button } from "@/components/ui/button";
import { AgentQueryEvidence, agentToolLabel } from "@/components/agent-query-evidence";
import { ChatMarkdown } from "@/components/chat-markdown";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import type { AgentSessionView } from "@/lib/web-api-types";
import { useAgentWorkspace } from "@/components/agent-workspace-context";
import { missionLiveHref } from "@/lib/mission-live-core";
import { agentFlightRunId } from "@/lib/agent-floating-flight";

type AgentApproval = NonNullable<AgentSessionView["approvals"]>[number];

const suggestions = [
  { title: "影像目标查询", prompt: "查询当前项目已完成识别的影像，帮我找出人和车辆，返回目标框与筛选依据。" },
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
  if (error.code.startsWith("AGENT_TOOL_")) return "本次工具调用未完成，请查看调用状态后重试。";
  if (error.code === "ISSUE_VERSION_CONFLICT") return "案件已更新，请让智能体重新查询后再发起操作。";
  if (error.code === "MCP_CONFIG_CHANGED" || error.code === "MCP_SCHEMA_CHANGED") return "MCP 配置或工具已更新，请重新发现工具并发起调用。";
  if (error.code.startsWith("MCP_")) return "MCP 调用未完成，请检查工具状态与连接配置。";
  if (error.status === 403) return "你没有在此项目中使用智能体的权限。";
  return "消息未能完成，请稍后重试。";
}

export function AgentConsole({ projectId, sessions: initialSessions }: { projectId: number; sessions: AgentSessionView[] }) {
  const { attach } = useAgentWorkspace();
  const slot = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (slot.current) return attach(projectId, initialSessions, slot.current);
  }, [attach, projectId, initialSessions]);
  return <div ref={slot} className="min-h-0 flex-1" />;
}

export function AgentConversation({ projectId, sessions: initialSessions, initiallyBlank = false, floating = false, visible = true, unavailableSessions = [], onHeaderChange, onActivity, onFloat, onFlightAccepted }: {
  projectId: number; sessions: AgentSessionView[]; initiallyBlank?: boolean; floating?: boolean; visible?: boolean; unavailableSessions?: number[];
  onActivity: (sessionId: number | null, active: boolean, title: string) => void;
  onHeaderChange: (header: ReactNode) => void;
  onFloat: () => void; onFlightAccepted: (href: string) => void;
}) {
  const [sessions, setSessions] = useState(initialSessions);
  const [activeId, setActiveId] = useState<number | null>(initiallyBlank ? null : initialSessions[0]?.id ?? null);
  const [flightRunId, setFlightRunId] = useState<number | null>(null);
  const flightNavigation = useRef(onFlightAccepted);
  flightNavigation.current = onFlightAccepted;
  useEffect(() => {
    if (!flightRunId) return;
    const abort = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const deadline = Date.now() + 5 * 60_000;
    const poll = async () => {
      try {
        const model = await apiJSON<{ run: Record<string, unknown> }>(`/api/projects/${projectId}/task-runs/${flightRunId}`, { signal: abort.signal });
        const href = missionLiveHref(projectId, model.run);
        if (href) { setFlightRunId(null); flightNavigation.current(href); return; }
        if (["failed", "canceled", "succeeded", "blocked"].includes(String(model.run.status))) {
          setFlightRunId(null);
          if (["failed", "blocked"].includes(String(model.run.status))) setError(`飞行任务未被受理：${String(model.run.stateReason || model.run.status)}。请核对任务状态后重新发起授权。`);
          return;
        }
      } catch (failure) {
        if (abort.signal.aborted) return;
        if (failure instanceof APIError && [401, 403, 404].includes(failure.status)) { setFlightRunId(null); return; }
      }
      if (!abort.signal.aborted && Date.now() < deadline) timer = setTimeout(poll, 2000);
      else if (!abort.signal.aborted) { setFlightRunId(null); setError("尚未收到飞行任务受理结果，请查看任务状态。"); }
    };
    void poll();
    return () => { abort.abort(); clearTimeout(timer); };
  }, [flightRunId, projectId]);
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [deciding, setDeciding] = useState<string | null>(null);
  const [pending, setPending] = useState<string | null>(null);
  const [liveSteps, setLiveSteps] = useState<Array<{ content: string; tools: Array<Record<string, unknown>> }>>([]);
  const [progress, setProgress] = useState("");
  const [voice, setVoice] = useState<"off" | "connecting" | "connected">("off");
  const [inputStatus, setInputStatus] = useState("");
  const [voiceStatus, setVoiceStatus] = useState("");
  const [now, setNow] = useState(() => Date.now());
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
  const approvals = session?.approvals ?? [];
  useEffect(() => {
    const nextExpiry = approvals.filter(approval => approval.status === "pending").map(approval => Date.parse(approval.expiresAt)).filter(expiry => expiry > now).sort((a, b) => a - b)[0];
    if (nextExpiry === undefined) return;
    const timer = window.setTimeout(() => setNow(Date.now()), Math.min(nextExpiry - now + 50, 60_000));
    return () => window.clearTimeout(timer);
  }, [session?.approvals, now]);
  const approvalById = new Map(approvals.map(approval => [approval.id, approval]));
  const legacyApprovals = [...approvals].sort((a, b) => Date.parse(a.createdAt) - Date.parse(b.createdAt));
  const usedApprovals = new Set<string>();
  const approvalByCall = new Map<string, AgentApproval>();
  for (const message of messages) {
    if (!Array.isArray(message.toolCalls)) continue;
    message.toolCalls.forEach((rawCall, index) => {
      if (!rawCall || typeof rawCall !== "object") return;
      const call = rawCall as Record<string, unknown>;
      if (call.status !== "confirmation_required") return;
      const explicit = typeof call.approvalId === "string" ? approvalById.get(call.approvalId) : undefined;
      const approval = explicit ?? legacyApprovals.find(candidate => candidate.toolName === call.name && !usedApprovals.has(candidate.id) && Date.parse(candidate.createdAt) <= Date.parse(message.createdAt));
      if (approval && !usedApprovals.has(approval.id)) {
        usedApprovals.add(approval.id);
        approvalByCall.set(`${message.id}:${index}`, approval);
      }
    });
  }
  const occupied = busy || voice !== "off";
  const activityCallback = useRef(onActivity);
  activityCallback.current = onActivity;
  const conversationTitle = session ? titleOf(session) : "新对话";
  useEffect(() => { activityCallback.current(activeId, occupied || Boolean(deciding) || flightRunId !== null || Boolean(draft.trim()), conversationTitle); }, [activeId, occupied, deciding, flightRunId, draft, conversationTitle]);
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
        if (Array.isArray(message.toolCalls) && message.toolCalls.some(tool => tool.status === "confirmation_required")) {
          void apiJSON<AgentSessionView[]>(base).then(refreshed => {
            if (!mounted.current) return;
            setSessions(previous => previous.map(item => item.id === id ? { ...item, approvals: refreshed.find(row => row.id === id)?.approvals ?? [] } : item));
          }).catch(() => {});
        }
      },
      status: status => { if (mounted.current) setVoiceStatus(status); },
      inputStatus: (status) => { if (mounted.current) setInputStatus(status); },
      ready: () => { if (mounted.current) setVoice("connected"); },
      ended: failure => {
        if (!mounted.current) return;
        voiceCall.current = null; lock.current = false; setVoice("off");
        setVoiceStatus("通话已结束，可以继续打字交流。");
        if (failure) setError(realtimeErrorMessage(failure));
        void apiJSON<AgentSessionView[]>(base).then(refreshed => { if (mounted.current) setSessions(refreshed); }).catch(() => {});
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
          setProgress(event.data.status === "running" ? (event.data.name === "mutate_issue" || event.data.name === "create_task_draft" ? "正在准备待授权操作…" : "正在查询项目数据…") : "正在整理工具结果…");
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

  async function decide(approvalId: string, decision: "approve" | "reject") {
    if (activeId === null || deciding) return;
    setDeciding(approvalId); setError(null);
    const approval = approvals.find(item => item.id === approvalId);
    const receiptMonitor = new AbortController();
    let receiptTimer: ReturnType<typeof setTimeout> | undefined;
    let watching = false;
    const watchFlight = (status: string, receipt: unknown) => {
      if (watching || !approval || decision !== "approve") return;
      const flight = agentFlightRunId(approval.toolName, status, approval.input, receipt);
      if (flight) { watching = true; setFlightRunId(flight); }
    };
    // The authorization endpoint also waits for the AI's follow-up reply.
    // Observe its persisted receipt so live monitoring can open immediately,
    // while that reply continues in the same floating conversation.
    const readReceipt = async () => {
      try {
        const refreshed = await apiJSON<AgentSessionView[]>(base, { signal: receiptMonitor.signal });
        const decided = refreshed.find(item => item.id === activeId)?.approvals?.find(item => item.id === approvalId);
        if (decided) watchFlight(decided.status, decided.result);
      } catch { /* The original authorization request reports any failure. */ }
      if (!receiptMonitor.signal.aborted && !watching && mounted.current) receiptTimer = setTimeout(readReceipt, 2000);
    };
    if (decision === "approve" && approval && ["launch_flight", "submit_flight", "run_task"].includes(approval.toolName)) receiptTimer = setTimeout(readReceipt, 1000);
    try {
      const result = await apiJSON<{ status: string; followupStatus?: string; result?: unknown }>(`${base}/${activeId}/approvals/${approvalId}`, { method: "POST", body: JSON.stringify({ decision }) });
      watchFlight(result.status, result.result);
      setSessions(await apiJSON<AgentSessionView[]>(base));
      if (decision === "approve" && result.followupStatus === "failed") setError("授权结果已保存，但智能体回复暂未生成，请查看工具结果。");
    } catch (failure) {
      setNow(Date.now());
      if (failure instanceof APIError && failure.status === 403) setError("当前账号没有执行此操作的权限，请联系项目管理员。");
      else if (failure instanceof APIError && failure.code === "AGENT_APPROVAL_EXPIRED") setError("授权已过期，请让智能体重新发起操作。");
      else if (failure instanceof APIError && failure.status === 409) setError("资源已变化或授权已处理，请先核对平台状态，勿重复提交。");
      else if (failure instanceof APIError && failure.status === 404) setError("授权记录已失效，请让智能体重新发起操作。");
      else setError("授权处理失败，请稍后重试。");
      try { setSessions(await apiJSON<AgentSessionView[]>(base)); } catch { /* keep current view */ }
    } finally { receiptMonitor.abort(); clearTimeout(receiptTimer); setDeciding(null); }
  }

  function approvalDescription(approval: NonNullable<AgentSessionView["approvals"]>[number]) {
    if (approval.summary) return approval.summary;
    if (approval.toolName === "create_task_draft") return "为已有任务创建可编辑草稿版本；不会发布、启用或运行任务。";
    const mutation = approval.mutation;
    if (!mutation) return "修改案件";
    if (mutation.action === "comment") return `添加评论：${mutation.body ?? ""}`;
    if (mutation.action === "status") return `状态改为 ${mutation.status === "closed" ? "已关闭" : "待处理"}`;
    if (mutation.action === "labels") return `标签改为：${(mutation.labels ?? []).join("、") || "无"}`;
    if (mutation.action === "assign" || mutation.action === "unassign") return `${mutation.action === "assign" ? "分配给" : "取消分配"} ${mutation.assigneeType === "agent" ? "智能体" : "用户"} #${mutation.assigneeId}`;
    return mutation.action;
  }

  function outdatedApprovalPrompt(message: AgentSessionView["messages"][number], group: AgentSessionView["messages"]) {
    if (message.content.trim() !== "待授权操作已经生成。请核对内容后，手动点击“授权执行”或“拒绝”。") return false;
    const linked = group.flatMap(item => Array.isArray(item.toolCalls) ? item.toolCalls.map((_: unknown, index: number) => approvalByCall.get(`${item.id}:${index}`)).filter((approval): approval is AgentApproval => Boolean(approval)) : []);
    return linked.length > 0 && linked.every(approval => approval.status !== "pending" || Date.parse(approval.expiresAt) <= now);
  }

  const header = <>
      <Button size="icon" variant="ghost" title="悬浮当前会话" aria-label="悬浮当前会话" onClick={onFloat} disabled={activeId === null && !pending && !draft.trim() && voice === "off"}><PanelsTopLeft className="size-5" /></Button>
      <Button size="icon" variant="ghost" title="新对话" aria-label="新对话" onClick={() => select(null)} disabled={occupied}><Plus className="size-5" /></Button>
      <DropdownMenu open={historyOpen} onOpenChange={setHistoryOpen}>
        <DropdownMenuTrigger asChild><Button size="icon" variant="ghost" title="历史会话" aria-label="历史会话" disabled={occupied}><History className="size-5" /></Button></DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-80 max-w-[calc(100vw-2rem)]" onCloseAutoFocus={event => { event.preventDefault(); input.current?.focus(); }}>
          <DropdownMenuLabel>历史会话</DropdownMenuLabel>
          <DropdownMenuSeparator />
          <div className="max-h-80 overflow-y-auto">
            {sessions.filter(item => !unavailableSessions.includes(item.id)).map(item => <DropdownMenuItem key={item.id} onSelect={() => select(item.id)} className="gap-3 px-3 py-3" aria-current={activeId === item.id ? "true" : undefined}>
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
    </>;
  const headerCallback = useRef(onHeaderChange);
  headerCallback.current = onHeaderChange;
  useEffect(() => {
    headerCallback.current(!floating && visible ? header : null);
  }, [floating, visible, sessions, activeId, occupied, pending, Boolean(draft.trim()), historyOpen, unavailableSessions.join(",")]);
  useEffect(() => () => headerCallback.current(null), []);

  return <section aria-label="项目智能体聊天" className={`flex min-h-0 flex-col ${floating ? "h-full" : "h-[calc(100dvh-6rem)]"}`}>
    <h1 className="sr-only">项目智能体</h1>
      <div className={`min-h-0 flex-1 overflow-y-auto px-4 py-4 ${floating ? "" : "sm:px-8"}`} role="log" aria-label="对话消息" aria-live="polite">
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
                {message.content && !outdatedApprovalPrompt(message, group) && (message.role === "assistant" ? <ChatMarkdown content={message.content} /> : <p className="whitespace-pre-wrap break-words text-sm leading-7">{message.content}</p>)}
                {voice === "connected" && message.role === "user" && !message.content && <p className="text-sm opacity-70">正在聆听并转写…</p>}
                {Array.isArray(message.toolCalls) && message.toolCalls.length ? message.toolCalls.map((rawCall, index) => {
                  const approval = approvalByCall.get(`${message.id}:${index}`);
                  const call = rawCall && typeof rawCall === "object" ? rawCall as Record<string, unknown> : rawCall;
                  const expired = approval?.status === "pending" && new Date(approval.expiresAt).getTime() <= now;
                  const evidence = approval ? { ...call, status: expired ? "expired" : approval.status === "pending" ? "confirmation_required" : approval.status, summary: approvalDescription(approval) } : call;
                  return <div key={index}>
                    <AgentQueryEvidence toolCalls={[evidence]} inline />
                    {approval?.status === "pending" && !expired && <p className="mt-1 text-xs text-muted-foreground">已请求授权，请在下方确认</p>}
                    {expired ? <p className="mt-1 text-xs text-muted-foreground">本次授权已过期，请重新向智能体发起操作。</p> : null}
                    {approval?.status === "executing" && <p className="mt-1 text-xs text-muted-foreground">授权已提交，正在处理；若长时间未更新，请核对平台任务或飞行作业，勿重复提交。</p>}
                    {approval?.result != null && <details className="mt-1 text-xs text-muted-foreground"><summary className="cursor-pointer">查看平台返回结果</summary><pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-all">{JSON.stringify(approval.result, null, 2)}</pre></details>}
                  </div>;
                }) : <AgentQueryEvidence toolCalls={message.toolCalls} inline />}
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
      <div className={`sticky bottom-0 z-20 shrink-0 bg-background/95 px-4 pb-4 pt-3 backdrop-blur-sm ${floating ? "" : "sm:px-8"}`}>
        {flightRunId && <p role="status" className="mb-2 text-xs text-muted-foreground">等待飞行任务受理，随后自动打开直播…</p>}
        <div className="mx-auto max-w-3xl">
          {session?.status === "open" && approvals.some(approval => approval.status === "pending" && Date.parse(approval.expiresAt) > now) && <section aria-label="待授权操作" className="mb-3 max-h-[min(18rem,35dvh)] space-y-2 overflow-y-auto">
            {approvals.filter(approval => approval.status === "pending" && Date.parse(approval.expiresAt) > now).map(approval => <div key={approval.id} className="rounded-xl border bg-card p-3 text-sm">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="font-medium">请确认：{agentToolLabel(approval.toolName)}<span className="inline-block whitespace-pre-line">{approval.taskId ? `\n任务 #${approval.taskId}` : approval.issueId ? `\n案件 #${approval.issueId}` : ""}</span></p>
                <div className="flex gap-2">
                  <Button size="sm" disabled={Boolean(deciding) || busy} onClick={() => void decide(approval.id, "approve")}>{deciding === approval.id ? <Loader2 className="mr-2 size-4 animate-spin" /> : null}授权执行</Button>
                  <Button size="sm" variant="outline" disabled={Boolean(deciding) || busy} onClick={() => void decide(approval.id, "reject")}>拒绝</Button>
                </div>
              </div>
              <p className="mt-2 whitespace-pre-wrap break-words text-sm">{approvalDescription(approval)}</p>
              {approval.input && <details className="mt-2"><summary className="cursor-pointer text-xs text-muted-foreground">查看本次执行参数</summary><pre className="mt-2 max-h-40 overflow-auto whitespace-pre-wrap break-all rounded bg-muted p-2 text-xs">{JSON.stringify(approval.input, null, 2)}</pre></details>}
              <p className="mt-2 text-xs text-muted-foreground">{approval.toolName === "mutate_issue" ? `基于案件版本 ${approval.expectedVersion}，执行时重新检查权限和版本。` : "执行时重新检查当前账号权限及资源状态。"}</p>
              {["launch_flight", "run_task", "set_task_state", "submit_flight", "control_flight", "control_task_run"].includes(approval.toolName) && <p className="mt-1 text-xs text-amber-700 dark:text-amber-400">可能影响真实设备或后续定时运行；提交成功不等于飞机已起飞或停止。</p>}
            </div>)}
          </section>}
          {error && <p role="alert" className="mb-3 rounded-lg bg-destructive/10 px-4 py-3 text-sm text-destructive">{error}</p>}
          {session && session.status !== "open" ? <p className="mb-3 text-sm text-muted-foreground">此对话已结束，可以新建对话继续交流。</p> : null}
          <form onSubmit={event => { event.preventDefault(); void send(); }} className="rounded-2xl border bg-muted/15 p-3 shadow-sm focus-within:border-primary/50 focus-within:ring-2 focus-within:ring-primary/10">
            {voice !== "off" ? <div className="flex min-h-16 items-center gap-3 px-1" role="status">
              {voice === "connecting" ? <Loader2 className="size-5 animate-spin text-primary" /> : <AudioLines className="size-5 animate-pulse text-primary" />}
              <div><p className="text-sm font-medium">{voice === "connecting" ? "正在接通实时对话" : "实时语音对话中"}</p><p className="mt-1 text-xs text-muted-foreground">{voiceStatus}</p>{voice === "connected" && <p role="status" className="mt-1 text-xs text-muted-foreground">{inputStatus}</p>}</div>
            </div> : <textarea ref={input} aria-label="发送给项目智能体" placeholder="询问项目情况，或继续追问…" value={draft} onChange={event => setDraft(event.target.value)} disabled={busy || Boolean(session && session.status !== "open")} rows={2} className="max-h-40 min-h-16 w-full resize-none bg-transparent px-1 py-1 text-sm leading-6 outline-none placeholder:text-muted-foreground disabled:opacity-60" onKeyDown={event => { if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing && event.keyCode !== 229) { event.preventDefault(); void send(); } }} />}
            <div className="flex items-center justify-between gap-2"><span className="px-1 text-[11px] text-muted-foreground">{voice !== "off" ? "可以随时说话打断，转写与查询显示在对话中" : voiceButton ? "输入文字，或点击声波开始实时对话" : "Enter 发送 · Shift + Enter 换行"}</span>
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
