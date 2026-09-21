"use client";

import { createContext, useContext, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { Separator } from "@/components/ui/separator";
import { SidebarTrigger } from "@/components/ui/sidebar";

const HeaderActionsContext = createContext<HTMLDivElement | null>(null);

export function SiteHeaderLayout({ children }: { children: ReactNode }) {
  const [actions, setActions] = useState<HTMLDivElement | null>(null);
  return <HeaderActionsContext.Provider value={actions}>
    <SiteHeader actionsRef={setActions} />
    {children}
  </HeaderActionsContext.Provider>;
}

export function SiteHeaderActions({ children }: { children: ReactNode }) {
  const target = useContext(HeaderActionsContext);
  return target ? createPortal(children, target) : null;
}

export function SiteHeader({ actionsRef }: { actionsRef?: (element: HTMLDivElement | null) => void }) {
  return (
    <header className="flex h-16 shrink-0 items-center gap-2">
      <div className="flex items-center gap-2 px-4">
        <SidebarTrigger className="-ml-1" />
        <Separator className="mr-2 data-vertical:h-4 data-vertical:self-auto" orientation="vertical" />
        <span className="font-semibold">AeroSight</span>
      </div>
      <div ref={actionsRef} className="ml-auto flex items-center gap-1 pr-4" />
    </header>
  );
}
