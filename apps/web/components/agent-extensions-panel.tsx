"use client";

import { useState } from "react";
import { Page } from "@/components/page";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { APIStateView } from "@/components/api-state";
import { useAPI } from "@/lib/use-api";
import { apiJSON } from "@/lib/api-client";

type Policy = "disabled" | "readonly" | "approval";
type MCPTool = { name: string; description: string; policy: Policy; inputSchema: unknown };
type Extension = { id: number; name: string; enabled: boolean; revision: number; builtin?: boolean; slug?: string; description?: string; body?: string; endpoint?: string; hasCredential?: boolean; tools?: MCPTool[] };
const inputStyle = "w-full rounded-md border bg-background px-3 py-2 text-sm";
const policyLabels: Record<Policy, string> = { disabled: "禁用", readonly: "只读调用", approval: "需用户确认" };
function extensionError(error: unknown) {
  const code = error instanceof Error ? error.message : "";
  if (code.includes("REVISION_CHANGED")) return "配置已被修改，请关闭后重新打开编辑。";
  if (code.includes("CREDENTIAL_REQUIRED")) return "修改端点时请重新填写凭据，或勾选清除凭据。";
  if (code.includes("CONNECTION_FAILED") || code.includes("DISCOVERY_FAILED")) return "连接或工具发现失败，请检查 MCP 地址和凭据。";
  if (code.includes("FORBIDDEN")) return "仅平台管理员可管理。";
  if (code.includes("INPUT_INVALID")) return "请检查必填内容、Skill 标识和 MCP 地址。";
  return "操作失败，请检查配置后重试。";
}

export function AgentExtensionsPanel({ kind }: { kind: "skills" | "mcp" }) {
  const base = `/api/admin/ai-providers/${kind}`;
  const state = useAPI<Extension[]>(base);
  const [editing, setEditing] = useState<Extension | "new" | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  async function discover(row: Extension) {
    setBusy(true); setError(""); setNotice("");
    try { const result = await apiJSON<{ toolCount: number }>(`${base}/${row.id}/discover`, { method: "POST" }); setNotice(`已发现 ${result.toolCount} 个工具，请在编辑中设置调用方式。`); state.reload(); }
    catch (error) { setError(extensionError(error)); } finally { setBusy(false); }
  }
  return <APIStateView state={state}>{rows => <Page title={kind === "skills" ? "Skills" : "MCP"} description={kind === "skills" ? "平台共享的技能。启用后，智能体按需加载 Markdown 指令。" : "平台共享的远程 MCP 连接。管理员选择适合全部 Agent 用户的工具访问范围；需确认的调用必须手动授权。"} actions={<Button onClick={() => setEditing("new")}>{kind === "skills" ? "新建 Skill" : "新建 MCP"}</Button>}>
    {error ? <p role="alert" className="text-sm text-destructive">{error}</p> : null}
    {notice ? <p role="status" className="text-sm">{notice}</p> : null}
    <div className="overflow-x-auto rounded-xl border"><table className="w-full text-left text-sm"><thead className="bg-muted/50"><tr><th className="px-4 py-3">名称</th><th className="px-4 py-3">{kind === "skills" ? "简介" : "连接 / 工具"}</th><th className="px-4 py-3">状态</th><th className="px-4 py-3 text-right">操作</th></tr></thead><tbody className="divide-y">
      {rows.map(row => <tr key={row.builtin ? "builtin" : row.id}><td className="px-4 py-3 font-medium">{row.name}<div className="mt-1 text-xs font-normal text-muted-foreground">{row.builtin ? "内置 · 只读" : kind === "skills" ? row.slug : "Streamable HTTP"}</div></td><td className="max-w-lg px-4 py-3"><div className="break-all text-muted-foreground">{kind === "skills" ? row.description || "—" : row.endpoint}</div>{kind === "mcp" ? <div className="mt-1 text-xs">{row.tools?.length ?? 0} 个工具 · {row.tools?.filter(tool => tool.policy !== "disabled").length ?? 0} 个允许调用{row.hasCredential ? " · 已配置凭据" : ""}</div> : null}</td><td className="px-4 py-3">{row.enabled ? "启用" : "停用"}</td><td className="px-4 py-3"><div className="flex justify-end gap-1">{kind === "mcp" ? <Button variant="ghost" size="sm" disabled={busy} onClick={() => discover(row)}>测试并发现</Button> : null}<Button variant="ghost" size="sm" disabled={busy} onClick={() => setEditing(row)}>{row.builtin ? "查看" : "编辑"}</Button></div></td></tr>)}
      {!rows.length ? <tr><td colSpan={4} className="p-10 text-center text-muted-foreground">尚未配置 {kind === "skills" ? "Skills" : "MCP"}。</td></tr> : null}
    </tbody></table></div>
    {editing ? <ExtensionEditor key={editing === "new" ? "new" : editing.id} kind={kind} row={editing === "new" ? undefined : editing} onClose={() => setEditing(null)} onChanged={() => { setEditing(null); state.reload(); }} /> : null}
  </Page>}</APIStateView>;
}

function ExtensionEditor({ kind, row, onClose, onChanged }: { kind: "skills" | "mcp"; row?: Extension; onClose: () => void; onChanged: () => void }) {
  const [name, setName] = useState(row?.name ?? "");
  const [slug, setSlug] = useState(row?.slug ?? "");
  const [description, setDescription] = useState(row?.description ?? "");
  const [body, setBody] = useState(row?.body ?? "");
  const [endpoint, setEndpoint] = useState(row?.endpoint ?? "");
  const [bearer, setBearer] = useState("");
  const [clearBearer, setClearBearer] = useState(false);
  const [enabled, setEnabled] = useState(row?.enabled ?? false);
  const [tools, setTools] = useState(row?.tools ?? []);
  const [busy, setBusy] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState("");
  const readonly = !!row?.builtin;
  const base = `/api/admin/ai-providers/${kind}`;
  async function save(event: React.FormEvent) {
    event.preventDefault(); setBusy(true); setError("");
    const data = kind === "skills" ? { name, slug, description, body, enabled, revision: row?.revision ?? 0 } : { name, endpoint, enabled, revision: row?.revision ?? 0, ...(clearBearer || bearer ? { bearer: clearBearer ? "" : bearer } : {}), tools: endpoint === row?.endpoint ? tools.map(tool => ({ name: tool.name, policy: tool.policy })) : [] };
    try { await apiJSON(row ? `${base}/${row.id}` : base, { method: row ? "PATCH" : "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(data) }); onChanged(); }
    catch (error) { setError(extensionError(error)); } finally { setBusy(false); }
  }
  async function remove() {
    setBusy(true); setError("");
    try { await apiJSON(`${base}/${row?.id}`, { method: "DELETE" }); onChanged(); }
    catch (error) { setError(extensionError(error)); } finally { setBusy(false); }
  }
  return <Dialog open onOpenChange={open => { if (!open && !busy) onClose(); }}><DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-2xl"><DialogHeader><DialogTitle>{readonly ? "内置 Skill" : `${row ? "编辑" : "新建"} ${kind === "skills" ? "Skill" : "MCP"}`}</DialogTitle><DialogDescription>{kind === "skills" ? "用 Markdown 描述技能步骤；不会执行脚本或赋予额外权限。" : "支持远程 Streamable HTTP。保存连接后测试并发现，再设置各工具的调用方式。"}</DialogDescription></DialogHeader>
    <form onSubmit={save} className="space-y-4"><fieldset disabled={busy || readonly} className="space-y-4">
      <label className="grid gap-2 text-sm">名称<input required maxLength={128} className={inputStyle} value={name} onChange={event => setName(event.target.value)} /></label>
      {kind === "skills" ? <>
        <label className="grid gap-2 text-sm">Skill 标识<input required pattern="[a-z0-9][a-z0-9-]{0,79}" className={inputStyle} value={slug} onChange={event => setSlug(event.target.value)} placeholder="例如 material-review" /></label>
        <label className="grid gap-2 text-sm">简介<input maxLength={2048} className={inputStyle} value={description} onChange={event => setDescription(event.target.value)} /></label>
        <label className="grid gap-2 text-sm">Markdown 指令<textarea required maxLength={65536} rows={12} className={`${inputStyle} font-mono`} value={body} onChange={event => setBody(event.target.value)} /></label>
      </> : <>
        <label className="grid gap-2 text-sm">MCP 地址<input required type="url" className={inputStyle} value={endpoint} onChange={event => setEndpoint(event.target.value)} placeholder="https://example.com/mcp" /><span className="text-xs text-muted-foreground">地址不含凭据、查询参数或片段。更换地址后需重新发现工具。</span></label>
        <label className="grid gap-2 text-sm">Bearer 凭据<input type="password" autoComplete="new-password" disabled={clearBearer} className={inputStyle} value={bearer} onChange={event => setBearer(event.target.value)} placeholder={row?.hasCredential ? "已配置；留空保持原值" : "可选"} /></label>
        {row?.hasCredential ? <label className="flex gap-2 text-sm"><input type="checkbox" checked={clearBearer} onChange={event => setClearBearer(event.target.checked)} />清除已保存凭据</label> : null}
        {endpoint === row?.endpoint && tools.length ? <div className="space-y-3"><p className="text-sm font-medium">工具调用方式</p>{tools.map((tool, index) => <div key={tool.name} className="rounded-lg border p-3"><div className="flex items-center justify-between gap-3"><span className="break-all text-sm font-medium">{tool.name}</span><select aria-label={`${tool.name} 调用方式`} className="rounded-md border bg-background p-2 text-sm" value={tool.policy} onChange={event => setTools(tools.map((item, i) => i === index ? { ...item, policy: event.target.value as Policy } : item))}>{Object.entries(policyLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></div><p className="mt-2 whitespace-pre-wrap text-xs text-muted-foreground">{tool.description}</p></div>)}</div> : null}
      </>}
      <label className="flex gap-2 text-sm"><input type="checkbox" checked={enabled} onChange={event => setEnabled(event.target.checked)} />启用，允许 Agent 发现并使用</label>
    </fieldset>
    {error ? <p role="alert" className="text-sm text-destructive">{error}</p> : null}
    {deleting ? <div className="rounded-lg border p-3 text-sm"><p>删除“{name}”？后续 Agent 调用将不可用。</p><div className="mt-3 flex gap-2"><Button type="button" variant="destructive" disabled={busy} onClick={remove}>确认删除</Button><Button type="button" variant="outline" disabled={busy} onClick={() => setDeleting(false)}>取消删除</Button></div></div> : null}
    <div className="flex justify-end gap-2">{row && !readonly && !deleting ? <Button type="button" variant="destructive" disabled={busy} onClick={() => setDeleting(true)}>删除</Button> : null}<Button type="button" variant="outline" disabled={busy} onClick={onClose}>关闭</Button>{!readonly ? <Button type="submit" disabled={busy || deleting}>{busy ? "处理中…" : "保存"}</Button> : null}</div>
    </form>
  </DialogContent></Dialog>;
}
