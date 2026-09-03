import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export function Page({
  title,
  description,
  actions,
  variant = "default",
  children
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
  variant?: "default" | "workspace";
  children: ReactNode;
}) {
  return (
    <section className={cn(
      variant === "workspace" ? "flex min-h-[calc(100svh-5rem)] flex-col gap-3" : "space-y-6"
    )}>
      <header className={cn(
        "flex flex-wrap items-start justify-between gap-3 border-b",
        variant === "workspace" ? "shrink-0 pb-3" : "pb-4"
      )}>
        <div>
          <h1 className={cn("font-semibold tracking-tight", variant === "workspace" ? "text-xl" : "text-2xl")}>{title}</h1>
          {description ? <p className="mt-1 text-sm text-muted-foreground">{description}</p> : null}
        </div>
        {actions}
      </header>
      {children}
    </section>
  );
}
