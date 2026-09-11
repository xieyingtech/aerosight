"use client";

import { useEffect, useRef, useState } from "react";
import { ChevronRight, FolderTree } from "lucide-react";
import { Button } from "@/components/ui/button";
import { apiFetch } from "@/lib/api-client";
import { featureChanges, featureLeaves, featureSelection, type FeatureNode, type FeatureSettings, type FeatureValues } from "@/lib/project-features";

function FeatureCheckbox({ node, values, disabled, onToggle }: {
  node: FeatureNode; values: FeatureValues; disabled: boolean; onToggle: (node: FeatureNode, enabled: boolean) => void;
}) {
  const selection = featureSelection(node, values);
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => { if (ref.current) ref.current.indeterminate = selection === "some"; }, [selection]);
  return <input ref={ref} type="checkbox" className="size-4 shrink-0 accent-primary" aria-label={`${node.label}功能开关`}
    aria-checked={selection === "some" ? "mixed" : selection === "all"} checked={selection === "all"} disabled={disabled}
    onChange={(event) => onToggle(node, event.target.checked)} />;
}

function FeatureBranch({ node, values, disabled, onToggle }: {
  node: FeatureNode; values: FeatureValues; disabled: boolean; onToggle: (node: FeatureNode, enabled: boolean) => void;
}) {
  const [open, setOpen] = useState(true);
  const children = node.children;
  const count = featureLeaves(node).filter(id => values[id]).length;
  return <li>
    <div className="flex items-start gap-3 rounded-md px-3 py-3 hover:bg-muted/40">
      {children ? <button type="button" className="mt-0.5 shrink-0" aria-expanded={open} aria-label={`${open ? "收起" : "展开"}${node.label}`} onClick={() => setOpen(!open)}>
        <ChevronRight className={`size-4 transition-transform ${open ? "rotate-90" : ""}`} />
      </button> : <span className="w-4 shrink-0" />}
      <div className="pt-0.5"><FeatureCheckbox node={node} values={values} disabled={disabled} onToggle={onToggle} /></div>
      <div className="min-w-0 flex-1"><span className={`text-sm ${children ? "font-semibold" : "font-medium"}`}>{node.label}</span>
        {node.description && <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{node.description}</p>}
      </div>
      <span className="shrink-0 text-xs text-muted-foreground">{children ? `${count}/${featureLeaves(node).length} 已启用` : values[node.id] ? "已启用" : "未启用"}</span>
    </div>
    {children && open && <ul className="ml-5 border-l pl-3">{children.map(child => <FeatureBranch key={child.id} node={child} values={values} disabled={disabled} onToggle={onToggle} />)}</ul>}
  </li>;
}

export function ProjectFeatureSettings({ data }: { data: FeatureSettings }) {
  const [saved, setSaved] = useState(data.values);
  const [draft, setDraft] = useState(data.values);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const pending = Object.keys(featureChanges(saved, draft)).length;
  const toggle = (node: FeatureNode, enabled: boolean) => {
    setMessage(""); setError("");
    setDraft(current => ({ ...current, ...Object.fromEntries(featureLeaves(node).map(id => [id, enabled])) }));
  };
  async function save() {
    setBusy(true); setError(""); setMessage("");
    try {
      const response = await apiFetch(`/api/projects/${data.projectId}/feature-settings`, {
        method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ changes: featureChanges(saved, draft) }),
      });
      if (response.status === 409) { setError("其他管理员已修改这些开关，请重新加载后再保存。"); return; }
      if (response.status === 403) { setError("只有项目所有者和管理员可以修改功能开关。"); return; }
      if (!response.ok) throw new Error("save failed");
      const result = await response.json() as { values: FeatureValues };
      setSaved(result.values); setDraft(result.values); setMessage("项目功能开关已保存。");
    } catch { setError("保存失败，请稍后重试。"); }
    finally { setBusy(false); }
  }
  async function refresh() {
    if (pending && !window.confirm("重新加载会丢弃尚未保存的修改，确认重新加载？")) return;
    setBusy(true); setError(""); setMessage("");
    try {
      const response = await apiFetch(`/api/projects/${data.projectId}/feature-settings`, { cache: "no-store" });
      if (!response.ok) throw new Error("reload failed");
      const result = await response.json() as FeatureSettings;
      setSaved(result.values); setDraft(result.values);
    } catch { setError("重新加载失败，请稍后重试。"); }
    finally { setBusy(false); }
  }
  return <section className="mx-auto w-full max-w-4xl space-y-4">
    <div className="rounded-lg border bg-card p-5">
      <div className="flex items-center gap-2"><FolderTree className="size-5" /><h2 className="font-semibold">项目功能</h2></div>
      <p className="mt-2 text-sm text-muted-foreground">勾选分组可批量调整其下功能，保存后对整个项目生效。成员按预设角色使用功能，这里不分配个人权限。</p>
      <p className="mt-2 text-sm text-muted-foreground">启用开关不会直接执行操作。飞行控制等功能仍需完成能力验证；开启开关不代表设备已就绪。</p>
    </div>
    <div className="rounded-lg border bg-card p-2 sm:p-4"><ul className="space-y-3">{data.tree.map(node => <FeatureBranch key={node.id} node={node} values={draft} disabled={busy} onToggle={toggle} />)}</ul></div>
    <div className="sticky bottom-0 flex flex-wrap items-center justify-between gap-3 rounded-lg border bg-background p-4">
      <div className="text-sm">{error ? <p role="alert" className="text-destructive">{error}</p> : message ? <p role="status">{message}</p> : <span className="text-muted-foreground">{pending ? `${pending} 项修改待保存` : "所有修改已保存"}</span>}</div>
      <div className="flex gap-2"><Button type="button" variant="outline" disabled={busy} onClick={() => void refresh()}>重新加载</Button>
        <Button type="button" disabled={busy || !pending} onClick={() => void save()}>{busy ? "处理中…" : "保存修改"}</Button></div>
    </div>
  </section>;
}
