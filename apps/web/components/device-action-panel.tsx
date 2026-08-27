"use client";

import Link from "next/link";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { DeviceCapabilityAction } from "@/lib/device-capability-actions";

export function DeviceActionPanel({ projectId, deviceId, actions }: { projectId: number; deviceId: number; actions: DeviceCapabilityAction[] }) {
  const [reason, setReason] = useState("本地演示操作");
  const [confirmation, setConfirmation] = useState("");
  const [parameters, setParameters] = useState<Record<string, string>>({});
  const [status, setStatus] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  if (!actions.length) return null;

  const invoke = async (action: DeviceCapabilityAction) => {
    setPending(true);
    setStatus(null);
    const dynamicParameters = Object.fromEntries(action.fields
      .filter((field) => parameters[`${action.capabilityCode}:${action.key}:${field.key}`] !== "")
      .map((field) => {
        const value = parameters[`${action.capabilityCode}:${action.key}:${field.key}`];
        return [field.key, field.type === "number" ? Number(value) : value];
      }));
    const response = await fetch(action.kind === "live"
      ? `/api/projects/${projectId}/devices/${deviceId}/live-streams`
      : `/api/projects/${projectId}/devices/${deviceId}/commands`, {
      method: "POST", headers: { "content-type": "application/json" },
      body: JSON.stringify(action.kind === "live" ? {} : {
        capabilityCode: action.capabilityCode, commandKey: action.key,
        parameters: { ...action.fixedParameters, ...dynamicParameters },
        idempotencyKey: crypto.randomUUID(), reason,
        confirmation: ["high", "critical"].includes(action.risk) ? confirmation : null
      })
    });
    const result = await response.json() as { error?: string; session?: { id: number; status: string }; id?: string; status?: string };
    setPending(false);
    setStatus(response.ok
      ? action.kind === "live" ? `直播 #${result.session?.id}：${result.session?.status}` : `命令 ${result.id}：${result.status}`
      : result.error ?? "操作失败");
  };

  const requiresConfirmation = actions.some((action) => ["high", "critical"].includes(action.risk));
  return <div className="mt-3 space-y-2 rounded-lg border bg-muted/20 p-3">
    <div className="flex flex-wrap gap-2">
      {actions.map((action) => action.kind === "workflow"
        ? <Button asChild key={`${action.capabilityCode}:${action.key}`} size="sm" variant="outline"><Link href={`/projects/${projectId}/tasks`}>{action.label}</Link></Button>
        : <Button disabled={pending || !reason.trim() || (["high", "critical"].includes(action.risk) && confirmation !== `CONFIRM ${deviceId} ${action.capabilityCode}`)} key={`${action.capabilityCode}:${action.key}`} onClick={() => invoke(action)} size="sm" variant={action.risk === "critical" ? "destructive" : "outline"}>{action.label}</Button>)}
    </div>
    {actions.flatMap((action) => action.fields.map((field) => {
      const key = `${action.capabilityCode}:${action.key}:${field.key}`;
      return <label className="grid gap-1 text-xs" key={key}>{field.label}{field.unit ? ` (${field.unit})` : ""}<Input onChange={(event) => setParameters((current) => ({ ...current, [key]: event.target.value }))} required={field.required} type={field.type} value={parameters[key] ?? ""} /></label>;
    }))}
    <Input aria-label={`设备 ${deviceId} 操作原因`} onChange={(event) => setReason(event.target.value)} placeholder="操作原因" value={reason} />
    {requiresConfirmation && <Input aria-label={`设备 ${deviceId} 二次确认`} onChange={(event) => setConfirmation(event.target.value)} placeholder={`高风险操作请输入：CONFIRM ${deviceId} capability.code`} value={confirmation} />}
    {status && <p className="text-xs text-muted-foreground">{status}</p>}
  </div>;
}
