"use client";
import { useState } from "react";
import { AlgorithmProviderForm, algorithmProviderError } from "@/components/algorithm-provider-form";
import { APIStateView } from "@/components/api-state";
import { Page } from "@/components/page";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { apiJSON } from "@/lib/api-client";
import { useAPI } from "@/lib/use-api";
import type { AlgorithmProviderView } from "@/lib/web-api-types";

export default function AdminAlgorithmProvidersPage() {
  const state = useAPI<AlgorithmProviderView[]>("/api/admin/algorithm-providers");
  const [editing, setEditing] = useState<AlgorithmProviderView | "new" | null>(null);
  const [deleting, setDeleting] = useState<AlgorithmProviderView | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState("");
  async function test(provider: AlgorithmProviderView) {
    setBusy(provider.id); setError(null); setNotice("");
    try { const result = await apiJSON<{ safe: boolean }>(`/api/admin/algorithm-providers/${provider.id}/test`, { method: "POST" }); setNotice(`${provider.name}：${result.safe ? "连接地址安全且协议可用" : "测试失败"}`); }
    catch (error) { setError(algorithmProviderError(error)); } finally { setBusy(null); }
  }
  async function remove() {
    if (!deleting) return;
    setBusy(deleting.id); setError(null);
    try { await apiJSON(`/api/admin/algorithm-providers/${deleting.id}`, { method: "DELETE" }); setDeleting(null); setEditing(null); state.reload(); }
    catch (error) { setError(algorithmProviderError(error)); } finally { setBusy(null); }
  }
  return <APIStateView state={state}>{(providers) => <Page title="算法服务" description="管理全平台共享的算法服务连接。仅平台管理员可修改。" actions={<Button onClick={() => setEditing("new")}>新建算法服务</Button>}>
    {error && !deleting ? <p role="alert" className="text-sm text-destructive">{error}</p> : null}
    {notice ? <p role="status" className="text-sm">{notice}</p> : null}
    <div className="overflow-x-auto rounded-xl border"><table className="w-full text-left text-sm"><thead className="bg-muted/50"><tr><th className="px-3 py-3">服务</th><th className="px-3 py-3">协议</th><th className="px-3 py-3">状态</th><th className="px-3 py-3 text-right">操作</th></tr></thead><tbody>
      {providers.map((provider) => <tr key={provider.id} className="border-t"><td className="px-3 py-3"><div className="font-medium">{provider.name}</div><div className="max-w-sm truncate text-xs text-muted-foreground" title={provider.baseUrl}>{provider.baseUrl}</div></td><td className="px-3 py-3">{provider.providerType}</td><td className="px-3 py-3">{provider.status === "active" ? "启用" : "停用"}</td><td className="px-3 py-3"><div className="flex justify-end gap-1"><Button size="sm" variant="ghost" disabled={!!busy} onClick={() => test(provider)}>{busy === provider.id ? "测试中" : "测试"}</Button><Button size="sm" variant="ghost" disabled={!!busy} onClick={() => setEditing(provider)}>编辑</Button></div></td></tr>)}
      {!providers.length ? <tr><td colSpan={4} className="p-10 text-center text-muted-foreground">尚未配置算法服务。点击“新建算法服务”添加。</td></tr> : null}
    </tbody></table></div>
    {editing ? <AlgorithmProviderForm key={editing === "new" ? "new" : editing.id} provider={editing === "new" ? undefined : editing} onChanged={state.reload} onClose={() => setEditing(null)} onDelete={editing === "new" ? undefined : () => { setError(null); setDeleting(editing); }} /> : null}
    <Dialog open={!!deleting} onOpenChange={(open) => { if (!open && !busy) setDeleting(null); }}><DialogContent><DialogHeader><DialogTitle>删除算法服务</DialogTitle><DialogDescription>删除“{deleting?.name}”？仍被项目算法定义使用的服务无法删除。</DialogDescription></DialogHeader>{error ? <p role="alert" className="text-sm text-destructive">{error}</p> : null}<div className="flex justify-end gap-2"><Button variant="outline" disabled={!!busy} onClick={() => setDeleting(null)}>取消</Button><Button variant="destructive" disabled={!!busy} onClick={remove}>删除</Button></div></DialogContent></Dialog>
  </Page>}</APIStateView>;
}
