"use client";

import { useId, useState } from "react";
import { iconNames } from "lucide-react/dynamic";
import { APIStateView } from "@/components/api-state";
import { DeviceTypeIcon } from "@/components/device-type-icon";
import { Page } from "@/components/page";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { apiJSON } from "@/lib/api-client";
import { useAPI } from "@/lib/use-api";

type TypePresentation = { id: string; typeKey: string; version: number; displayName: string; icon: string; status: string };

function TypeRow({ type, onChanged }: { type: TypePresentation; onChanged: () => void }) {
  const [icon, setIcon] = useState(type.icon);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const listId = useId();
  const valid = (iconNames as readonly string[]).includes(icon);
  async function save() {
    setBusy(true); setError(""); setSaved(false);
    try {
      await apiJSON(`/api/admin/device-types/${type.id}`, { method: "PATCH", body: JSON.stringify({ icon }) });
      setSaved(true); onChanged();
    } catch { setError("图标保存失败，请重试。"); } finally { setBusy(false); }
  }
  return <Card size="sm"><CardHeader><CardTitle className="flex items-center gap-2"><DeviceTypeIcon name={icon} className="size-5" />{type.displayName}</CardTitle><p className="text-xs text-muted-foreground">{type.typeKey} · v{type.version}</p></CardHeader><CardContent className="space-y-2">
    <div className="flex gap-2"><Input aria-label={`${type.displayName}图标`} list={listId} value={icon} onChange={event => { setIcon(event.target.value); setSaved(false); }} /><Button size="sm" disabled={busy || !valid || icon === type.icon} onClick={save}>{busy ? "保存中" : "保存"}</Button></div>
    <datalist id={listId}>{iconNames.filter(name => name.includes(icon)).slice(0, 80).map(name => <option key={name} value={name} />)}</datalist>
    {!valid && <p className="text-xs text-destructive">请选择有效的 Lucide 图标名称。</p>}
    {error && <p role="alert" className="text-xs text-destructive">{error}</p>}
    {saved && <p role="status" className="text-xs text-muted-foreground">已保存</p>}
  </CardContent></Card>;
}

export default function DeviceTypesPage() {
  const state = useAPI<TypePresentation[]>("/api/admin/device-types");
  return <Page title="设备类型" description="为设备类型配置统一图标，地图标点与设备筛选使用同一份定义。"><APIStateView state={state}>{types => <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">{types.map(type => <TypeRow key={type.id} type={type} onChanged={state.reload} />)}{!types.length && <p className="text-sm text-muted-foreground">暂无设备类型。</p>}</div>}</APIStateView></Page>;
}
