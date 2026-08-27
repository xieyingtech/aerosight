"use client";

import { useState } from "react";
import { CheckCircle2Icon, Loader2Icon, NetworkIcon, ShieldCheckIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

type AdapterSummary = {
  id: string;
  name: string;
  adapterType: string;
  protocolVersion: string;
  status: string;
  hasSecret: boolean;
  lastHealth: Record<string, unknown>;
  lastCheckedAt: string | Date | null;
};

type SetupIssue = { field: string; code: string };
type ConfigurationSummary = Record<string, unknown>;

export function DjiAdapterWizard({ projectId, initialAdapters }: { projectId: number; initialAdapters: AdapterSummary[] }) {
  const [step, setStep] = useState<1 | 2>(1);
  const [mode, setMode] = useState<"lan" | "public">("lan");
  const [adapters, setAdapters] = useState(initialAdapters.filter((adapter) => adapter.adapterType === "dji"));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [issues, setIssues] = useState<SetupIssue[]>([]);
  const [testResult, setTestResult] = useState<Record<string, unknown> | null>(null);
  const [configurationSummary, setConfigurationSummary] = useState<ConfigurationSummary | null>(null);

  const submit = async (form: HTMLFormElement) => {
    setBusy(true); setError(null); setIssues([]); setTestResult(null);
    const values = new FormData(form);
    const response = await fetch(`/api/projects/${projectId}/device-adapters/dji-setup`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        name: values.get("name"), mode,
        mqttEndpoint: values.get("mqttEndpoint"),
        apiPublicBaseUrl: values.get("apiPublicBaseUrl"),
        websocketPublicUrl: values.get("websocketPublicUrl"),
        mediaIngestBaseUrl: values.get("mediaIngestBaseUrl"),
        mediaPlaybackBaseUrl: values.get("mediaPlaybackBaseUrl"),
        tlsRequired: mode === "public" || values.get("tlsRequired") === "on",
        mqttAnonymous: false,
        secretRef: values.get("secretRef"),
        ntpServerHost: values.get("ntpServerHost"),
        ntpServerPort: Number(values.get("ntpServerPort")),
        gatewaySerials: String(values.get("gatewaySerials") ?? "").split(/[\s,]+/).filter(Boolean)
      })
    });
    const result = await response.json() as AdapterSummary & { error?: string; issues?: SetupIssue[]; configurationSummary?: ConfigurationSummary };
    setBusy(false);
    if (!response.ok) {
      setError(result.error === "NETWORK_PROFILE_INVALID" ? "网络配置未通过安全策略" : (result.error ?? "创建失败"));
      setIssues(result.issues ?? []);
      return;
    }
    setAdapters((current) => [...current, result]);
    setConfigurationSummary(result.configurationSummary ?? null);
    form.reset(); setStep(1);
  };

  const testConnection = async (adapter: AdapterSummary) => {
    setBusy(true); setError(null); setTestResult(null);
    const response = await fetch(`/api/projects/${projectId}/device-adapters/${adapter.id}/test`, { method: "POST" });
    const result = await response.json() as Record<string, unknown> & { error?: string };
    setBusy(false);
    if (!response.ok) { setError(result.error ?? "自检失败"); return; }
    setTestResult(result);
  };

  return <section className="space-y-5 rounded-xl border p-4">
    <div>
      <h2 className="flex items-center gap-2 font-medium"><NetworkIcon className="size-4" />DJI Cloud API 接入</h2>
      <p className="mt-1 text-sm text-muted-foreground">配置 Dock 2/3 共用的 MQTT、应用 API 与媒体网关。秘密只保存引用，页面不会回显凭据。</p>
    </div>

    <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); void submit(event.currentTarget); }}>
      <div className="text-xs text-muted-foreground">步骤 {step}/2 · {step === 1 ? "设备身份" : "网络与媒体端点"}</div>
      <div className={step === 1 ? "grid gap-3 md:grid-cols-2" : "hidden"}>
        <label className="space-y-1 text-sm">名称<Input name="name" placeholder="例如：华东机场集群" required /></label>
        <label className="space-y-1 text-sm">网络模式<select className="flex h-9 w-full rounded-md border bg-transparent px-3 text-sm" onChange={(event) => setMode(event.target.value as "lan" | "public")} value={mode}><option value="lan">局域网 LAN</option><option value="public">公网 Public</option></select></label>
        <label className="space-y-1 text-sm md:col-span-2">机场序列号<Input name="gatewaySerials" placeholder="多个序列号用逗号或空格分隔" required /></label>
        <label className="space-y-1 text-sm md:col-span-2">凭据引用<Input name="secretRef" placeholder="env://DJI_MQTT_CREDENTIALS 或 vault://…" required /></label>
      </div>
      <div className={step === 2 ? "grid gap-3 md:grid-cols-2" : "hidden"}>
        <label className="space-y-1 text-sm">MQTT<Input name="mqttEndpoint" placeholder={mode === "public" ? "mqtts://mqtt.example.com:8883" : "mqtt://192.168.1.10:1883"} required /></label>
        <label className="space-y-1 text-sm">应用 API<Input name="apiPublicBaseUrl" placeholder={mode === "public" ? "https://api.example.com" : "http://192.168.1.10:3100"} required /></label>
        <label className="space-y-1 text-sm">WebSocket<Input name="websocketPublicUrl" placeholder={mode === "public" ? "wss://api.example.com" : "ws://192.168.1.10:3100"} required /></label>
        <label className="space-y-1 text-sm">媒体摄取<Input name="mediaIngestBaseUrl" placeholder={mode === "public" ? "rtmps://media.example.com:443" : "rtmp://192.168.1.10:1935"} required /></label>
        <label className="space-y-1 text-sm">媒体播放<Input name="mediaPlaybackBaseUrl" placeholder={mode === "public" ? "https://media.example.com" : "http://192.168.1.10:8888"} required /></label>
        <label className="space-y-1 text-sm">NTP Host<Input name="ntpServerHost" placeholder="time.example.com" required /></label>
        <label className="space-y-1 text-sm">NTP Port<Input defaultValue="123" max="65535" min="1" name="ntpServerPort" required type="number" /></label>
        <label className="flex items-center gap-2 self-end text-sm"><input defaultChecked={mode === "public"} disabled={mode === "public"} name="tlsRequired" type="checkbox" />强制 TLS</label>
      </div>
      <div className="flex gap-2">
        {step === 2 && <Button onClick={() => setStep(1)} type="button" variant="outline">上一步</Button>}
        {step === 1
          ? <Button onClick={() => setStep(2)} type="button">下一步</Button>
          : <Button disabled={busy} type="submit">{busy && <Loader2Icon className="animate-spin" />}保存并连接</Button>}
      </div>
    </form>

    {error && <div className="rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive">{error}{issues.length > 0 && <ul className="mt-2 list-inside list-disc">{issues.map((issue) => <li key={`${issue.field}:${issue.code}`}>{issue.field}: {issue.code}</li>)}</ul>}</div>}
    {configurationSummary && <div className="rounded-lg border p-3 text-sm"><p className="font-medium">DJI 配置摘要（秘密已脱敏）</p><pre className="mt-2 max-h-72 overflow-auto whitespace-pre-wrap text-xs text-muted-foreground">{JSON.stringify(configurationSummary, null, 2)}</pre></div>}

    <div className="space-y-2">
      {adapters.map((adapter) => <article className="flex flex-wrap items-center justify-between gap-3 rounded-lg bg-muted/30 p-3" key={adapter.id}>
        <div><p className="text-sm font-medium">{adapter.name}</p><p className="text-xs text-muted-foreground">DJI · {adapter.protocolVersion} · {adapter.status} · 凭据{adapter.hasSecret ? "已配置（不显示）" : "未配置"}</p></div>
        <Button disabled={busy} onClick={() => void testConnection(adapter)} size="sm" type="button" variant="outline">连接自检</Button>
      </article>)}
      {adapters.length === 0 && <p className="text-sm text-muted-foreground">尚未配置 DJI Adapter。</p>}
    </div>
    {testResult && <div className="rounded-lg border p-3 text-sm"><p className="flex items-center gap-2 font-medium"><ShieldCheckIcon className="size-4" />服务端自检结果</p><pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap text-xs text-muted-foreground">{JSON.stringify(testResult, null, 2)}</pre><p className="mt-2 flex items-center gap-1 text-xs text-muted-foreground"><CheckCircle2Icon className="size-3.5" />设备侧可达性仍需在 DJI Pilot/Dock 现场确认。</p></div>}
  </section>;
}
