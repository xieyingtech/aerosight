"use client";

import { useLayoutEffect, useRef, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

export function ProviderModelBadges({ models }: { models: { id: string }[] }) {
  const root = useRef<HTMLDivElement>(null);
  const measure = useRef<HTMLDivElement>(null);
  const [visible, setVisible] = useState(0);
  const [badgeWidth, setBadgeWidth] = useState(160);
  const [open, setOpen] = useState(false);
  const signature = JSON.stringify(models.map((model) => model.id));

  useLayoutEffect(() => {
    const container = root.current;
    const measurements = measure.current;
    if (!container || !measurements) return;
    function update() {
      const width = container!.clientWidth;
      const items = Array.from(measurements!.querySelectorAll<HTMLElement>("[data-model]"));
      const more = measurements!.querySelector<HTMLElement>("[data-more]")?.offsetWidth ?? 40;
      // Keep at least one readable (possibly truncated) model on narrow screens.
      const maxBadge = Math.max(0, Math.min(160, width - (items.length > 1 ? more + 6 : 0)));
      const sizes = items.map((item) => Math.min(item.offsetWidth, maxBadge));
      let count = sizes.length;
      let used = sizes.reduce((sum, size) => sum + size, 0) + Math.max(0, count - 1) * 6;
      while (count > 0 && used + (count < items.length ? more + 6 : 0) > width) {
        used -= sizes[--count] + (count > 0 ? 6 : 0);
      }
      setBadgeWidth(maxBadge);
      setVisible(count);
    }
    const observer = new ResizeObserver(update);
    observer.observe(container);
    observer.observe(measurements);
    update();
    return () => observer.disconnect();
  }, [signature]);

  return <div ref={root} className="relative min-w-0">
    <div ref={measure} aria-hidden className="pointer-events-none invisible absolute flex w-max gap-1.5">
      {models.map((model, index) => <Badge data-model key={index} variant="secondary" className="max-w-40"><span className="truncate">{model.id}</span></Badge>)}
      <Badge data-more variant="outline">...+{models.length}</Badge>
    </div>
    {models.length ? <Tooltip open={open} onOpenChange={setOpen}>
      <TooltipTrigger asChild><button type="button" aria-label={`查看全部 ${models.length} 个模型`} onClick={() => setOpen(!open)} className="flex h-8 w-full min-w-0 items-center gap-1.5 overflow-hidden rounded text-left outline-none focus-visible:ring-2 focus-visible:ring-ring">
        {models.slice(0, visible).map((model, index) => <Badge key={index} variant="secondary" className="shrink-0" style={{ maxWidth: badgeWidth }}><span className="truncate">{model.id}</span></Badge>)}
        {visible < models.length ? <Badge variant="outline" className="shrink-0">...+{models.length - visible}</Badge> : null}
      </button></TooltipTrigger>
      <TooltipContent side="bottom" align="start" sideOffset={6} className="block max-w-[min(24rem,90vw)] border bg-popover p-3 text-popover-foreground shadow-md">
        <p className="mb-2 font-medium">全部模型 · {models.length}</p>
        <ul className="max-h-64 space-y-1.5 overflow-y-auto">{models.map((model, index) => <li key={index} className="break-all">{model.id}</li>)}</ul>
      </TooltipContent>
    </Tooltip> : <span className="text-xs text-muted-foreground">暂无模型</span>}
  </div>;
}
