"use client";
import { useState } from "react";
import { apiJSON, APIError } from "@/lib/api-client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import type { AlgorithmProviderView } from "@/lib/web-api-types";

export const algorithmProviderError = (error: unknown) => {
  if (!(error instanceof APIError)) return "操作失败，请稍后重试。";
  return ({
    ALGORITHM_PROVIDER_INPUT_INVALID: "请检查服务名称、地址和配置项。",
    ALGORITHM_PROVIDER_CREDENTIAL_REQUIRED: "当前认证方式需要填写凭据。",
    ALGORITHM_PROVIDER_IN_USE: "该服务仍被项目算法定义使用，无法删除。",
    OUTBOUND_URL_INVALID: "请填写有效的服务地址。",
    FORBIDDEN: "仅平台管理员可以管理算法服务。"
  } as Record<string, string>)[error.code] ?? error.code;
};
export function AlgorithmProviderForm({ provider, onChanged, onClose, onDelete }: { provider?: AlgorithmProviderView; onChanged: () => void; onClose: () => void; onDelete?: () => void }) {
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError(null); setPending(true);
    const formData = new FormData(event.currentTarget);
    try {
      await apiJSON(provider ? `/api/admin/algorithm-providers/${provider.id}` : "/api/admin/algorithm-providers", { method: provider ? "PATCH" : "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({
        name: formData.get("name"), providerType: formData.get("providerType"), baseUrl: formData.get("baseUrl"), credential: formData.get("credential") || "", username: formData.get("username") || "", authType: formData.get("authType"),
        allowedHeaders: String(formData.get("allowedHeaders") ?? "").split(",").map((value) => value.trim()).filter(Boolean),
        timeoutSeconds: Number(formData.get("timeoutSeconds")), concurrencyLimit: Number(formData.get("concurrencyLimit")), rateLimitPerMinute: Number(formData.get("rateLimitPerMinute")), status: formData.get("status")
      }) }); onChanged(); onClose();
    } catch (error) { setError(algorithmProviderError(error)); } finally { setPending(false); }
  }
  const field = "grid gap-1.5 text-sm";
  const select = "h-9 w-full rounded-md border bg-background px-3 text-sm";
  return <Dialog open onOpenChange={(open) => { if (!open && !pending) onClose(); }}><DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-2xl" showCloseButton={!pending}>
    <DialogHeader><DialogTitle>{provider ? "编辑算法服务" : "新建算法服务"}</DialogTitle><DialogDescription>平台级连接配置。凭据加密保存，编辑时留空保持原值。</DialogDescription></DialogHeader>
    <form onSubmit={submit} className="grid gap-4 sm:grid-cols-2">
      <label className={field}>服务名称<Input name="name" defaultValue={provider?.name} maxLength={120} required /></label>
      <label className={field}>状态<select name="status" defaultValue={provider?.status ?? "disabled"} className={select}><option value="disabled">停用</option><option value="active">启用</option></select></label>
      <label className={`${field} sm:col-span-2`}>基础地址<Input name="baseUrl" defaultValue={provider?.baseUrl} placeholder="https://algorithm.example.test" required /></label>
      <label className={field}>服务协议<select name="providerType" defaultValue={provider?.providerType ?? "http-json"} className={select}><option value="http-json">HTTP JSON</option><option value="kserve-v2">KServe V2（未启用）</option><option value="ogc-processes">OGC Processes（未启用）</option><option value="ai-sdk">AI SDK（未启用）</option></select></label>
      <label className={field}>认证方式<select name="authType" defaultValue={provider?.authType ?? "none"} className={select}><option value="none">无认证</option><option value="bearer">Bearer</option><option value="api-key-header">API Key Header</option><option value="basic">Basic</option><option value="signed">签名</option></select></label>
      <label className={field}>用户名（Basic）<Input name="username" autoComplete="off" /></label>
      <label className={field}>凭据<Input name="credential" type="password" autoComplete="new-password" placeholder={provider ? "留空保持原凭据" : "Token / API Key / 密码"} /></label>
      <label className={`${field} sm:col-span-2`}>允许的 Header 名称<Input name="allowedHeaders" defaultValue={provider?.allowedHeaders.join(", ")} placeholder="以逗号分隔" /></label>
      <label className={field}>超时（秒）<Input name="timeoutSeconds" type="number" min="1" defaultValue={provider?.timeoutSeconds ?? 30} required /></label>
      <label className={field}>并发限制<Input name="concurrencyLimit" type="number" min="1" defaultValue={provider?.concurrencyLimit ?? 1} required /></label>
      <label className={field}>每分钟速率限制<Input name="rateLimitPerMinute" type="number" min="1" defaultValue={provider?.rateLimitPerMinute ?? 60} required /></label>
      {error ? <p role="alert" className="text-sm text-destructive sm:col-span-2">{error}</p> : null}
      <div className="flex justify-end gap-2 border-t pt-4 sm:col-span-2">{onDelete ? <Button type="button" variant="ghost" className="mr-auto text-destructive" disabled={pending} onClick={onDelete}>删除服务</Button> : null}<Button type="button" variant="outline" disabled={pending} onClick={onClose}>取消</Button><Button type="submit" disabled={pending}>{pending ? "保存中…" : "保存配置"}</Button></div>
    </form>
  </DialogContent></Dialog>;
}
