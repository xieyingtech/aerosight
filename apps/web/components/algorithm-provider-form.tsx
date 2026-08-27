"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export function AlgorithmProviderForm({ projectId }: { projectId: number }) {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);
  async function submit(formData: FormData) {
    setError(null);
    const response = await fetch(`/api/projects/${projectId}/algorithm-providers`, { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({
      name: formData.get("name"), providerType: formData.get("providerType"), baseUrl: formData.get("baseUrl"),
      secretRef: formData.get("secretRef") || null, authType: formData.get("authType"),
      allowedHeaders: String(formData.get("allowedHeaders") ?? "").split(",").map((value) => value.trim()).filter(Boolean),
      timeoutSeconds: Number(formData.get("timeoutSeconds")), concurrencyLimit: Number(formData.get("concurrencyLimit")),
      rateLimitPerMinute: Number(formData.get("rateLimitPerMinute"))
    }) });
    if (!response.ok) setError((await response.json()).error ?? "保存失败"); else router.refresh();
  }
  return <form action={submit} className="grid gap-3 rounded-xl border p-4 md:grid-cols-2">
    <Input name="name" placeholder="服务名称" required /><Input name="baseUrl" placeholder="https://algorithm.example.test" required />
    <select className="h-9 rounded-md border bg-transparent px-3 text-sm" name="providerType"><option value="http-json">HTTP JSON</option><option value="kserve-v2">KServe V2</option><option value="ogc-processes">OGC Processes</option><option value="ai-sdk">AI SDK</option></select>
    <select className="h-9 rounded-md border bg-transparent px-3 text-sm" name="authType"><option value="none">无认证</option><option value="bearer">Bearer</option><option value="api-key-header">API Key Header</option><option value="basic">Basic</option><option value="signed">签名</option></select>
    <Input className="md:col-span-2" name="secretRef" placeholder="secret://projects/...（不填写密钥原文）" />
    <Input className="md:col-span-2" name="allowedHeaders" placeholder="允许的 Header 名称，以逗号分隔" />
    <Input defaultValue="30" min="1" name="timeoutSeconds" placeholder="超时（秒）" type="number" />
    <Input defaultValue="1" min="1" name="concurrencyLimit" placeholder="并发限制" type="number" />
    <Input className="md:col-span-2" defaultValue="60" min="1" name="rateLimitPerMinute" placeholder="每分钟速率限制" type="number" />
    {error ? <p className="text-sm text-destructive md:col-span-2">{error}</p> : null}<Button className="md:col-span-2" type="submit">添加算法服务</Button>
  </form>;
}
