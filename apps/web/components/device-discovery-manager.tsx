"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { AlertTriangleIcon, CheckIcon, RefreshCwIcon, ScanSearchIcon, XIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  canConfirmDiscovery, DISCOVERY_STATUS_LABELS, filterDiscoveries,
  type DeviceDiscovery, type DeviceTypeOption, type DiscoveryConnector, type DiscoveryStatus,
} from "@/lib/device-discovery-core";

type Props = {
  projectId: number;
  discoveries: DeviceDiscovery[];
  deviceTypes: DeviceTypeOption[];
  connectors: DiscoveryConnector[];
  canManage: boolean;
};

const STATUS_ORDER: Array<DiscoveryStatus | "all"> = ["all", "discovered", "conflicted", "managed", "missing", "ignored"];

function DiscoveryRow({ item, projectId, deviceTypes, canManage, busy, act }: {
  item: DeviceDiscovery; projectId: number; deviceTypes: DeviceTypeOption[]; canManage: boolean; busy: boolean;
  act: (url: string, body: unknown) => Promise<void>;
}) {
  const [name, setName] = useState(item.externalDeviceId);
  const [typeKey, setTypeKey] = useState(item.suggestedTypeKey ?? "");
  return <article className="rounded-lg border p-3">
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium">{item.externalDeviceId}</span>
          <Badge variant={item.status === "conflicted" ? "destructive" : item.status === "managed" ? "secondary" : "outline"}>{DISCOVERY_STATUS_LABELS[item.status]}</Badge>
          {item.matchConfidence !== null && <Badge variant="outline">匹配 {Math.round(item.matchConfidence * 100)}%</Badge>}
        </div>
        <p className="mt-1 text-xs text-muted-foreground">{item.connectorName} · {item.externalDeviceType ?? "未知类型"}{item.parentExternalId ? ` · 上级 ${item.parentExternalId}` : ""}</p>
        <p className="mt-1 text-xs text-muted-foreground">建议类型：{item.suggestedTypeName ?? "未匹配"} · 最近发现 {new Date(item.lastSeenAt).toLocaleString("zh-CN")}</p>
      </div>
      {item.status === "conflicted" && <AlertTriangleIcon className="size-4 text-destructive" />}
    </div>
    {canManage && canConfirmDiscovery(item.status) && <div className="mt-3 grid gap-2 border-t pt-3 sm:grid-cols-[minmax(140px,1fr)_minmax(180px,1fr)_auto]">
      <Input aria-label="设备名称" onChange={(event) => setName(event.target.value)} value={name} />
      <select aria-label="DeviceType" className="h-9 rounded-md border bg-background px-3 text-sm" onChange={(event) => setTypeKey(event.target.value)} value={typeKey}>
        <option value="">选择 DeviceType</option>
        {deviceTypes.map((type) => <option key={type.id} value={type.typeKey}>{type.displayName} · {type.category}</option>)}
      </select>
      <Button disabled={busy || !name.trim() || !typeKey} onClick={() => act(
        `/api/projects/${projectId}/device-adapters/discoveries/${item.id}/bind`, { name, deviceTypeKey: typeKey }
      )} size="sm"><CheckIcon />确认纳管</Button>
    </div>}
    {canManage && item.status !== "managed" && <div className="mt-2 flex flex-wrap justify-end gap-2">
      <Button disabled={busy} onClick={() => act(`/api/projects/${projectId}/device-adapters/discoveries/${item.id}`, { action: "rematch" })} size="sm" variant="ghost"><RefreshCwIcon />重新匹配</Button>
      {item.status !== "ignored" && <Button disabled={busy} onClick={() => act(`/api/projects/${projectId}/device-adapters/discoveries/${item.id}`, { action: "ignore" })} size="sm" variant="ghost"><XIcon />忽略</Button>}
      {item.status === "ignored" && <Button disabled={busy} onClick={() => act(`/api/projects/${projectId}/device-adapters/discoveries/${item.id}`, { action: "review" })} size="sm" variant="ghost">恢复待确认</Button>}
    </div>}
  </article>;
}

export function DeviceDiscoveryManager({ projectId, discoveries, deviceTypes, connectors, canManage }: Props) {
  const router = useRouter();
  const [status, setStatus] = useState<DiscoveryStatus | "all">("all");
  const [connectorId, setConnectorId] = useState("all");
  const [query, setQuery] = useState("");
  const [busyKey, setBusyKey] = useState("");
  const [error, setError] = useState("");
  const filtered = useMemo(() => filterDiscoveries(discoveries, { status, connectorId, query }), [discoveries, status, connectorId, query]);
  async function act(url: string, body: unknown, method = "PATCH") {
    setBusyKey(url); setError("");
    try {
      const response = await fetch(url, { method, headers: { "content-type": "application/json" }, body: JSON.stringify(body) });
      const result = await response.json() as { error?: string };
      if (!response.ok) throw new Error(result.error ?? "操作失败");
      router.refresh();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "操作失败");
    } finally { setBusyKey(""); }
  }
  return <section className="mb-5 space-y-3 rounded-xl border bg-card p-4" aria-label="设备发现管理器">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div><h2 className="font-semibold">设备发现</h2><p className="text-xs text-muted-foreground">跨连接器核对来源、类型匹配和纳管状态</p></div>
      {canManage && <div className="flex flex-wrap gap-2">{connectors.filter((item) => item.canScan).map((item) => <Button disabled={Boolean(busyKey)} key={item.id}
        onClick={() => act(`/api/projects/${projectId}/device-adapters/${item.id}/scan`, {}, "POST")} size="sm" variant="outline"><ScanSearchIcon />扫描 {item.name}</Button>)}</div>}
    </div>
    <div className="flex flex-wrap gap-2">{STATUS_ORDER.map((item) => <Button key={item} onClick={() => setStatus(item)} size="sm" variant={status === item ? "secondary" : "ghost"}>{item === "all" ? `全部 ${discoveries.length}` : `${DISCOVERY_STATUS_LABELS[item]} ${discoveries.filter((row) => row.status === item).length}`}</Button>)}</div>
    <div className="grid gap-2 sm:grid-cols-[minmax(180px,1fr)_220px]">
      <Input aria-label="搜索发现对象" onChange={(event) => setQuery(event.target.value)} placeholder="搜索外部 ID、类型或连接器" value={query} />
      <select aria-label="筛选连接器" className="h-9 rounded-md border bg-background px-3 text-sm" onChange={(event) => setConnectorId(event.target.value)} value={connectorId}>
        <option value="all">全部连接器</option>{connectors.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
      </select>
    </div>
    {error && <p className="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
    {filtered.length ? <div className="grid gap-2 xl:grid-cols-2">{filtered.map((item) => <DiscoveryRow act={(url, body) => act(url, body, url.endsWith("/bind") ? "POST" : "PATCH")}
      busy={busyKey.includes(`/discoveries/${item.id}`)} canManage={canManage} deviceTypes={deviceTypes} item={item} key={item.id} projectId={projectId} />)}</div>
      : <div className="rounded-lg border border-dashed p-5 text-center text-sm text-muted-foreground">没有匹配的发现对象</div>}
    {!canManage && <p className="text-xs text-muted-foreground">当前角色可查看设备来源与状态；扫描和纳管操作仅限项目管理员。</p>}
  </section>;
}
