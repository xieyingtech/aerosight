"use client";

import { useEffect, useRef, useState } from "react";
import { ExternalLinkIcon, KeyRoundIcon, Loader2Icon, RefreshCwIcon, ShieldCheckIcon, UnplugIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { FlightHubProject } from "@/lib/dji-flighthub-client-core";
import {
  defaultFlightHubProjectSelection,
  discoveryStatusLabel,
  flightHubErrorMessage,
  flightHubReadOnlyCapabilities,
  flightHubStatusLabel,
  flightHubUnavailableActions,
} from "@/lib/dji-flighthub-ui-core";
import {
  capabilityStatusLabels,
  connectorDiagnosticHealth,
  evidenceLevelLabels,
  type FlightHubDiagnosticsPayload,
} from "@/lib/dji-flighthub-diagnostics-ui-core";

export type ConnectorSummary = {
  id: string;
  name: string;
  status: string;
  projectUuid: string;
  projectName: string;
  lastErrorCode: string | null;
  lastValidatedAt: string | Date | null;
  lastSyncAt: string | Date | null;
  lastSyncStatus: string | null;
  discoveredCount: number;
  managedCount: number;
  missingCount: number;
  createdAt: string | Date;
  updatedAt: string | Date;
};

export type OtherConnectorSummary = {
  id: string;
  name: string;
  status: string;
  typeLabel: string;
  version: string;
  lastCheckedAt: string | Date | null;
};

export type DiscoveryIdentity = {
  id: string;
  connectorId: string;
  connectorName: string;
  externalDeviceId: string;
  externalDeviceType: string | null;
  serialNumber: string | null;
  callsign: string | null;
  parentExternalId: string | null;
  discoveryStatus: "discovered" | "managed" | "ignored" | "conflicted" | "missing";
  sourceVersion: string | null;
  deviceId: number | null;
  lastSeenAt: string | Date;
};

export type SyncRun = {
  id: string;
  connectorId: string;
  connectorName: string;
  status: "pending" | "running" | "succeeded" | "failed" | "cancelled";
  discoveredCount: number;
  managedCount: number;
  missingCount: number;
  errorCode: string | null;
  startedAt: string | Date | null;
  finishedAt: string | Date | null;
  createdAt: string | Date;
};

type PageData = { connectors: ConnectorSummary[]; identities: DiscoveryIdentity[]; syncRuns: SyncRun[] };
type SafeErrorResponse = { error?: { code?: string } };

const consoleUrl = "https://fh.dji.com";

function formatDate(value: string | Date | null) {
  if (!value) return "—";
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.valueOf())) return "—";
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "short", timeStyle: "short" }).format(date);
}

function statusVariant(status: string): "default" | "secondary" | "destructive" | "outline" {
  if (status === "connected" || status === "succeeded" || status === "managed") return "default";
  if (status === "failed" || status === "conflicted") return "destructive";
  if (status === "degraded" || status === "missing") return "secondary";
  return "outline";
}

export function DjiFlightHubSetup({ projectId, enabled, onCreated }: { projectId: number; enabled: boolean; onCreated: () => void }) {
  const [phase, setPhase] = useState<"token" | "project">("token");
  const [token, setToken] = useState("");
  const [projects, setProjects] = useState<FlightHubProject[]>([]);
  const [selectedProject, setSelectedProject] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const tokenInputRef = useRef<HTMLInputElement>(null);

  const clearTransientToken = () => {
    setToken("");
    setProjects([]);
    setSelectedProject("");
    setPhase("token");
    if (tokenInputRef.current) tokenInputRef.current.value = "";
  };

  useEffect(() => () => {
    if (tokenInputRef.current) tokenInputRef.current.value = "";
  }, []);

  const discoverProjects = async () => {
    if (!token.trim()) return;
    setBusy(true);
    setError(null);
    try {
      const response = await fetch(`/api/projects/${projectId}/connectors/dji-flighthub/projects`, {
        method: "POST", headers: { "content-type": "application/json" }, cache: "no-store", body: JSON.stringify({ token }),
      });
      const data = await response.json() as { projects?: FlightHubProject[] } & SafeErrorResponse;
      if (!response.ok) throw new Error(data.error?.code ?? "upstream_error");
      const discovered = data.projects ?? [];
      setProjects(discovered);
      setSelectedProject(defaultFlightHubProjectSelection(discovered));
      setPhase("project");
    } catch (cause) {
      clearTransientToken();
      setError(flightHubErrorMessage(cause instanceof Error ? cause.message : undefined));
    } finally {
      setBusy(false);
    }
  };

  const createConnection = async () => {
    if (!selectedProject) return;
    setBusy(true);
    setError(null);
    try {
      const response = await fetch(`/api/projects/${projectId}/connectors/dji-flighthub`, {
        method: "POST", headers: { "content-type": "application/json" }, cache: "no-store", body: JSON.stringify({ token, projectUuid: selectedProject }),
      });
      const data = await response.json() as SafeErrorResponse;
      if (!response.ok) throw new Error(data.error?.code ?? "upstream_error");
      clearTransientToken();
      onCreated();
    } catch (cause) {
      clearTransientToken();
      setError(flightHubErrorMessage(cause instanceof Error ? cause.message : undefined));
    } finally {
      setBusy(false);
    }
  };

  return <div className="space-y-4">
    <div className="flex flex-wrap items-start justify-between gap-3">
      <p className="max-w-2xl text-sm text-muted-foreground">
        输入组织 Token 后读取可访问项目并完成连接。此通道仅提供{flightHubReadOnlyCapabilities.join("、")}；不开放{flightHubUnavailableActions.join("、")}。
      </p>
      <Button asChild size="sm" variant="outline"><a href={consoleUrl} rel="noreferrer" target="_blank">打开司空 2 <ExternalLinkIcon /></a></Button>
    </div>
    <div className="rounded-lg bg-muted/35 p-3 text-sm text-muted-foreground">
      在司空中进入“我的组织 → 组织设置 → OpenAPI → 复制密钥”。这不是 OAuth；Token 只存在于当前向导内存，连接后加密保存。
    </div>
    {!enabled ? <div className="rounded-lg border border-dashed p-3 text-sm text-muted-foreground">
      当前部署尚未启用司空连接器。请先设置 <code>DJI_FLIGHTHUB_ENABLED=true</code>。
    </div> : <div className="space-y-3 rounded-lg border p-3">
      <div className="text-xs text-muted-foreground">步骤 {phase === "token" ? "1/2 · 验证 Token" : "2/2 · 选择司空项目"}</div>
      {phase === "token" ? <div className="grid gap-3 md:grid-cols-[1fr_auto]">
        <label className="space-y-1 text-sm">司空组织 Token
          <Input ref={tokenInputRef} autoComplete="new-password" onChange={(event) => setToken(event.target.value)} placeholder="仅在当前向导内临时使用" type="password" value={token} />
        </label>
        <Button className="self-end" disabled={busy || !token.trim()} onClick={() => void discoverProjects()} type="button">
          {busy ? <Loader2Icon className="animate-spin" /> : <KeyRoundIcon />}验证并获取项目
        </Button>
      </div> : <div className="space-y-3">
        {projects.length === 0 ? <p className="rounded-md bg-muted p-3 text-sm text-muted-foreground">这个 Token 当前没有可访问项目。请在司空中确认组织与项目权限。</p> : <label className="block space-y-1 text-sm">选择司空项目
          <select className="flex h-9 w-full rounded-md border bg-transparent px-3 text-sm" onChange={(event) => setSelectedProject(event.target.value)} value={selectedProject}>
            <option value="">请选择项目</option>
            {projects.map((project) => <option key={project.uuid} value={project.uuid}>{project.name}</option>)}
          </select>
        </label>}
        <div className="flex gap-2">
          <Button disabled={busy || !selectedProject} onClick={() => void createConnection()} type="button">{busy && <Loader2Icon className="animate-spin" />}连接所选项目</Button>
          <Button disabled={busy} onClick={() => { clearTransientToken(); setError(null); }} type="button" variant="outline">重新输入 Token</Button>
        </div>
      </div>}
    </div>}
    {error && <div className="rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive" role="alert">{error}</div>}
  </div>;
}

export function DjiFlightHubConnections({
  projectId, initialConnectors, initialIdentities, initialSyncRuns, otherConnectors,
}: {
  projectId: number;
  initialConnectors: ConnectorSummary[];
  initialIdentities: DiscoveryIdentity[];
  initialSyncRuns: SyncRun[];
  otherConnectors: OtherConnectorSummary[];
}) {
  const [connectors, setConnectors] = useState(initialConnectors);
  const [identities, setIdentities] = useState(initialIdentities);
  const [syncRuns, setSyncRuns] = useState(initialSyncRuns);
  const [selectedConnectorId, setSelectedConnectorId] = useState<string | null>(null);
  const [updateTokens, setUpdateTokens] = useState<Record<string, string>>({});
  const [busyAction, setBusyAction] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [diagnostics, setDiagnostics] = useState<Record<string, FlightHubDiagnosticsPayload>>({});
  const [diagnosticsLoading, setDiagnosticsLoading] = useState(false);

  useEffect(() => {
    setConnectors(initialConnectors);
    setIdentities(initialIdentities);
    setSyncRuns(initialSyncRuns);
  }, [initialConnectors, initialIdentities, initialSyncRuns]);

  const refresh = async () => {
    const response = await fetch(`/api/projects/${projectId}/connectors/dji-flighthub`, { cache: "no-store" });
    if (!response.ok) return;
    const data = await response.json() as PageData;
    setConnectors(data.connectors);
    setIdentities(data.identities);
    setSyncRuns(data.syncRuns);
  };

  const loadDiagnostics = async (connectorId: string) => {
    setDiagnosticsLoading(true);
    try {
      const response = await fetch(`/api/projects/${projectId}/connectors/dji-flighthub/${connectorId}/diagnostics`, { cache: "no-store" });
      if (!response.ok) return;
      const data = await response.json() as FlightHubDiagnosticsPayload;
      setDiagnostics((current) => ({ ...current, [connectorId]: data }));
    } finally {
      setDiagnosticsLoading(false);
    }
  };

  useEffect(() => {
    if (selectedConnectorId) void loadDiagnostics(selectedConnectorId);
  // loadDiagnostics intentionally follows the selected connector only.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedConnectorId]);

  const runConnectorAction = async (connectorId: string, action: "sync" | "disconnect" | "reconnect") => {
    if (action === "disconnect" && !window.confirm("断开后会停止新同步，但保留设备、身份和审计历史。确认断开？")) return;
    setBusyAction(`${action}:${connectorId}`); setError(null); setNotice(null);
    try {
      const response = await fetch(`/api/projects/${projectId}/connectors/dji-flighthub/${connectorId}${action === "sync" ? "/sync" : ""}`, {
        method: action === "sync" ? "POST" : action === "reconnect" ? "PUT" : "DELETE", cache: "no-store",
      });
      const data = await response.json() as SafeErrorResponse & { deduplicated?: boolean };
      if (!response.ok) throw new Error(data.error?.code ?? "upstream_error");
      setNotice(action === "sync"
        ? (data.deduplicated ? "已有同步请求正在等待或执行，本次已自动合并。" : "同步请求已进入队列。")
        : action === "reconnect"
          ? "连接器正在重新连接，历史设备绑定已恢复，只读同步已进入队列。"
          : "连接器已断开；历史设备与审计记录仍然保留。");
      await refresh();
    } catch (cause) {
      setError(flightHubErrorMessage(cause instanceof Error ? cause.message : undefined));
    } finally {
      setBusyAction(null);
    }
  };

  const updateToken = async (connectorId: string) => {
    const replacement = updateTokens[connectorId]?.trim() ?? "";
    if (!replacement) return;
    setBusyAction(`token:${connectorId}`); setError(null); setNotice(null);
    setUpdateTokens((current) => ({ ...current, [connectorId]: "" }));
    try {
      const response = await fetch(`/api/projects/${projectId}/connectors/dji-flighthub/${connectorId}/token`, {
        method: "PUT", headers: { "content-type": "application/json" }, cache: "no-store", body: JSON.stringify({ token: replacement }),
      });
      const data = await response.json() as SafeErrorResponse;
      if (!response.ok) throw new Error(data.error?.code ?? "upstream_error");
      setNotice("Token 已验证并替换，重新同步已进入队列。输入内容已清除。");
      await refresh();
    } catch (cause) {
      setError(flightHubErrorMessage(cause instanceof Error ? cause.message : undefined));
    } finally {
      setUpdateTokens((current) => ({ ...current, [connectorId]: "" }));
      setBusyAction(null);
    }
  };

  const reprobeCapabilities = async (connectorId: string) => {
    setBusyAction(`probe:${connectorId}`); setError(null); setNotice(null);
    try {
      const response = await fetch(`/api/projects/${projectId}/connectors/dji-flighthub/${connectorId}/diagnostics`, {
        method: "POST", cache: "no-store",
      });
      const data = await response.json() as SafeErrorResponse & { deduplicated?: boolean };
      if (!response.ok) throw new Error(data.error?.code ?? "upstream_error");
      setNotice(data.deduplicated ? "只读能力探测已合并到等待中的同步。" : "只读能力探测已进入队列；只会调用官方 GET 接口。");
      await loadDiagnostics(connectorId);
    } catch (cause) {
      setError(flightHubErrorMessage(cause instanceof Error ? cause.message : undefined));
    } finally {
      setBusyAction(null);
    }
  };

  const selectedConnector = connectors.find((connector) => connector.id === selectedConnectorId) ?? null;
  const selectedIdentities = identities.filter((identity) => identity.connectorId === selectedConnectorId);
  const selectedSyncRuns = syncRuns.filter((run) => run.connectorId === selectedConnectorId);
  const selectedDiagnostics = selectedConnectorId ? diagnostics[selectedConnectorId] ?? null : null;
  const diagnosticHealth = selectedDiagnostics ? connectorDiagnosticHealth(selectedDiagnostics) : null;
  const totalCount = connectors.length + otherConnectors.length;

  return <div className="space-y-5">
    <div className="overflow-hidden rounded-xl border">
      <Table>
        <TableHeader><TableRow><TableHead>名称</TableHead><TableHead>类型</TableHead><TableHead>状态</TableHead><TableHead>最近同步 / 检查</TableHead><TableHead>设备</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
        <TableBody>
          {connectors.map((connector) => <TableRow key={connector.id}>
            <TableCell><p className="font-medium">{connector.projectName || connector.name}</p><p className="text-xs text-muted-foreground">司空项目</p></TableCell>
            <TableCell>DJI 司空 2</TableCell>
            <TableCell><Badge variant={statusVariant(connector.status)}>{flightHubStatusLabel(connector.status)}</Badge></TableCell>
            <TableCell>{formatDate(connector.lastSyncAt)}</TableCell>
            <TableCell>{connector.discoveredCount + connector.managedCount + connector.missingCount}</TableCell>
            <TableCell className="text-right"><Button onClick={() => setSelectedConnectorId(connector.id)} size="sm" type="button" variant="outline">管理</Button></TableCell>
          </TableRow>)}
          {otherConnectors.map((connector) => <TableRow key={connector.id}>
            <TableCell><p className="font-medium">{connector.name}</p><p className="text-xs text-muted-foreground">历史连接器</p></TableCell>
            <TableCell>{connector.typeLabel}</TableCell>
            <TableCell><Badge variant={statusVariant(connector.status)}>{flightHubStatusLabel(connector.status)}</Badge></TableCell>
            <TableCell>{formatDate(connector.lastCheckedAt)}</TableCell><TableCell>—</TableCell>
            <TableCell className="text-right text-xs text-muted-foreground">暂不开放现场配置</TableCell>
          </TableRow>)}
          {totalCount === 0 && <TableRow><TableCell className="h-28 text-center text-muted-foreground" colSpan={6}>暂无连接器。点击右上角“新建连接器”开始接入。</TableCell></TableRow>}
        </TableBody>
      </Table>
    </div>

    {error && <div className="rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive" role="alert">{error}</div>}
    {notice && <div className="rounded-lg border p-3 text-sm" role="status">{notice}</div>}

    {selectedConnector && <section className="space-y-4 rounded-xl border p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div><div className="flex items-center gap-2"><h2 className="font-medium">{selectedConnector.projectName}</h2><Badge variant={statusVariant(selectedConnector.status)}>{flightHubStatusLabel(selectedConnector.status)}</Badge></div><p className="mt-1 text-xs text-muted-foreground">DJI 司空 2 · 项目 UUID {selectedConnector.projectUuid} · 最近验证 {formatDate(selectedConnector.lastValidatedAt)}</p></div>
        <div className="flex flex-wrap gap-2">
          {selectedConnector.status === "disabled" && <Button disabled={busyAction !== null} onClick={() => void runConnectorAction(selectedConnector.id, "reconnect")} size="sm" type="button" variant="default"><RefreshCwIcon />重新连接</Button>}
          <Button disabled={busyAction !== null || selectedConnector.status === "disabled"} onClick={() => void runConnectorAction(selectedConnector.id, "sync")} size="sm" type="button" variant="outline"><RefreshCwIcon />立即同步</Button>
          <Button disabled={busyAction !== null || selectedConnector.status === "disabled"} onClick={() => void reprobeCapabilities(selectedConnector.id)} size="sm" type="button" variant="outline"><ShieldCheckIcon />只读重新探测</Button>
          <Button disabled={busyAction !== null || selectedConnector.status === "disabled"} onClick={() => void runConnectorAction(selectedConnector.id, "disconnect")} size="sm" type="button" variant="destructive"><UnplugIcon />断开</Button>
        </div>
      </div>
      <div className="grid gap-2 text-xs sm:grid-cols-4">
        <div className="rounded-lg bg-muted/35 p-3"><span className="text-muted-foreground">发现</span><p className="mt-1 font-medium">{selectedConnector.discoveredCount}</p></div>
        <div className="rounded-lg bg-muted/35 p-3"><span className="text-muted-foreground">已纳管</span><p className="mt-1 font-medium">{selectedConnector.managedCount}</p></div>
        <div className="rounded-lg bg-muted/35 p-3"><span className="text-muted-foreground">缺失</span><p className="mt-1 font-medium">{selectedConnector.missingCount}</p></div>
        <div className="rounded-lg bg-muted/35 p-3"><span className="text-muted-foreground">健康</span><p className="mt-1 font-medium">{selectedConnector.lastErrorCode ?? selectedConnector.lastSyncStatus ?? "等待同步"}</p></div>
      </div>
      <section className="space-y-3 rounded-lg border p-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2"><h3 className="text-sm font-medium">能力与同步诊断</h3>{diagnosticHealth && <Badge variant={diagnosticHealth.status === "failed" ? "destructive" : diagnosticHealth.status === "degraded" ? "secondary" : "default"}>{diagnosticHealth.label}</Badge>}</div>
          <Button disabled={diagnosticsLoading} onClick={() => void loadDiagnostics(selectedConnector.id)} size="sm" type="button" variant="ghost"><RefreshCwIcon className={diagnosticsLoading ? "animate-spin" : ""} />刷新诊断</Button>
        </div>
        {!selectedDiagnostics ? <p className="text-xs text-muted-foreground">{diagnosticsLoading ? "正在读取能力证据…" : "暂无能力快照；可执行只读重新探测。"}</p> : <>
          <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
            {selectedDiagnostics.resourceWatermarks.map((watermark) => <div className="rounded-md bg-muted/35 p-2 text-xs" key={watermark.resourceKind}>
              <div className="flex items-center justify-between gap-2"><span className="font-medium">{watermark.resourceKind}</span><Badge variant={statusVariant(watermark.status)}>{watermark.status}</Badge></div>
              <p className="mt-1 text-muted-foreground">成功水位 {formatDate(watermark.lastSucceededAt)} · 尝试 {watermark.attemptCount}</p>
              {watermark.lastErrorCode && <p className="mt-1 text-destructive">{watermark.lastErrorCode} · 下次 {formatDate(watermark.nextAttemptAt)}</p>}
            </div>)}
            {selectedDiagnostics.resourceWatermarks.length === 0 && <p className="text-xs text-muted-foreground">暂无资源流水位。</p>}
          </div>
          <div className="overflow-x-auto rounded-md border"><Table>
            <TableHeader><TableRow><TableHead>能力</TableHead><TableHead>状态</TableHead><TableHead>验证证据</TableHead><TableHead>型号 / 固件</TableHead><TableHead>验证时间</TableHead><TableHead>说明</TableHead></TableRow></TableHeader>
            <TableBody>{selectedDiagnostics.capabilities.map((capability) => <TableRow key={`${capability.capabilityCode}:${capability.deviceModel ?? "all"}:${capability.firmwareVersion ?? "all"}`}>
              <TableCell className="font-mono text-xs">{capability.capabilityCode}</TableCell>
              <TableCell><Badge variant={capability.status === "failed" || capability.status === "forbidden" ? "destructive" : capability.status === "supported" || capability.status === "empty" ? "default" : "secondary"}>{capabilityStatusLabels[capability.status]}</Badge></TableCell>
              <TableCell><Badge variant="outline">{evidenceLevelLabels[capability.evidenceLevel] ?? capability.evidenceLevel}</Badge></TableCell>
              <TableCell className="text-xs">{capability.deviceModel ?? "全部"} / {capability.firmwareVersion ?? "全部"}</TableCell>
              <TableCell className="text-xs">{formatDate(capability.verifiedAt)}</TableCell>
              <TableCell className="max-w-64 text-xs text-muted-foreground">{capability.reason ?? "—"}</TableCell>
            </TableRow>)}
            {selectedDiagnostics.capabilities.length === 0 && <TableRow><TableCell className="text-center text-muted-foreground" colSpan={6}>暂无能力证据。</TableCell></TableRow>}</TableBody>
          </Table></div>
        </>}
      </section>
      <details><summary className="cursor-pointer text-sm">更新 Token</summary><div className="mt-2 flex flex-col gap-2 sm:flex-row">
        <Input autoComplete="new-password" onChange={(event) => setUpdateTokens((current) => ({ ...current, [selectedConnector.id]: event.target.value }))} placeholder="新 Token；提交后立即清除" type="password" value={updateTokens[selectedConnector.id] ?? ""} />
        <Button disabled={busyAction !== null || !(updateTokens[selectedConnector.id]?.trim())} onClick={() => void updateToken(selectedConnector.id)} size="sm" type="button">验证并替换</Button>
      </div></details>
      <div className="grid gap-4 xl:grid-cols-2">
        <div className="space-y-2"><h3 className="flex items-center gap-2 text-sm font-medium"><ShieldCheckIcon className="size-4" />设备候选</h3><div className="max-h-80 space-y-2 overflow-auto rounded-lg border p-2">
          {selectedIdentities.map((identity) => <div className="rounded-md bg-muted/30 p-2 text-xs" key={identity.id}>
            <div className="flex items-center justify-between gap-2"><span className="font-medium">{identity.callsign || identity.serialNumber || identity.externalDeviceId}</span><Badge variant={statusVariant(identity.discoveryStatus)}>{discoveryStatusLabel(identity.discoveryStatus)}</Badge></div>
            <p className="mt-1 text-muted-foreground">{identity.externalDeviceType ?? "未知型号"} · SN {identity.serialNumber ?? "—"} · {formatDate(identity.lastSeenAt)}</p>
            {identity.parentExternalId && <p className="mt-1 text-muted-foreground">上级：{identity.parentExternalId}</p>}
            {identity.discoveryStatus === "conflicted" && <p className="mt-1 text-destructive">同一 SN 存在其他来源；需人工确认，系统不会自动改变下行路由。</p>}
          </div>)}
          {selectedIdentities.length === 0 && <p className="p-2 text-xs text-muted-foreground">首次同步完成后会在这里显示候选设备。</p>}
        </div></div>
        <div className="space-y-2"><h3 className="text-sm font-medium">同步日志</h3><div className="max-h-80 space-y-2 overflow-auto rounded-lg border p-2">
          {selectedSyncRuns.map((run) => <div className="rounded-md bg-muted/30 p-2 text-xs" key={run.id}>
            <div className="flex items-center justify-between gap-2"><span className="font-medium">{run.connectorName}</span><Badge variant={statusVariant(run.status)}>{run.status}</Badge></div>
            <p className="mt-1 text-muted-foreground">发现 {run.discoveredCount} · 纳管 {run.managedCount} · 缺失 {run.missingCount} · {formatDate(run.finishedAt ?? run.createdAt)}</p>
            {run.errorCode && <p className="mt-1 text-destructive">{run.errorCode}</p>}
          </div>)}
          {selectedSyncRuns.length === 0 && <p className="p-2 text-xs text-muted-foreground">暂无同步记录。</p>}
        </div></div>
      </div>
    </section>}
  </div>;
}
