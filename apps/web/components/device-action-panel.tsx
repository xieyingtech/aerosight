"use client";

import { apiFetch } from "@/lib/api-client";

import Link from "next/link";
import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { confirmationPhrase } from "@/lib/device-command-core";
import type { DeviceCapabilityAction } from "@/lib/device-capability-actions";

type ProjectedAction = DeviceCapabilityAction & { enabled?: boolean; unavailableReason?: string | null };

export function DeviceActionPanel({ projectId, deviceId, deviceName, actions, onChanged }: {
  projectId: number;
  deviceId: number;
  deviceName?: string;
  actions: ProjectedAction[];
  onChanged?: () => void | Promise<void>;
}) {
  const [reason, setReason] = useState("");
  const [selected, setSelected] = useState<ProjectedAction | null>(null);
  const [parameters, setParameters] = useState<Record<string, string>>({});
  const [status, setStatus] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  useEffect(() => { setSelected(null); setReason(""); setParameters({}); setStatus(null); }, [projectId, deviceId]);
  if (!actions.length) return null;

  const invoke = async (action: DeviceCapabilityAction, operationReason = reason) => {
    setPending(true);
    setStatus(null);
    try {
    const dynamicParameters = Object.fromEntries(action.fields
      .filter((field) => (parameters[`${action.capabilityCode}:${action.key}:${field.key}`] ?? "").trim() !== "")
      .map((field) => {
        const value = parameters[`${action.capabilityCode}:${action.key}:${field.key}`];
        return [field.key, field.type === "number" ? Number(value) : value];
      }));
    const response = await apiFetch(action.kind === "live"
      ? `/api/projects/${projectId}/devices/${deviceId}/live-streams`
      : `/api/projects/${projectId}/devices/${deviceId}/commands`, {
      method: "POST", headers: { "content-type": "application/json" },
      body: JSON.stringify(action.kind === "live" ? {} : {
        capabilityCode: action.capabilityCode, commandKey: action.key,
        parameters: { ...action.fixedParameters, ...dynamicParameters },
        idempotencyKey: crypto.randomUUID(), reason: operationReason.trim() || action.label,
        confirmation: ["high", "critical"].includes(action.risk) ? confirmationPhrase(deviceId, action.capabilityCode) : null
      })
    });
    const result = await response.json() as { error?: string; session?: { id: number; status: string }; id?: string; status?: string };
    setStatus(response.ok
      ? action.kind === "live" ? `直播 #${result.session?.id}：${result.session?.status}` : `命令 ${result.id}：${result.status}`
      : result.error ?? "操作失败");
    if (response.ok) { setSelected(null); await onChanged?.(); }
    } catch { setStatus("请求失败，请检查最新设备状态后重试。"); }
    finally { setPending(false); }
  };

  const choose = (action: ProjectedAction) => {
    setStatus(null); setReason(""); setParameters({});
    if (action.fields.length || ["high", "critical"].includes(action.risk)) setSelected(action);
    else void invoke(action, "");
  };
  const selectedAvailable = selected && actions.some(action => action.capabilityCode === selected.capabilityCode && action.key === selected.key && action.enabled !== false);
  return <div className="mt-3 space-y-2 rounded-lg border bg-muted/20 p-3">
    <div className="flex flex-wrap gap-2">
      {actions.map((action) => action.kind === "workflow"
        ? <Button asChild key={`${action.capabilityCode}:${action.key}`} size="sm" variant="outline"><Link href={`/projects/tasks/?projectId=${projectId}`}>{action.label}</Link></Button>
        : <Button disabled={action.enabled === false || pending} key={`${action.capabilityCode}:${action.key}`} onClick={() => choose(action)} size="sm" title={action.unavailableReason ?? undefined} variant={action.risk === "critical" ? "destructive" : "outline"}>{action.label}</Button>)}
    </div>
    {actions.some((action) => action.enabled === false && action.unavailableReason) && <div className="space-y-1 text-xs text-amber-700">
      {Array.from(new Set(actions.filter((action) => action.enabled === false).map((action) => action.unavailableReason).filter(Boolean))).map((message) => <p key={message}>{message}</p>)}
    </div>}
    <Dialog open={selected !== null} onOpenChange={(open) => { if (!open && !pending) setSelected(null); }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{selected?.label}</DialogTitle>
          <DialogDescription>即将对{deviceName || `设备 #${deviceId}`} 执行“{selected?.label}”。{selected && ["high", "critical"].includes(selected.risk) ? "此操作会影响设备运行，请确认现场条件允许后执行。" : "请填写本次操作的参数。"}</DialogDescription>
        </DialogHeader>
        <form onSubmit={(event) => { event.preventDefault(); if (selected && selectedAvailable && !pending) void invoke(selected); }} className="space-y-4">
          {selected?.fields.map((field) => {
            const key = `${selected.capabilityCode}:${selected.key}:${field.key}`;
            return <label className="grid gap-1 text-sm" key={key}>{field.label}{field.unit ? ` (${field.unit})` : ""}<Input disabled={pending} onChange={(event) => setParameters(current => ({ ...current, [key]: event.target.value }))} required={field.required} type={field.type} value={parameters[key] ?? ""} /></label>;
          })}
          <label className="grid gap-1 text-sm">操作备注（选填）<Input disabled={pending} onChange={event => setReason(event.target.value)} placeholder="填写本次操作的备注" value={reason} /></label>
          {status && <p role="status" className="text-sm text-muted-foreground">{status}</p>}
          {!selectedAvailable && <p className="text-sm text-destructive">设备状态已变化，请关闭弹窗后重新选择操作。</p>}
          <DialogFooter>
            <Button type="button" variant="outline" disabled={pending} onClick={() => setSelected(null)}>取消</Button>
            <Button type="submit" disabled={pending || !selectedAvailable} variant={selected && ["high", "critical"].includes(selected.risk) ? "destructive" : "default"}>{pending ? "正在提交…" : "确认执行"}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
    {status && <p className="text-xs text-muted-foreground">{status}</p>}
  </div>;
}
