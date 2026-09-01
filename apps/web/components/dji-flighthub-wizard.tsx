"use client";

import { useEffect, useRef, useState } from "react";
import {
  CloudIcon,
  ExternalLinkIcon,
  KeyRoundIcon,
  Loader2Icon,
  RefreshCwIcon,
  ShieldCheckIcon,
  UnplugIcon,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { FlightHubProject } from "@/lib/dji-flighthub-client-core";
import {
  defaultFlightHubProjectSelection,
  discoveryStatusLabel,
  flightHubErrorMessage,
  flightHubReadOnlyCapabilities,
  flightHubStatusLabel,
  flightHubUnavailableActions,
} from "@/lib/dji-flighthub-ui-core";

type ConnectorSummary = {
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

type DiscoveryIdentity = {
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

type SyncRun = {
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

export function DjiFlightHubWizard({
  projectId,
  enabled,
  initialConnectors,
  initialIdentities,
  initialSyncRuns,
}: {
  projectId: number;
  enabled: boolean;
  initialConnectors: ConnectorSummary[];
  initialIdentities: DiscoveryIdentity[];
  initialSyncRuns: SyncRun[];
}) {
  const [phase, setPhase] = useState<"token" | "project">("token");
  const [token, setToken] = useState("");
  const [projects, setProjects] = useState<FlightHubProject[]>([]);
  const [selectedProject, setSelectedProject] = useState("");
  const [connectors, setConnectors] = useState(initialConnectors);
  const [identities, setIdentities] = useState(initialIdentities);
  const [syncRuns, setSyncRuns] = useState(initialSyncRuns);
  const [updateTokens, setUpdateTokens] = useState<Record<string, string>>({});
  const [busyAction, setBusyAction] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
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

  const readJson = async <T,>(response: Response): Promise<T> => response.json() as Promise<T>;

  const refresh = async () => {
    const response = await fetch(`/api/projects/${projectId}/connectors/dji-flighthub`, { cache: "no-store" });
    if (!response.ok) return;
    const data = await readJson<PageData>(response);
    setConnectors(data.connectors);
    setIdentities(data.identities);
    setSyncRuns(data.syncRuns);
  };

  const discoverProjects = async () => {
    if (!token.trim()) {
      setError("请先填写司空组织 Token。");
      return;
    }
    setBusyAction("discover"); setError(null); setNotice(null);
    try {
      const response = await fetch(`/api/projects/${projectId}/connectors/dji-flighthub/projects`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        cache: "no-store",
        body: JSON.stringify({ token }),
      });
      const data = await readJson<{ projects?: FlightHubProject[] } & SafeErrorResponse>(response);
      if (!response.ok) throw new Error(data.error?.code ?? "upstream_error");
      const discovered = data.projects ?? [];
      setProjects(discovered);
      setSelectedProject(defaultFlightHubProjectSelection(discovered));
      setPhase("project");
    } catch (cause) {
      const code = cause instanceof Error ? cause.message : undefined;
      clearTransientToken();
      setError(flightHubErrorMessage(code));
    } finally {
      setBusyAction(null);
    }
  };

  const createConnection = async () => {
    if (!selectedProject) {
      setError("请选择一个司空项目。");
      return;
    }
    setBusyAction("create"); setError(null); setNotice(null);
    try {
      const response = await fetch(`/api/projects/${projectId}/connectors/dji-flighthub`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        cache: "no-store",
        body: JSON.stringify({ token, projectUuid: selectedProject }),
      });
      const data = await readJson<SafeErrorResponse>(response);
      if (!response.ok) throw new Error(data.error?.code ?? "upstream_error");
      clearTransientToken();
      setNotice("司空项目已连接，首次设备目录同步已进入队列。Token 已从表单清除。 ");
      await refresh();
    } catch (cause) {
      const code = cause instanceof Error ? cause.message : undefined;
      clearTransientToken();
      setError(flightHubErrorMessage(code));
    } finally {
      setBusyAction(null);
    }
  };

  const runConnectorAction = async (connectorId: string, action: "sync" | "disconnect") => {
    if (action === "disconnect" && !window.confirm("断开后会停止新同步，但保留设备、身份和审计历史。确认断开？")) return;
    setBusyAction(`${action}:${connectorId}`); setError(null); setNotice(null);
    try {
      const response = await fetch(
        `/api/projects/${projectId}/connectors/dji-flighthub/${connectorId}${action === "sync" ? "/sync" : ""}`,
        { method: action === "sync" ? "POST" : "DELETE", cache: "no-store" }
      );
      const data = await readJson<SafeErrorResponse & { deduplicated?: boolean }>(response);
      if (!response.ok) throw new Error(data.error?.code ?? "upstream_error");
      setNotice(action === "sync"
        ? (data.deduplicated ? "已有同步请求正在等待或执行，本次已自动合并。" : "同步请求已进入队列。")
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
    if (!replacement) {
      setError("请填写新的 Token。");
      return;
    }
    setBusyAction(`token:${connectorId}`); setError(null); setNotice(null);
    setUpdateTokens((current) => ({ ...current, [connectorId]: "" }));
    try {
      const response = await fetch(`/api/projects/${projectId}/connectors/dji-flighthub/${connectorId}/token`, {
        method: "PUT", headers: { "content-type": "application/json" }, cache: "no-store",
        body: JSON.stringify({ token: replacement }),
      });
      const data = await readJson<SafeErrorResponse>(response);
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

  return <section className="space-y-5 rounded-xl border p-4">
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h2 className="flex items-center gap-2 font-medium"><CloudIcon className="size-4" />DJI 司空 2 公有云</h2>
        <p className="mt-1 max-w-3xl text-sm text-muted-foreground">
          填写 Token 后自动读取可访问项目，选择项目即可连接。此通道只同步目录与状态；{flightHubUnavailableActions.join("、")}仍需使用 DJI Cloud API 直连。
        </p>
      </div>
      <Button asChild size="sm" variant="outline">
        <a href={consoleUrl} rel="noreferrer" target="_blank">打开司空 2 <ExternalLinkIcon /></a>
      </Button>
    </div>

    <div className="grid gap-3 md:grid-cols-2">
      <div className="rounded-lg bg-muted/35 p-3 text-sm">
        <p className="font-medium">可用能力</p>
        <p className="mt-1 text-muted-foreground">{flightHubReadOnlyCapabilities.join("、")} · 中国大陆公有云 · HTTPS 出站</p>
      </div>
      <div className="rounded-lg bg-muted/35 p-3 text-sm">
        <p className="font-medium">Token 安全</p>
        <p className="mt-1 text-muted-foreground">在司空中进入“我的组织 → 组织设置 → OpenAPI → 复制密钥”。这不是 OAuth；Token 不会进入跳转链接、URL、浏览器存储或页面源码，连接后以加密 envelope 保存。</p>
      </div>
    </div>

    {!enabled ? <div className="rounded-lg border border-dashed p-3 text-sm text-muted-foreground">
      当前部署尚未启用司空连接器。运维人员需要同时为 Web 与 Worker 设置 <code>DJI_FLIGHTHUB_ENABLED=true</code>。
    </div> : <div className="space-y-3 rounded-lg border p-3">
      <div className="text-xs text-muted-foreground">步骤 {phase === "token" ? "1/2 · 验证 Token" : "2/2 · 选择司空项目"}</div>
      {phase === "token" ? <div className="grid gap-3 md:grid-cols-[1fr_auto]">
        <label className="space-y-1 text-sm">司空组织 Token
          <Input
            ref={tokenInputRef}
            autoComplete="new-password"
            onChange={(event) => setToken(event.target.value)}
            placeholder="仅在当前向导内临时使用"
            type="password"
            value={token}
          />
        </label>
        <Button className="self-end" disabled={busyAction !== null || !token.trim()} onClick={() => void discoverProjects()} type="button">
          {busyAction === "discover" ? <Loader2Icon className="animate-spin" /> : <KeyRoundIcon />}验证并获取项目
        </Button>
      </div> : <div className="space-y-3">
        {projects.length === 0 ? <p className="rounded-md bg-muted p-3 text-sm text-muted-foreground">这个 Token 当前没有可访问项目。请在司空中确认组织与项目权限。</p> : <label className="block space-y-1 text-sm">选择司空项目
          <select className="flex h-9 w-full rounded-md border bg-transparent px-3 text-sm" onChange={(event) => setSelectedProject(event.target.value)} value={selectedProject}>
            <option value="">请选择项目</option>
            {projects.map((project) => <option key={project.uuid} value={project.uuid}>{project.name}</option>)}
          </select>
        </label>}
        <div className="flex gap-2">
          <Button disabled={busyAction !== null || !selectedProject} onClick={() => void createConnection()} type="button">
            {busyAction === "create" && <Loader2Icon className="animate-spin" />}连接所选项目
          </Button>
          <Button disabled={busyAction !== null} onClick={() => { clearTransientToken(); setError(null); }} type="button" variant="outline">取消并清除 Token</Button>
        </div>
      </div>}
    </div>}

    {error && <div className="rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive" role="alert">{error}</div>}
    {notice && <div className="rounded-lg border p-3 text-sm" role="status">{notice}</div>}

    <div className="space-y-3">
      <h3 className="text-sm font-medium">已连接的司空项目</h3>
      {connectors.map((connector) => <article className="space-y-3 rounded-lg bg-muted/30 p-3" key={connector.id}>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <div className="flex items-center gap-2"><p className="text-sm font-medium">{connector.projectName}</p><Badge variant={statusVariant(connector.status)}>{flightHubStatusLabel(connector.status)}</Badge></div>
            <p className="mt-1 text-xs text-muted-foreground">项目 UUID {connector.projectUuid} · 最近同步 {formatDate(connector.lastSyncAt)}</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button disabled={busyAction !== null || connector.status === "disabled"} onClick={() => void runConnectorAction(connector.id, "sync")} size="sm" type="button" variant="outline"><RefreshCwIcon />立即同步</Button>
            <Button disabled={busyAction !== null || connector.status === "disabled"} onClick={() => void runConnectorAction(connector.id, "disconnect")} size="sm" type="button" variant="destructive"><UnplugIcon />断开</Button>
          </div>
        </div>
        <div className="grid gap-2 text-xs sm:grid-cols-4">
          <div><span className="text-muted-foreground">发现</span><p className="mt-1 font-medium">{connector.discoveredCount}</p></div>
          <div><span className="text-muted-foreground">已纳管</span><p className="mt-1 font-medium">{connector.managedCount}</p></div>
          <div><span className="text-muted-foreground">缺失</span><p className="mt-1 font-medium">{connector.missingCount}</p></div>
          <div><span className="text-muted-foreground">健康</span><p className="mt-1 font-medium">{connector.lastErrorCode ?? connector.lastSyncStatus ?? "等待同步"}</p></div>
        </div>
        <details>
          <summary className="cursor-pointer text-sm">更新 Token</summary>
          <div className="mt-2 flex flex-col gap-2 sm:flex-row">
            <Input autoComplete="new-password" onChange={(event) => setUpdateTokens((current) => ({ ...current, [connector.id]: event.target.value }))} placeholder="新 Token；提交后立即清除" type="password" value={updateTokens[connector.id] ?? ""} />
            <Button disabled={busyAction !== null || !(updateTokens[connector.id]?.trim())} onClick={() => void updateToken(connector.id)} size="sm" type="button">验证并替换</Button>
          </div>
        </details>
      </article>)}
      {connectors.length === 0 && <p className="text-sm text-muted-foreground">尚未连接司空项目。</p>}
    </div>

    <div className="grid gap-4 xl:grid-cols-2">
      <div className="space-y-2">
        <h3 className="flex items-center gap-2 text-sm font-medium"><ShieldCheckIcon className="size-4" />设备候选</h3>
        <div className="max-h-80 space-y-2 overflow-auto rounded-lg border p-2">
          {identities.map((identity) => <div className="rounded-md bg-muted/30 p-2 text-xs" key={identity.id}>
            <div className="flex items-center justify-between gap-2"><span className="font-medium">{identity.callsign || identity.serialNumber || identity.externalDeviceId}</span><Badge variant={statusVariant(identity.discoveryStatus)}>{discoveryStatusLabel(identity.discoveryStatus)}</Badge></div>
            <p className="mt-1 text-muted-foreground">{identity.externalDeviceType ?? "未知型号"} · SN {identity.serialNumber ?? "—"} · {formatDate(identity.lastSeenAt)}</p>
            {identity.parentExternalId && <p className="mt-1 text-muted-foreground">上级：{identity.parentExternalId}</p>}
            {identity.discoveryStatus === "conflicted" && <p className="mt-1 text-destructive">同一 SN 存在其他来源；需人工确认，系统不会自动改变下行路由。</p>}
          </div>)}
          {identities.length === 0 && <p className="p-2 text-xs text-muted-foreground">首次同步完成后会在这里显示候选设备。</p>}
        </div>
      </div>
      <div className="space-y-2">
        <h3 className="text-sm font-medium">同步日志</h3>
        <div className="max-h-80 space-y-2 overflow-auto rounded-lg border p-2">
          {syncRuns.map((run) => <div className="rounded-md bg-muted/30 p-2 text-xs" key={run.id}>
            <div className="flex items-center justify-between gap-2"><span className="font-medium">{run.connectorName}</span><Badge variant={statusVariant(run.status)}>{run.status}</Badge></div>
            <p className="mt-1 text-muted-foreground">发现 {run.discoveredCount} · 纳管 {run.managedCount} · 缺失 {run.missingCount} · {formatDate(run.finishedAt ?? run.createdAt)}</p>
            {run.errorCode && <p className="mt-1 text-destructive">{run.errorCode}</p>}
          </div>)}
          {syncRuns.length === 0 && <p className="p-2 text-xs text-muted-foreground">暂无同步记录。</p>}
        </div>
      </div>
    </div>
  </section>;
}
