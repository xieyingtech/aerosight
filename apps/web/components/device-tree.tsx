"use client";

import Link from "next/link";
import { useEffect, useMemo, useState, type ComponentType } from "react";
import {
  ActivityIcon, ArrowRightIcon, BotIcon, BoxIcon, CameraIcon, ChevronRightIcon,
  CpuIcon, PlaneIcon, RadioIcon, SearchIcon, WarehouseIcon
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Input } from "@/components/ui/input";
import { filterDeviceTree, flattenDeviceTree } from "@/lib/device-manager-core";
import type { DeviceTreeNode } from "@/lib/device-tree-core";
import { cn } from "@/lib/utils";

const CATEGORY_LABELS: Record<string, string> = {
  aircraft: "无人机", camera: "摄像头", dock: "机场", robot: "机器人", sensor: "传感器"
};

const CATEGORY_ICONS: Record<string, ComponentType<{ className?: string }>> = {
  aircraft: PlaneIcon, camera: CameraIcon, dock: WarehouseIcon, robot: BotIcon, sensor: ActivityIcon
};

const STATUS_LABELS: Record<string, string> = {
  degraded: "异常", offline: "离线", online: "在线", unknown: "未知"
};

function statusClass(status: string) {
  return status === "online" ? "bg-emerald-500" : status === "degraded" ? "bg-amber-500" : "bg-muted-foreground/50";
}

function availabilityLabel(availability: string) {
  return availability === "available" ? "可用" : availability === "degraded" ? "降级" : "不可用";
}

function DeviceBranch({ node, depth, selectedId, onSelect, searching }: {
  node: DeviceTreeNode; depth: number; selectedId: number; onSelect: (id: number) => void; searching: boolean;
}) {
  const [open, setOpen] = useState(depth < 1);
  const hasChildren = node.children.length > 0;
  const Icon = CATEGORY_ICONS[node.category] ?? CpuIcon;

  useEffect(() => {
    if (searching) setOpen(true);
  }, [searching]);

  return <li role="treeitem" aria-expanded={hasChildren ? open : undefined} aria-selected={node.id === selectedId}>
    <Collapsible open={open} onOpenChange={setOpen}>
      <div className={cn("group flex items-center rounded-md", node.id === selectedId ? "bg-primary/10 text-primary" : "hover:bg-muted/70")}
        style={{ paddingLeft: `${depth * 16 + 4}px` }}>
        {hasChildren ? <CollapsibleTrigger asChild>
          <button aria-label={open ? `收起${node.name}` : `展开${node.name}`} className="flex size-7 shrink-0 items-center justify-center rounded-sm hover:bg-muted" type="button">
            <ChevronRightIcon className={cn("size-3.5 transition-transform", open && "rotate-90")} />
          </button>
        </CollapsibleTrigger> : <span className="size-7 shrink-0" />}
        <button className="flex min-w-0 flex-1 items-center gap-2 py-2 pr-2 text-left" onClick={() => onSelect(node.id)} type="button">
          <Icon className="size-4 shrink-0" />
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm font-medium">{node.name}</span>
            <span className="block truncate text-[11px] text-muted-foreground">{CATEGORY_LABELS[node.category] ?? node.category}{node.relationType ? ` · ${node.relationType}` : ""}</span>
          </span>
          <span aria-label={STATUS_LABELS[node.status] ?? node.status} className={cn("size-2 shrink-0 rounded-full", statusClass(node.status))} />
        </button>
      </div>
      {hasChildren && <CollapsibleContent>
        <ul role="group">{node.children.map((child) => <DeviceBranch depth={depth + 1} key={child.id} node={child}
          onSelect={onSelect} searching={searching} selectedId={selectedId} />)}</ul>
      </CollapsibleContent>}
    </Collapsible>
  </li>;
}

function DetailItem({ label, value }: { label: string; value: string }) {
  return <div className="rounded-lg border bg-muted/25 px-3 py-2.5">
    <dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1 truncate text-sm font-medium" title={value}>{value}</dd>
  </div>;
}

function DeviceDetails({ device, projectId }: { device: DeviceTreeNode; projectId: number }) {
  const Icon = CATEGORY_ICONS[device.category] ?? BoxIcon;
  return <div className="flex h-full min-h-0 flex-col">
    <header className="flex flex-wrap items-start justify-between gap-4 border-b p-5">
      <div className="flex min-w-0 items-start gap-3">
        <div className="rounded-lg bg-primary/10 p-2.5 text-primary"><Icon className="size-5" /></div>
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2"><h2 className="truncate text-lg font-semibold">{device.name}</h2><Badge variant="outline">{CATEGORY_LABELS[device.category] ?? device.category}</Badge></div>
          <p className="mt-1 text-xs text-muted-foreground">设备 ID {device.id} · {device.typeKey}</p>
        </div>
      </div>
      <Button asChild size="sm"><Link href={`/projects/${projectId}/realtime?deviceId=${device.id}`}>进入实时作业<ArrowRightIcon /></Link></Button>
    </header>
    <div className="min-h-0 flex-1 space-y-6 overflow-y-auto p-5">
      <section>
        <h3 className="mb-3 text-sm font-semibold">设备概况</h3>
        <dl className="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
          <DetailItem label="设备类型" value={device.typeName} />
          <DetailItem label="驱动" value={`${device.driverKey}@${device.driverVersion}`} />
          <DetailItem label="连接状态" value={`${STATUS_LABELS[device.status] ?? device.status} · ${device.dataFreshness}`} />
          <DetailItem label="厂商 / 型号" value={[device.vendor, device.model].filter(Boolean).join(" · ") || "未设置"} />
        </dl>
        {device.statusReason && <p className="mt-2 rounded-lg bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-400">{device.statusReason}</p>}
      </section>
      <section>
        <div className="mb-3 flex items-center gap-2"><CpuIcon className="size-4 text-muted-foreground" /><h3 className="text-sm font-semibold">设备能力</h3><Badge variant="secondary">{device.capabilities.length}</Badge></div>
        {device.capabilities.length ? <div className="grid gap-2 md:grid-cols-2">{device.capabilities.map((capability) =>
          <div className="rounded-lg border p-3" key={capability.code}>
            <div className="flex items-center justify-between gap-2"><code className="text-xs font-medium">{capability.code}</code><Badge variant={capability.availability === "available" ? "secondary" : "outline"}>{availabilityLabel(capability.availability)}</Badge></div>
            <p className="mt-2 text-xs text-muted-foreground">{capability.authorized ? "已授权" : "未授权"} · 风险 {capability.risk}{capability.reason ? ` · ${capability.reason}` : ""}</p>
          </div>)}</div> : <p className="rounded-lg border border-dashed p-5 text-center text-sm text-muted-foreground">该设备类型尚未声明能力</p>}
      </section>
      <section>
        <div className="mb-3 flex items-center gap-2"><RadioIcon className="size-4 text-muted-foreground" /><h3 className="text-sm font-semibold">实时通道</h3><Badge variant="secondary">{device.channels.length}</Badge></div>
        {device.channels.length ? <div className="grid gap-2 md:grid-cols-2">{device.channels.map((channel) =>
          <div className="rounded-lg border p-3" key={channel.stableChannelId}>
            <div className="flex items-center justify-between gap-2"><span className="text-sm font-medium">{channel.name}</span><Badge variant="outline">{availabilityLabel(channel.availability)}</Badge></div>
            <p className="mt-1 text-xs text-muted-foreground">{channel.dataType} · {channel.stableChannelId}</p>
          </div>)}</div> : <p className="rounded-lg border border-dashed p-5 text-center text-sm text-muted-foreground">该设备没有实时数据或直播通道</p>}
      </section>
    </div>
  </div>;
}

export function DeviceTree({ nodes, projectId }: { nodes: DeviceTreeNode[]; projectId: number }) {
  const devices = useMemo(() => flattenDeviceTree(nodes), [nodes]);
  const [selectedId, setSelectedId] = useState(devices[0]?.id ?? 0);
  const [query, setQuery] = useState("");
  const filteredNodes = useMemo(() => filterDeviceTree(nodes, query), [nodes, query]);
  const selected = devices.find((device) => device.id === selectedId) ?? devices[0];

  if (!selected) return <div className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">暂无已认领设备</div>;

  return <div className="grid min-h-[620px] overflow-hidden rounded-xl border bg-card lg:h-[calc(100vh-12rem)] lg:grid-cols-[320px_minmax(0,1fr)]">
    <aside className="flex min-h-0 flex-col border-b lg:border-r lg:border-b-0">
      <div className="border-b p-4">
        <div className="mb-3 flex items-center justify-between"><div><h2 className="text-sm font-semibold">设备树</h2><p className="text-xs text-muted-foreground">{devices.length} 个设备</p></div><Badge variant="outline">拓扑</Badge></div>
        <label className="relative block"><SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" /><span className="sr-only">搜索设备</span><Input className="pl-8" onChange={(event) => setQuery(event.target.value)} placeholder="搜索名称、类型、驱动…" value={query} /></label>
      </div>
      <div className="max-h-80 min-h-0 flex-1 overflow-y-auto p-2 lg:max-h-none">
        {filteredNodes.length ? <ul aria-label="项目设备" role="tree">{filteredNodes.map((node) => <DeviceBranch depth={0} key={node.id} node={node} onSelect={setSelectedId} searching={Boolean(query.trim())} selectedId={selected.id} />)}</ul>
          : <div className="p-6 text-center text-sm text-muted-foreground">没有匹配的设备</div>}
      </div>
    </aside>
    <main className="min-h-0"><DeviceDetails device={selected} projectId={projectId} /></main>
  </div>;
}
