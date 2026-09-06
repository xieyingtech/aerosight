"use client";

import { useState } from "react";
import { apiJSON, APIError } from "@/lib/api-client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { AIProviderView } from "@/lib/web-api-types";

function payload(formData: FormData) {
  return {
    name: String(formData.get("name") ?? ""), providerType: "openai",
    baseUrl: String(formData.get("baseUrl") ?? ""), modelId: String(formData.get("modelId") ?? ""),
    apiKey: String(formData.get("apiKey") ?? ""), enabled: formData.get("enabled") === "on",
    isDefault: formData.get("isDefault") === "on"
  };
}

export function AIProviderForm({ provider, onChanged }: { provider?: AIProviderView; onChanged?: () => void }) {
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
    <Input defaultValue={provider?.modelId} name="modelId" placeholder="模型 ID，例如 gpt-5-mini" required />
    <Input className="md:col-span-2" defaultValue={provider?.baseUrl ?? ""} name="baseUrl" placeholder="基础地址（可选，默认 OpenAI）" />
    <Input className="md:col-span-2" name="apiKey" placeholder={provider ? "API Key（留空保持不变）" : "API Key"} type="password" autoComplete="new-password" required={!provider} />
    <label className="flex items-center gap-2 text-sm"><input defaultChecked={provider?.enabled} name="enabled" type="checkbox" />启用</label>
    <label className="flex items-center gap-2 text-sm"><input defaultChecked={provider?.isDefault} name="isDefault" type="checkbox" />设为默认</label>
    {provider ? <div className="flex flex-wrap gap-2 md:col-span-2">
      <Button disabled={busy} type="submit">保存</Button>
      <Button disabled={busy} onClick={() => request(`/api/admin/ai-providers/${provider.id}/test`, "POST")} type="button" variant="outline">测试连接</Button>
      <Button disabled={busy} onClick={() => request(`/api/admin/ai-providers/${provider.id}`, "DELETE")} type="button" variant="destructive">删除</Button>
    </div> : <Button className="md:col-span-2" disabled={busy} type="submit">添加 AI Provider</Button>}
    {error ? <p className="text-sm text-destructive md:col-span-2">{error}</p> : null}
  </form>;
}
