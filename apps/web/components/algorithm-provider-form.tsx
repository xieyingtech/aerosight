"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

type Provider = {
  id: string; name: string; providerType: "http-json" | "kserve-v2" | "ogc-processes" | "ai-sdk";
  baseUrl: string; authType: "none" | "bearer" | "api-key-header" | "basic" | "signed";
  allowedHeaders: string[]; timeoutSeconds: number; concurrencyLimit: number; rateLimitPerMinute: number;
};

export function AlgorithmProviderForm({ projectId, provider }: { projectId: number; provider?: Provider }) {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);
  async function submit(formData: FormData) {
    setError(null);
    const response = await fetch(provider ? `/api/projects/${projectId}/algorithm-providers/${provider.id}` : `/api/projects/${projectId}/algorithm-providers`, { method: provider ? "PATCH" : "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({
      name: formData.get("name"), providerType: formData.get("providerType"), baseUrl: formData.get("baseUrl"),
      credential: formData.get("credential") || "", username: formData.get("username") || "", authType: formData.get("authType"),
      allowedHeaders: String(formData.get("allowedHeaders") ?? "").split(",").map((value) => value.trim()).filter(Boolean),
      timeoutSeconds: Number(formData.get("timeoutSeconds")), concurrencyLimit: Number(formData.get("concurrencyLimit")),
      rateLimitPerMinute: Number(formData.get("rateLimitPerMinute"))
    }) });
    if (!response.ok) setError((await response.json()).error ?? "保存失败"); else router.refresh();
  }
  return <form action={submit} className="grid gap-3 rounded-xl border p-4 md:grid-cols-2">
    <Input defaultValue={provider?.name} name="name" placeholder="服务名称" required /><Input defaultValue={provider?.baseUrl} name="baseUrl" placeholder="https://algorithm.example.test" required />
    <select className="h-9 rounded-md border bg-transparent px-3 text-sm" defaultValue={provider?.providerType ?? "http-json"} name="providerType"><option value="http-json">HTTP JSON（已启用）</option><option value="kserve-v2">KServe V2（未启用）</option><option value="ogc-processes">OGC Processes（未启用）</option><option value="ai-sdk">AI SDK（未启用）</option></select>
    <select className="h-9 rounded-md border bg-transparent px-3 text-sm" defaultValue={provider?.authType ?? "none"} name="authType"><option value="none">无认证</option><option value="bearer">Bearer</option><option value="api-key-header">API Key Header</option><option value="basic">Basic</option><option value="signed">签名</option></select>
    <Input name="username" placeholder="用户名（仅 Basic 认证）" autoComplete="off" />
    <Input name="credential" placeholder="Token / API Key / 密码（留空则不更新）" type="password" autoComplete="new-password" />
    <Input className="md:col-span-2" defaultValue={provider?.allowedHeaders.join(", ")} name="allowedHeaders" placeholder="允许的 Header 名称，以逗号分隔" />
    <Input defaultValue={provider?.timeoutSeconds ?? 30} min="1" name="timeoutSeconds" placeholder="超时（秒）" type="number" />
    <Input defaultValue={provider?.concurrencyLimit ?? 1} min="1" name="concurrencyLimit" placeholder="并发限制" type="number" />
    <Input className="md:col-span-2" defaultValue={provider?.rateLimitPerMinute ?? 60} min="1" name="rateLimitPerMinute" placeholder="每分钟速率限制" type="number" />
    {error ? <p className="text-sm text-destructive md:col-span-2">{error}</p> : null}<Button className="md:col-span-2" type="submit">{provider ? "保存算法服务" : "添加算法服务"}</Button>
  </form>;
}
