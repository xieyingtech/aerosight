"use client";

import { useId, useState } from "react";
import { apiJSON, APIError } from "@/lib/api-client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { AIProviderView } from "@/lib/web-api-types";

function payload(formData: FormData) {
  return {
    name: String(formData.get("name") ?? ""), providerType: "openai",
    baseUrl: String(formData.get("baseUrl") ?? ""), modelId: String(formData.get("modelId") ?? ""),
    apiKey: String(formData.get("apiKey") ?? ""), enabled: formData.get("enabled") === "on",
    isDefault: formData.get("isDefault") === "on",
    realtimeProtocol: String(formData.get("realtimeProtocol") ?? "disabled"),
    realtimeModelId: formData.get("realtimeProtocol") === "disabled" ? "" : String(formData.get("realtimeModelId") ?? "")
  };
}

export function AIProviderForm({ provider, onChanged }: { provider?: AIProviderView; onChanged?: () => void }) {
  const modelListId = useId();
  const [realtimeProtocol, setRealtimeProtocol] = useState(provider?.realtimeProtocol ?? "disabled");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  async function request(url: string, method: string, body?: unknown) {
    setBusy(true); setError(null);
    try {
      await apiJSON(url, { method, headers: body ? { "content-type": "application/json" } : undefined, body: body ? JSON.stringify(body) : undefined });
      onChanged?.(); return true;
    } catch (error) { setError(error instanceof APIError ? error.code : "操作失败，请重试。"); return false; }
    finally { setBusy(false); }
  }
  async function submit(formData: FormData) {
    await request(provider ? `/api/admin/ai-providers/${provider.id}` : "/api/admin/ai-providers", provider ? "PATCH" : "POST", payload(formData));
  }
  return <form action={submit} className="grid gap-3 rounded-xl border p-4 md:grid-cols-2">
    <Input defaultValue={provider?.name} name="name" placeholder="配置名称" required />
    <label className="grid gap-1 text-sm">文字模型<Input defaultValue={provider?.modelId} name="modelId" placeholder="模型 ID，例如 step-5-preview" required /></label>
    <label className="grid gap-1 text-sm md:col-span-2">文字协议<select aria-label="文字协议" disabled className="h-9 rounded-md border bg-muted px-3"><option>OpenAI Responses 兼容协议</option></select></label>
    <Input className="md:col-span-2" defaultValue={provider?.baseUrl ?? ""} name="baseUrl" required={realtimeProtocol !== "disabled"} placeholder="基础地址（可选，默认 OpenAI）" />
    <Input className="md:col-span-2" name="apiKey" placeholder={provider ? "API Key（留空保持不变）" : "API Key"} type="password" autoComplete="new-password" required={!provider} />
    <label className="flex items-center gap-2 text-sm"><input defaultChecked={provider?.enabled} name="enabled" type="checkbox" />启用</label>
    <label className="flex items-center gap-2 text-sm"><input defaultChecked={provider?.isDefault} name="isDefault" type="checkbox" />设为默认</label>
    <fieldset className="grid gap-3 border-t pt-4 md:col-span-2 md:grid-cols-2">
      <legend className="px-1 text-sm font-medium">实时语音</legend>
      <label className="grid gap-1 text-sm">实时协议
        <select name="realtimeProtocol" value={realtimeProtocol} onChange={(event) => setRealtimeProtocol(event.target.value as "disabled" | "stepfun")} className="h-9 rounded-md border bg-background px-3">
          <option value="disabled">未启用</option><option value="stepfun">StepFun Realtime</option>
        </select>
      </label>
      <label className="grid gap-1 text-sm">实时模型 ID
        <Input name="realtimeModelId" defaultValue={provider?.realtimeModelId ?? ""} placeholder="例如 stepaudio-2.5-realtime" disabled={realtimeProtocol === "disabled"} required={realtimeProtocol !== "disabled"} maxLength={255} list={modelListId} />
      </label>
      <datalist id={modelListId}><option value="stepaudio-2.5-realtime" /></datalist>
      <p className="text-xs text-muted-foreground md:col-span-2">复用本配置的基础地址与 API Key，使用默认 Provider。可输入该服务支持的模型 ID；目前支持 StepFun Realtime 协议。</p>
    </fieldset>
    {provider ? <div className="flex flex-wrap gap-2 md:col-span-2">
      <Button disabled={busy} type="submit">保存</Button>
      <Button disabled={busy} onClick={() => request(`/api/admin/ai-providers/${provider.id}/test`, "POST")} type="button" variant="outline">测试 API 连接</Button>
      <Button disabled={busy} onClick={() => request(`/api/admin/ai-providers/${provider.id}`, "DELETE")} type="button" variant="destructive">删除</Button>
    </div> : <Button className="md:col-span-2" disabled={busy} type="submit">添加 AI Provider</Button>}
    {error ? <p className="text-sm text-destructive md:col-span-2">{error}</p> : null}
  </form>;
}
