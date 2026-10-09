"use client";

import { Suspense, useEffect, useRef, type ComponentProps, type ReactNode } from "react";
import { AppSidebar } from "@/components/app-sidebar";
import { SiteHeaderLayout } from "@/components/site-header";
import { SidebarInset, SidebarProvider, useSidebar } from "@/components/ui/sidebar";
import { SessionProvider, useSessionUser } from "@/components/session-provider";
import { APIStateView } from "@/components/api-state";
import { useAPI } from "@/lib/use-api";
import { AgentWorkspace } from "@/components/agent-workspace";
import { usePathname } from "next/navigation";

function CanvasNavigation({ canvas }: { canvas: boolean }) {
  const { setOpen } = useSidebar();
  const previous = useRef(false);
  useEffect(() => {
    if (canvas && !previous.current) setOpen(false);
    previous.current = canvas;
  }, [canvas, setOpen]);
  return null;
}

function Shell({ children }: { children: ReactNode }) {
  const user = useSessionUser();
  const pathname = usePathname();
  const canvas = /^\/projects\/(?:[1-9]\d*\/?|[1-9]\d*\/realtime(?:\/devices\/[1-9]\d*)?\/?|detail\/?|realtime\/?)$/.test(pathname);
  const projects = useAPI<ComponentProps<typeof AppSidebar>["projects"]>("/api/projects");
  if (!user) return null;
  return <APIStateView state={projects}>{(projects) => <SidebarProvider defaultOpen={!canvas} className={canvas ? "h-dvh min-h-0 overflow-hidden" : undefined}>
    <CanvasNavigation canvas={canvas} />
    <Suspense><AppSidebar projects={projects} user={user} variant={canvas ? "sidebar" : "inset"} /></Suspense>
    <SidebarInset className={canvas ? "min-h-0 min-w-0 overflow-hidden" : undefined}><SiteHeaderLayout compact={canvas}><AgentWorkspace projects={projects}><div className={canvas ? "relative min-h-0 flex-1 overflow-hidden" : "flex flex-1 flex-col gap-4 p-4 pt-0"}>{children}</div></AgentWorkspace></SiteHeaderLayout></SidebarInset>
  </SidebarProvider>}</APIStateView>;
}

export function ClientAppShell({ children }: { children: ReactNode }) {
  return <SessionProvider><Shell>{children}</Shell></SessionProvider>;
}
