"use client";

import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { MaximizeIcon, MinimizeIcon, PanelLeftIcon, PanelRightIcon, XIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";

// Panels overlay the canvas, so opening them never resizes or reconnects video.
export function CanvasWorkspace({ title, subtitle, showTitle = true, leftTitle, left, rightTitle, right, children }: {
  title: string; subtitle?: string; showTitle?: boolean; leftTitle?: string; left?: ReactNode;
  rightTitle?: string; right?: ReactNode; children: ReactNode;
}) {
  const ownsFullscreen = useRef(false);
  const id = useId();
  const [leftOpen, setLeftOpen] = useState(false);
  const [rightOpen, setRightOpen] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    const wide = window.matchMedia("(min-width: 1280px)");
    setLeftOpen(wide.matches);
    setRightOpen(wide.matches);
    const resize = () => { if (!wide.matches) { setLeftOpen(false); setRightOpen(false); } };
    const change = () => {
      const active = document.fullscreenElement === document.documentElement;
      setFullscreen(active);
      if (!active) ownsFullscreen.current = false;
    };
    wide.addEventListener("change", resize);
    document.addEventListener("fullscreenchange", change);
    return () => {
      wide.removeEventListener("change", resize);
      document.removeEventListener("fullscreenchange", change);
      if (ownsFullscreen.current && document.fullscreenElement) void document.exitFullscreen().catch(() => {});
    };
  }, []);
  const toggle = (side: "left" | "right") => {
    if (side === "left") { setLeftOpen(value => !value); if (window.innerWidth < 1280) setRightOpen(false); }
    else { setRightOpen(value => !value); if (window.innerWidth < 1280) setLeftOpen(false); }
  };
  const expand = async () => {
    try {
      setError("");
      if (document.fullscreenElement) await document.exitFullscreen();
      else { await document.documentElement.requestFullscreen(); ownsFullscreen.current = true; }
    } catch { setError("浏览器暂不支持全屏，请使用浏览器的全屏菜单。"); }
  };
  return <div className={cn("isolate min-h-0 w-full overflow-hidden bg-background", fullscreen ? "fixed inset-0 z-40 h-dvh" : "relative h-full")} aria-label={title}>
    <div className="absolute inset-0">{children}</div>
    <div className="pointer-events-none absolute inset-x-0 top-0 z-20 flex h-14 items-center justify-between gap-2 px-3">
      { (showTitle || left) ? <div className="pointer-events-auto flex min-w-0 items-center gap-2 rounded-lg border bg-background/95 p-1 shadow-sm">
        {left && <Button variant="ghost" size="icon-sm" aria-label={leftOpen ? `收起${leftTitle}` : `展开${leftTitle}`} aria-expanded={leftOpen} aria-controls={`${id}-left`} onClick={() => toggle("left")}><PanelLeftIcon /></Button>}
        {showTitle && <div className="min-w-0 pr-2"><h2 className="truncate text-sm font-semibold">{title}</h2>{subtitle && <p className="max-w-48 truncate text-xs text-muted-foreground sm:max-w-80">{subtitle}</p>}</div>}
      </div> : <span />}
      <div className="pointer-events-auto flex shrink-0 gap-1 rounded-lg border bg-background/95 p-1 shadow-sm">
        <Button variant="ghost" size="icon-sm" aria-label={fullscreen ? "退出全屏" : "全屏主画面"} title={fullscreen ? "退出全屏" : "全屏主画面"} onClick={expand}>{fullscreen ? <MinimizeIcon /> : <MaximizeIcon />}</Button>
        {right && <Button variant="ghost" size="icon-sm" aria-label={rightOpen ? `收起${rightTitle}` : `展开${rightTitle}`} aria-expanded={rightOpen} aria-controls={`${id}-right`} onClick={() => toggle("right")}><PanelRightIcon /></Button>}
      </div>
    </div>
    {(["left", "right"] as const).map(side => {
      if (!(side === "left" ? left : right)) return null;
      const open = side === "left" ? leftOpen : rightOpen;
      const label = side === "left" ? leftTitle : rightTitle;
      return <aside key={side} id={`${id}-${side}`} aria-label={label} hidden={!open} className={cn("absolute bottom-14 top-16 z-20 w-[min(20rem,calc(100%-1.5rem))]", side === "left" ? "left-3" : "right-3")}>
        <Card className="h-full gap-0 overflow-hidden py-0 shadow-xl">
          <div className="flex shrink-0 items-center justify-between border-b px-3 py-2"><h3 className="text-sm font-medium">{label}</h3><Button size="icon-sm" variant="ghost" aria-label={`收起${label}`} onClick={() => toggle(side)}><XIcon /></Button></div>
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-3">{side === "left" ? left : right}</div>
        </Card>
      </aside>;
    })}
    {error && <p role="alert" className="absolute bottom-3 left-1/2 z-30 w-max max-w-[90%] -translate-x-1/2 rounded-lg border bg-background p-3 text-sm">{error}</p>}
  </div>;
}
