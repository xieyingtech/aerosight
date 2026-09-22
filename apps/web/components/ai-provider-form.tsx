"use client";

import { useEffect, useState } from "react";
import { Tabs } from "radix-ui";
import { RefreshCwIcon, XIcon } from "lucide-react";
import { InputSelect } from "@/components/ui/input-select";
import { apiJSON, APIError } from "@/lib/api-client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import type { AIModelConfig, AIProviderView } from "@/lib/web-api-types";

const protocols = [
  ["openai-compatible", "OpenAI Compatible"], ["responses", "Responses"],
  ["anthropic-messages", "Anthropic Messages"], ["stepfun-realtime", "StepFun Realtime"]
] as const;
const errors: Record<string, string> = {
  AI_PROVIDER_INPUT_INVALID: "请检查配置：模型 ID 不能重复，且必须选择协议。",
  AI_PROVIDER_MODELS_FAILED: "获取模型失败，请检查地址、API Key 和服务的 /models 接口，也可以手动添加模型。",
  AI_PROVIDER_ENDPOINT_KEY_REQUIRED: "基础地址已修改，请填写该地址的 API Key 后再获取模型。",
  OUTBOUND_DNS_FAILED: "无法解析服务地址，请检查服务器的网络和 DNS。",
  OUTBOUND_URL_INVALID: "请填写有效的 HTTP 或 HTTPS 基础地址，不要包含账号、查询参数或锚点。",
  AI_PROVIDER_FAILED: "保存失败，请检查供应商名称是否重复及配置是否有效。",
  FORBIDDEN: "仅平台管理员可以管理 AI Provider。"
};
export function aiProviderError(error: unknown) {
  return error instanceof APIError ? errors[error.code] ?? error.code : "操作失败，请重试。";
}

export function AIProviderForm({ provider, onChanged, onClose, onDelete }: { provider?: AIProviderView; onChanged: () => void; onClose: () => void; onDelete?: () => void }) {
  const [tab, setTab] = useState("basic");
  const [name, setName] = useState(provider?.name ?? "");
  const [baseUrl, setBaseUrl] = useState(provider?.baseUrl ?? "");
  const [apiKey, setAPIKey] = useState("");
  const [enabled, setEnabled] = useState(provider?.enabled ?? true);
  const [models, setModels] = useState<AIModelConfig[]>(() => (provider?.models ?? []).map((model) => ({ ...model, protocol: protocols.some(([key]) => key === model.protocol) ? model.protocol : "openai-compatible", enabled: true })));
  const [discovered, setDiscovered] = useState<string[]>([]);
  const [loadingModels, setLoadingModels] = useState(false);
  const [discoveryRevision, setDiscoveryRevision] = useState(0);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  function updateModel(index: number, patch: Partial<AIModelConfig>) {
    setModels(models.map((model, i) => i === index ? { ...model, ...patch } : model));
  }
  function addModel(id: string) {
    id = id.trim();
    if (!id || models.length >= 500 || models.some((model) => model.id === id)) return;
    setModels([...models, { id, protocol: "openai-compatible", capabilities: [], enabled: true }]);
  }
  useEffect(() => {
    if (tab !== "models") return;
    const controller = new AbortController();
    setLoadingModels(true); setNotice(""); setDiscovered([]);
    const timer = setTimeout(async () => {
      try {
        const result = await apiJSON<{ models: string[] }>("/api/admin/ai-providers/models", {
          method: "POST", headers: { "content-type": "application/json" }, signal: controller.signal,
          body: JSON.stringify({ providerId: provider?.id ?? "", baseUrl: baseUrl.trim(), apiKey })
        });
        if (!controller.signal.aborted) {
          setDiscovered(result.models);
          setNotice(result.models.length ? `${result.models.length} 个可用模型` : "没有可用列表，可直接输入模型 ID 添加");
        }
      } catch {
        if (!controller.signal.aborted) setNotice("模型列表加载失败，仍可输入模型 ID 添加");
      } finally { if (!controller.signal.aborted) setLoadingModels(false); }
    }, 250);
    return () => { clearTimeout(timer); controller.abort(); };
  }, [tab, baseUrl, apiKey, provider?.id, discoveryRevision]);
  async function submit(event: React.FormEvent) {
    event.preventDefault(); setError(null);
    if (!name.trim()) { setTab("basic"); setError("请填写供应商名称。"); return; }
    const normalized = models.map((model) => ({ ...model, id: model.id.trim() }));
    const keys = normalized.map((model) => model.id);
    if (normalized.some((model) => !model.id || !model.protocol) || new Set(keys).size !== keys.length) {
      setTab("models"); setError("请填写模型 ID 并选择协议，模型 ID 不能重复。"); return;
    }
    setBusy(true);
    try {
      await apiJSON(provider ? `/api/admin/ai-providers/${provider.id}` : "/api/admin/ai-providers", {
        method: provider ? "PATCH" : "POST", headers: { "content-type": "application/json" },
        body: JSON.stringify({ name: name.trim(), providerType: "openai", baseUrl: baseUrl.trim(), apiKey, enabled,
          models: normalized.map(({ id, protocol }) => ({ id, protocol })) })
      });
      onChanged(); onClose();
    } catch (error) { setError(aiProviderError(error)); }
    finally { setBusy(false); }
  }
  const options = discovered.filter((id) => !models.some((model) => model.id === id)).map((id) => ({ value: id, label: id }));
  const selectClass = "h-8 w-full rounded-md border bg-background px-2 text-xs";
  return <Dialog open onOpenChange={(open) => { if (!open && !busy) onClose(); }}>
    <DialogContent className="grid h-[min(760px,90svh)] grid-rows-[auto_minmax(0,1fr)] gap-3 overflow-hidden sm:max-w-3xl" showCloseButton={!busy}>
      <DialogHeader className="pr-8"><DialogTitle>{provider ? "编辑 Provider" : "新建 Provider"}</DialogTitle><DialogDescription>{name || "供应商"} · 连接与模型配置</DialogDescription></DialogHeader>
      <form onSubmit={submit} className="grid min-h-0 min-w-0 grid-rows-[minmax(0,1fr)_auto] gap-3">
        <Tabs.Root value={tab} onValueChange={setTab} className="flex min-h-0 min-w-0 flex-col">
          <Tabs.List aria-label="Provider 配置" className="mb-4 flex shrink-0 gap-4 border-b">
            <Tabs.Trigger value="basic" className="pb-2 text-sm data-[state=active]:border-b-2 data-[state=active]:border-primary">基础配置</Tabs.Trigger>
            <Tabs.Trigger value="models" className="pb-2 text-sm data-[state=active]:border-b-2 data-[state=active]:border-primary">模型配置 · {models.length}</Tabs.Trigger>
          </Tabs.List>
          <Tabs.Content value="basic" className="min-h-0 overflow-y-auto">
            <fieldset disabled={busy} className="grid min-w-0 grid-cols-1 items-start gap-4 pb-2 sm:grid-cols-3">
              <label className="grid min-w-0 gap-2">供应商名称<Input value={name} onChange={(e) => setName(e.target.value)} maxLength={120} placeholder="例如：内网推理服务" /></label>
              <div className="grid min-w-0 gap-2 sm:col-span-2">
                <label className="grid gap-2">基础地址<Input value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} placeholder="http://192.168.1.10:8000/v1" /></label>
                <p className="text-xs text-muted-foreground">支持内网 HTTP / HTTPS，留空使用 OpenAI。</p>
              </div>
              <label className="grid min-w-0 gap-2 sm:col-span-3">API Key<Input value={apiKey} onChange={(e) => setAPIKey(e.target.value)} type="password" autoComplete="new-password" placeholder={provider ? "留空保持原 Key" : "无认证服务可留空"} /></label>
              <label className="flex items-center gap-2 sm:col-span-3"><input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />启用供应商</label>
            </fieldset>
          </Tabs.Content>
          <Tabs.Content value="models" className="flex min-h-0 min-w-0 flex-1 flex-col gap-3 data-[state=inactive]:hidden">
            <div className="relative z-20 shrink-0 space-y-1.5">
              <div className="flex items-center gap-2">
                <div className="min-w-0 flex-1"><InputSelect value={null} options={options} onValueChange={addModel} allowCustom ariaLabel="添加模型" disabled={busy || models.length >= 500} placeholder="搜索、选择或输入模型 ID 添加" emptyMessage={loadingModels ? "正在加载模型…" : "输入模型 ID 即可添加"} /></div>
                <Button type="button" variant="ghost" size="icon" aria-label="刷新模型列表" disabled={loadingModels || busy} onClick={() => setDiscoveryRevision((value) => value + 1)}><RefreshCwIcon className={loadingModels ? "size-4 animate-spin" : "size-4"} /></Button>
              </div>
              <p role="status" className="text-xs text-muted-foreground">{loadingModels ? "正在加载模型列表…" : notice || "支持添加列表以外的模型"}</p>
            </div>
            <div data-testid="model-matrix-scroll" className="min-h-0 flex-1 overflow-auto overscroll-contain rounded-lg border">
              <table aria-label="模型协议配置" className="w-full table-fixed border-separate border-spacing-0 text-xs">
                <thead className="sticky top-0 z-10 bg-muted"><tr>
                  <th className="sticky left-0 z-20 w-[48%] border-b bg-muted px-3 py-3 text-left">模型</th>
                  <th className="w-[42%] border-b px-2 py-3 text-left">协议</th>
                  <th className="border-b px-2 py-3"><span className="sr-only">操作</span></th>
                </tr></thead>
                <tbody>{models.map((model, index) => <tr key={index}>
                  <th scope="row" className="sticky left-0 z-[1] border-b bg-popover px-3 py-2 text-left font-normal"><Input aria-label={`模型 ${index + 1} ID`} className="h-8 w-full min-w-0 border-transparent px-0 text-xs shadow-none focus:border-input" title={model.id} value={model.id} maxLength={255} disabled={busy} onChange={(e) => updateModel(index, { id: e.target.value })} /></th>
                  <td className="border-b px-2 py-2"><select aria-label={`${model.id} 调用协议`} className={selectClass} disabled={busy} value={model.protocol} onChange={(e) => {
                    const protocol = e.target.value as AIModelConfig["protocol"];
                    updateModel(index, { protocol });
                  }}>{protocols.map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></td>
                  <td className="border-b px-2 py-2"><Button type="button" variant="ghost" size="icon-sm" aria-label={`移除 ${model.id}`} disabled={busy} onClick={() => { updateModel(index, { enabled: false }); setModels(models.filter((_, i) => i !== index)); }}><XIcon className="size-3.5" /></Button></td>
                </tr>)}</tbody>
              </table>
              {!models.length ? <p className="p-8 text-sm text-muted-foreground">从上方添加模型，再选择调用协议。</p> : null}
            </div>
            <p className="shrink-0 text-xs text-muted-foreground">新模型默认使用 OpenAI Compatible。</p>
          </Tabs.Content>
        </Tabs.Root>
        <div className="space-y-2 border-t bg-popover pt-3">
          {error ? <p role="alert" className="text-sm text-destructive">{error}</p> : null}
          <div className="flex items-center gap-2">{provider && onDelete ? <Button type="button" variant="ghost" className="mr-auto text-destructive hover:text-destructive" disabled={busy} onClick={onDelete}>删除 Provider</Button> : <span className="mr-auto" />}<Button type="button" variant="outline" disabled={busy} onClick={onClose}>取消</Button><Button type="submit" disabled={busy}>{busy ? "保存中…" : "保存配置"}</Button></div>
        </div>
      </form>
    </DialogContent>
  </Dialog>;
}
