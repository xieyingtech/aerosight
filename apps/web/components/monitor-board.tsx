"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { ExpandIcon, GripVerticalIcon, MapIcon, VideoIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { closestMonitorCorner, mergeMonitorOrder, swapMonitorViews, type MonitorView } from "@/lib/monitor-layout";

export function MonitorBoard({ projectId, views, focusedId, onFocus, renderView }: {
  projectId: number; views: MonitorView[]; focusedId: string | null;
  onFocus: (view: MonitorView) => void; renderView: (view: MonitorView) => ReactNode;
}) {
  const [corner, setCorner] = useState("bottom-right");
  const boardRef = useRef<HTMLDivElement>(null);
  const snapAnimation = useRef<Animation | null>(null);
  useEffect(() => () => snapAnimation.current?.cancel(), []);
  const reorderDrag = useRef<string | null>(null);
  const floatingDrag = useRef<{ startX: number; startY: number; x: number; y: number; width: number; height: number } | null>(null);
  const [floatingPosition, setFloatingPosition] = useState<{ x: number; y: number } | null>(null);
  const [order, setOrder] = useState<string[]>([]);
  const [dragging, setDragging] = useState<string | null>(null);
  const [dropTarget, setDropTarget] = useState<string | null>(null);
  const [notice, setNotice] = useState("");
  const initialized = useRef(false);
  const storageKey = `aerosight:monitor-layout:${projectId}`;
  useEffect(() => {
    try {
      const saved: unknown = JSON.parse(localStorage.getItem(storageKey) ?? "[]");
      const savedCorner = localStorage.getItem(`${storageKey}:corner`);
      if (savedCorner && ["top-left", "top-right", "bottom-left", "bottom-right"].includes(savedCorner)) setCorner(savedCorner);
      if (Array.isArray(saved) && saved.every(id => typeof id === "string")) setOrder(saved);
    } catch { /* A blocked browser storage does not prevent arranging views. */ }
    initialized.current = true;
  }, [storageKey]);
  const positions = mergeMonitorOrder(order, views.map(view => view.id));
  const swap = (source: string, target: string) => {
    snapAnimation.current?.cancel();
    const next = swapMonitorViews(positions, source, target);
    setOrder(next);
    if (initialized.current) try { localStorage.setItem(storageKey, JSON.stringify(next)); } catch { /* Optional persistence. */ }
    setNotice("视角位置已更新");
  };
  const pictureInPicture = views.length === 2;
  const rows = Math.max(1, views.length - 1);
  return <div ref={boardRef} aria-label="多路监控" className="relative grid h-full min-h-0 gap-2 bg-slate-950 p-2" style={{ gridTemplateColumns: views.length > 2 ? "minmax(0, 3fr) minmax(160px, 1fr)" : "minmax(0, 1fr)", gridTemplateRows: `repeat(${rows}, minmax(0, 1fr))` }}>
    {/* Stable keyed siblings stay mounted; only their grid positions change. */}
    {views.map(view => {
      const position = positions.indexOf(view.id), main = position === 0;
      const Icon = view.kind === "map" ? MapIcon : VideoIcon;
      return <section key={view.id} aria-label={`${main ? "主视角" : "分视角"}：${view.label}`} data-view-id={view.id}
        className={cn("flex min-h-0 min-w-0 flex-col overflow-hidden rounded-lg border bg-slate-950", dragging === view.id && "opacity-70", pictureInPicture && !main && "absolute z-10 h-[32%] min-h-40 w-[34%] min-w-56 shadow-xl", dropTarget === view.id ? "border-sky-400 ring-2 ring-sky-400/40" : focusedId === view.id ? "border-slate-400" : "border-slate-700")}
        style={pictureInPicture && !main ? floatingPosition ? { top: floatingPosition.y, left: floatingPosition.x } : { top: corner.startsWith("top") ? 16 : undefined, bottom: corner.startsWith("bottom") ? 16 : undefined, left: corner.endsWith("left") ? 16 : undefined, right: corner.endsWith("right") ? 16 : undefined } : { gridColumn: main ? "1" : "2", gridRow: main ? `1 / span ${rows}` : String(position) }}>
        <div className="flex h-9 shrink-0 items-center gap-1 border-b border-slate-700 bg-slate-900 px-1 text-slate-200">
          <button type="button" aria-label={`拖动${view.label}`} title={pictureInPicture && !main ? "拖动分画面，松开后吸附到角落" : "拖动交换视角位置"} className="grid size-7 shrink-0 touch-none cursor-grab place-items-center rounded hover:bg-slate-800 active:cursor-grabbing"
            onPointerDown={event => {
              if (event.button !== 0 || !boardRef.current) return;
              event.currentTarget.setPointerCapture(event.pointerId);
              if (!pictureInPicture || main) { reorderDrag.current = view.id; setDragging(view.id); return; }
              const board = boardRef.current.getBoundingClientRect();
              const tileElement = event.currentTarget.closest("section")!;
              const tile = tileElement.getBoundingClientRect();
              snapAnimation.current?.cancel();
              setFloatingPosition({ x: tile.left - board.left, y: tile.top - board.top });
              floatingDrag.current = { startX: event.clientX, startY: event.clientY, x: tile.left - board.left, y: tile.top - board.top, width: tile.width, height: tile.height };
            }}
            onPointerMove={event => {
              if (reorderDrag.current) {
                const target = document.elementFromPoint(event.clientX, event.clientY)?.closest<HTMLElement>("[data-view-id]");
                setDropTarget(target && boardRef.current?.contains(target) && target.dataset.viewId !== reorderDrag.current ? target.dataset.viewId ?? null : null);
                return;
              }
              const drag = floatingDrag.current;
              if (!drag || !boardRef.current) return;
              const board = boardRef.current.getBoundingClientRect();
              setFloatingPosition({ x: Math.max(8, Math.min(board.width - drag.width - 8, drag.x + event.clientX - drag.startX)), y: Math.max(8, Math.min(board.height - drag.height - 8, drag.y + event.clientY - drag.startY)) });
            }}
            onPointerUp={event => {
              if (reorderDrag.current) {
                const target = document.elementFromPoint(event.clientX, event.clientY)?.closest<HTMLElement>("[data-view-id]");
                if (target?.dataset.viewId && boardRef.current?.contains(target)) swap(reorderDrag.current, target.dataset.viewId);
                reorderDrag.current = null; setDragging(null); setDropTarget(null);
                event.currentTarget.releasePointerCapture(event.pointerId);
                return;
              }
              const drag = floatingDrag.current;
              if (!drag || !boardRef.current) return;
              const board = boardRef.current.getBoundingClientRect();
              const nextCorner = closestMonitorCorner(drag.x + event.clientX - drag.startX + drag.width / 2, drag.y + event.clientY - drag.startY + drag.height / 2, board.width, board.height);
              const tileElement = event.currentTarget.closest("section")!;
              const tile = tileElement.getBoundingClientRect();
              const targetX = nextCorner.endsWith("left") ? 16 : board.width - tile.width - 16;
              const targetY = nextCorner.startsWith("top") ? 16 : board.height - tile.height - 16;
              setCorner(nextCorner);
              try { localStorage.setItem(`${storageKey}:corner`, nextCorner); } catch { /* Optional persistence. */ }
              floatingDrag.current = null;
              setFloatingPosition(null);
              if (!window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
                snapAnimation.current = tileElement.animate([
                  { transform: `translate(${tile.left - board.left - targetX}px, ${tile.top - board.top - targetY}px)` },
                  { transform: "translate(0, 0)" }
                ], { duration: 280, easing: "cubic-bezier(0.22, 1, 0.36, 1)" });
              }
              event.currentTarget.releasePointerCapture(event.pointerId);
            }}
            onPointerCancel={() => { floatingDrag.current = null; reorderDrag.current = null; setDragging(null); setDropTarget(null); setFloatingPosition(null); }}>
            <GripVerticalIcon className="size-4" /></button>
          <button type="button" className="flex min-w-0 flex-1 items-center gap-1.5 text-left text-xs" onClick={() => onFocus(view)}><Icon className="size-3.5 shrink-0" /><span className="truncate">{view.label}</span>{main && <span className="shrink-0 text-[10px] text-slate-400">主视角</span>}</button>
          {!main && <Button size="icon-sm" variant="ghost" className="size-7 shrink-0 text-slate-300 hover:bg-slate-800 hover:text-white" aria-label={`将${view.label}设为主视角`} title="设为主视角" onClick={() => { swap(view.id, positions[0]); onFocus(view); }}><ExpandIcon className="size-3.5" /></Button>}
        </div>
        <div className="relative min-h-0 flex-1">{renderView(view)}</div>
      </section>;
    })}
    <span role="status" className="sr-only">{notice}</span>
  </div>;
}
