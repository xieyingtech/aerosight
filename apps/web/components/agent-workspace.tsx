"use client";

import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { useRouter } from "next/navigation";
import { Maximize2, Minus, MessageSquare } from "lucide-react";
import { AgentConversation } from "@/components/agent-console";
import { AgentWorkspaceContext } from "@/components/agent-workspace-context";
import { Button } from "@/components/ui/button";
import { SiteHeaderActions } from "@/components/site-header";
import { projectPageHref } from "@/lib/page-routes";
import type { AgentSessionView } from "@/lib/web-api-types";

type Conversation = { key: string; projectId: number; sessions: AgentSessionView[]; element: HTMLDivElement; floating: boolean; blank: boolean; sessionId: number | null; active: boolean; title: string };

// Every conversation keeps the same portal container, including during navigation.
// Moving this container preserves the stream, voice connection and local draft.
export function AgentWorkspace({ children, projects }: { children: ReactNode; projects: Array<{ id: number; name: string }> }) {
  const router = useRouter();
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [minimized, setMinimized] = useState(false);
  const [headers, setHeaders] = useState<Record<string, ReactNode>>({});
  const slots = useRef(new Map<number, HTMLDivElement>());
  const parked = useRef<HTMLDivElement>(null);
  const floatingSlot = useRef<HTMLDivElement>(null);
  const create = (projectId: number, sessions: AgentSessionView[], blank = false): Conversation => ({
    key: crypto.randomUUID(), projectId, sessions, blank, floating: false, sessionId: blank ? null : sessions[0]?.id ?? null, active: false, title: "新对话", element: document.createElement("div"),
  });
  const attach = useCallback((projectId: number, sessions: AgentSessionView[], slot: HTMLDivElement) => {
    slots.current.set(projectId, slot);
    const candidate = create(projectId, sessions);
    setConversations(previous => previous.some(item => item.projectId === projectId && !item.floating) ? [...previous] : [...previous, { ...candidate, blank: previous.some(item => item.projectId === projectId && item.floating), sessionId: previous.some(item => item.projectId === projectId && item.floating) ? null : candidate.sessionId }]);
    return () => {
      if (slots.current.get(projectId) === slot) slots.current.delete(projectId);
      // Navigating away must not tear down an in-progress conversation.
      // Strict Mode and hot reload detach/re-attach the same outlet immediately.
      queueMicrotask(() => {
        if (slots.current.has(projectId)) return;
        setConversations(previous => previous.map(item => item.projectId === projectId && !item.floating && (item.sessionId !== null || item.active) ? { ...item, floating: true } : item));
      });
    };
  }, []);
  const float = (key: string) => {
    setMinimized(false);
    setConversations(previous => {
      const current = previous.find(item => item.key === key);
      if (!current || current.floating) return previous;
      return [...previous.map(item => item.key === key ? { ...item, floating: true } : item), create(current.projectId, current.sessions, true)];
    });
  };
  const restore = (key: string) => {
    const current = conversations.find(item => item.key === key);
    if (!current) return;
    setConversations(previous => previous.filter(item => item.key === key || item.projectId !== current.projectId || item.sessionId !== null || item.active).map(item => item.projectId === current.projectId ? { ...item, floating: item.key !== key } : item));
    router.push(projectPageHref(current.projectId, "agents"));
  };
  const floating = conversations.filter(item => item.floating);
  const [selected, setSelected] = useState<string | null>(null);
  const activeFloating = floating.find(item => item.key === selected) ?? floating[0];
  useEffect(() => {
    for (const item of conversations) {
      const slot = item.floating ? (item === activeFloating ? floatingSlot.current : parked.current) : slots.current.get(item.projectId) ?? parked.current;
      if (slot && item.element.parentElement !== slot) slot.appendChild(item.element);
    }
  }, [conversations, activeFloating, minimized]);
  return <AgentWorkspaceContext.Provider value={{ attach }}>
    {children}
    <SiteHeaderActions>{conversations.filter(item => !item.floating && slots.current.has(item.projectId)).map(item => <span key={item.key} className="flex items-center gap-1">{headers[item.key]}</span>)}</SiteHeaderActions>
    <div ref={parked} hidden />
    {activeFloating && <aside aria-label="悬浮智能体会话" className={`fixed bottom-4 right-4 z-50 flex max-h-[calc(100dvh-2rem)] w-[min(26rem,calc(100vw-2rem))] flex-col overflow-hidden rounded-xl border bg-background shadow-2xl ${minimized ? "" : "h-[min(38rem,calc(100dvh-2rem))]"}`}>
      <div className="flex shrink-0 items-center gap-2 border-b px-3 py-2">
        <MessageSquare className="size-4 text-primary" />
        <span className="min-w-0 flex-1 truncate text-sm font-medium" title={activeFloating.title}><span className="inline-flex max-w-full flex-wrap items-center gap-x-3 gap-y-1"><span>{projects.find(project => project.id === activeFloating.projectId)?.name ?? `项目 #${activeFloating.projectId}`}</span><span>智能体</span></span></span>
        <Button size="icon" variant="ghost" title="返回完整会话" aria-label="返回完整会话" onClick={() => restore(activeFloating.key)}><Maximize2 className="size-4" /></Button>
        <Button size="icon" variant="ghost" title={minimized ? "展开会话" : "收起会话"} aria-label={minimized ? "展开会话" : "收起会话"} onClick={() => setMinimized(value => !value)}><Minus className="size-4" /></Button>
      </div>
      {floating.length > 1 && !minimized && <select aria-label="切换悬浮会话" className="border-b bg-background p-2 text-sm" value={activeFloating.key} onChange={event => setSelected(event.target.value)}>{floating.map(item => <option key={item.key} value={item.key}>{projects.find(project => project.id === item.projectId)?.name ?? `项目 #${item.projectId}`}（{item.title.slice(0, 32)}）</option>)}</select>}
      <div ref={floatingSlot} className={minimized ? "hidden" : "min-h-0 flex-1 [&>div]:h-full"} />
    </aside>}
    {conversations.map(item => createPortal(<AgentConversation projectId={item.projectId} sessions={item.sessions} initiallyBlank={item.blank} floating={item.floating} visible={item.floating ? item === activeFloating && !minimized : slots.current.has(item.projectId)} unavailableSessions={conversations.filter(other => other.key !== item.key && other.projectId === item.projectId && other.sessionId !== null).map(other => other.sessionId!)} onHeaderChange={header => setHeaders(previous => ({ ...previous, [item.key]: header }))} onActivity={(sessionId, active, title) => setConversations(previous => previous.map(other => other.key === item.key && (other.sessionId !== sessionId || other.active !== active || other.title !== title) ? { ...other, sessionId, active, title } : other))} onFloat={() => { setSelected(item.key); float(item.key); }} onFlightAccepted={href => { setSelected(item.key); float(item.key); router.push(href); }} />, item.element, item.key))}
  </AgentWorkspaceContext.Provider>;
}
