"use client";

import { useState } from "react";
import { AIProviderForm, aiProviderError } from "@/components/ai-provider-form";
import { ProviderModelBadges } from "@/components/provider-model-badges";
import { Page } from "@/components/page";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import type { AIProviderView } from "@/lib/web-api-types";
import { useAPI } from "@/lib/use-api";
import { apiJSON } from "@/lib/api-client";
import { APIStateView } from "@/components/api-state";


export default function AdminAIProvidersPage() {
  const state = useAPI<AIProviderView[]>("/api/admin/ai-providers");
  const [editing, setEditing] = useState<AIProviderView | "new" | null>(null);
  const [deleting, setDeleting] = useState<AIProviderView | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState("");
  async function test(provider: AIProviderView) {
    setBusy(provider.id); setError(null); setNotice("");
    try {
      const result = await apiJSON<{ ok: boolean; code: string }>(`/api/admin/ai-providers/${provider.id}/test`, { method: "POST" });
      setNotice(`${provider.name}：${result.ok ? "API 连接正常" : `连接失败（${result.code}）`}`); state.reload();
    } catch (error) { setError(aiProviderError(error)); }
    finally { setBusy(null); }
  }
  async function remove() {
    if (!deleting) return;
    setBusy(deleting.id); setError(null);
    try { await apiJSON(`/api/admin/ai-providers/${deleting.id}`, { method: "DELETE" }); setDeleting(null); setEditing(null); state.reload(); }
    catch (error) { setError(aiProviderError(error)); }
    finally { setBusy(null); }
  }
  async function setDefault(kind: "text" | "realtime", value: string) {
    setBusy("defaults"); setError(null);
    try {
      const [providerId, modelId] = value ? JSON.parse(value) as [string, string] : ["", ""];
      await apiJSON("/api/admin/ai-providers/defaults", { method: "PUT", headers: { "content-type": "application/json" }, body: JSON.stringify({ kind, providerId, modelId }) });
      state.reload();
    } catch (error) { setError(aiProviderError(error)); }
    finally { setBusy(null); }
  }
  return <APIStateView state={state}>{(providers) => <Page title="AI Provider" description="管理供应商连接与模型目录，独立配置文字和实时语音的默认模型。仅平台管理员可管理。" actions={<Button onClick={() => setEditing("new")}>新建 Provider</Button>}>
    <div className="grid gap-3 sm:grid-cols-2">{(["text", "realtime"] as const).map((type) => {
      const provider = providers.find((item) => type === "text" ? item.isDefault : item.isRealtimeDefault);
      const label = type === "text" ? "默认文字模型" : "默认实时模型";
      const value = provider ? JSON.stringify([provider.id, type === "text" ? provider.modelId : provider.realtimeModelId]) : "";
      return <label key={type} className="grid gap-2 rounded-xl border p-4 text-sm"><span className="text-muted-foreground">{label}</span>
        <select aria-label={label} className="h-9 min-w-0 rounded-md border bg-background px-2" value={value} disabled={!!busy} onChange={(event) => setDefault(type, event.target.value)}>
          <option value="">未设置</option>
          {providers.filter((item) => item.enabled).map((item) => <optgroup key={item.id} label={item.name}>
            {(item.models ?? []).filter((model) => type === "realtime" ? model.protocol === "stepfun-realtime" : ["openai-compatible", "responses", "anthropic-messages"].includes(model.protocol)).map((model) => <option key={model.id} value={JSON.stringify([item.id, model.id])}>{item.name} / {model.id}</option>)}
          </optgroup>)}
        </select>
      </label>;
    })}</div>
    {error && !deleting ? <p role="alert" className="text-sm text-destructive">{error}</p> : null}
    {notice ? <p role="status" className="text-sm">{notice}</p> : null}
    <div className="overflow-x-auto rounded-xl border"><table className="w-full table-fixed text-left text-sm"><thead className="bg-muted/50"><tr><th className="w-[22%] px-3 py-3">供应商</th><th className="px-2 py-3">模型</th><th className="w-28 px-3 py-3 text-right">操作</th></tr></thead><tbody>
      {providers.map((provider) => <tr key={provider.id} className="border-t">
        <td className="px-3 py-3"><div className="truncate font-medium" title={provider.name}>{provider.name}</div></td>
        <td className="px-2 py-3"><ProviderModelBadges models={provider.models ?? []} /></td>
        <td className="px-3 py-3"><div className="flex justify-end gap-1"><Button size="sm" variant="ghost" disabled={!!busy} onClick={() => test(provider)}>{busy === provider.id ? "测试中" : "测试"}</Button><Button size="sm" variant="ghost" disabled={!!busy} onClick={() => setEditing(provider)}>编辑</Button></div></td>
      </tr>)}
      {!providers.length ? <tr><td colSpan={3} className="p-10 text-center text-muted-foreground">尚未配置供应商。点击“新建 Provider”连接你的 AI 服务。</td></tr> : null}
    </tbody></table></div>
    {editing ? <AIProviderForm key={editing === "new" ? "new" : editing.id} provider={editing === "new" ? undefined : editing} onChanged={state.reload} onClose={() => setEditing(null)} onDelete={editing === "new" ? undefined : () => { setError(null); setDeleting(editing); }} /> : null}
    <Dialog open={!!deleting} onOpenChange={(open) => { if (!open && !busy) setDeleting(null); }}><DialogContent><DialogHeader><DialogTitle>删除供应商</DialogTitle><DialogDescription>删除“{deleting?.name}”及其模型配置？若它提供默认模型，对应智能体功能将暂停，直到重新设置默认模型。</DialogDescription></DialogHeader>{error ? <p role="alert" className="text-destructive">{error}</p> : null}<div className="flex justify-end gap-2"><Button variant="outline" disabled={!!busy} onClick={() => setDeleting(null)}>取消</Button><Button variant="destructive" disabled={!!busy} onClick={remove}>删除</Button></div></DialogContent></Dialog>
  </Page>}</APIStateView>;
}
