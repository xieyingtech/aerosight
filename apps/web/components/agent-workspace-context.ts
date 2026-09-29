"use client";

import { createContext, useContext } from "react";
import type { AgentSessionView } from "@/lib/web-api-types";

export const AgentWorkspaceContext = createContext<{
  attach: (projectId: number, sessions: AgentSessionView[], slot: HTMLDivElement) => () => void;
} | null>(null);

export function useAgentWorkspace() {
  const value = useContext(AgentWorkspaceContext);
  if (!value) throw new Error("Agent workspace is missing");
  return value;
}
