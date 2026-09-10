"use client";

import { Suspense, type ComponentProps, type ReactNode } from "react";
import { AppSidebar } from "@/components/app-sidebar";
import { SiteHeader } from "@/components/site-header";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { SessionProvider, useSessionUser } from "@/components/session-provider";
import { APIStateView } from "@/components/api-state";
import { useAPI } from "@/lib/use-api";

function Shell({ children }: { children: ReactNode }) {
  const user = useSessionUser();
  const projects = useAPI<ComponentProps<typeof AppSidebar>["projects"]>("/api/projects");
  if (!user) return null;
  return <APIStateView state={projects}>{(projects) => <SidebarProvider>
    <Suspense><AppSidebar projects={projects} user={user} /></Suspense>
    <SidebarInset><SiteHeader /><main className="flex flex-1 flex-col gap-4 p-4 pt-0">{children}</main></SidebarInset>
  </SidebarProvider>}</APIStateView>;
}

export function ClientAppShell({ children }: { children: ReactNode }) {
  return <SessionProvider><Shell>{children}</Shell></SessionProvider>;
}
