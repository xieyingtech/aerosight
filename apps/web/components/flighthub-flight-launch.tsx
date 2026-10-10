"use client";

import { useEffect, useState } from "react";
import { apiFetch } from "@/lib/api-client";
import { projectPageHref } from "@/lib/page-routes";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";

type Options = { waylines: { id: number; name: string }[]; canExecute: boolean };
type Receipt = { status: string; accepted: boolean; runId: number; remoteStatus?: string; error?: string; refreshError?: string };
const messages: Record<string, string> = {
 queued: "正在检查设备与航线", prepared: "检查通过，准备下发", reconciling: "正在核对司空回执", succeeded: "司空已受理，等待飞行状态",
 waiting: "等待执行", executing: "执行中", success: "任务已完成", failed: "下发失败", blocked: "结果待核实",
 starting_failure: "启动失败", terminated: "任务已终止", timeout: "任务超时",
 wayline_requires_two_waypoints: "航线至少需要两个航点，请在司空修改后重试。",
 flight_device_not_ready_or_model_mismatch: "机场当前未就绪，或航线与飞行器型号不匹配。",
 dispatch_check_warning: "司空下发检查未通过，请检查设备状态与航线。",
 upstream_error: "司空拒绝请求，请查看任务记录。",
};

export function FlightHubFlightLaunch({ projectId, deviceId, deviceName }: { projectId: number; deviceId: number; deviceName: string }) {
 const endpoint = `/api/projects/${projectId}/devices/${deviceId}/flight-launch`;
 const storageKey = `aerosight:flight-launch:${projectId}:${deviceId}`;
 const [open, setOpen] = useState(false), [options, setOptions] = useState<Options | null>(null);
 const [route, setRoute] = useState(""), [name, setName] = useState(""), [precision, setPrecision] = useState("gps"), [height, setHeight] = useState("50");
 const [key, setKey] = useState(""), [watchKey, setWatchKey] = useState(""), [pending, setPending] = useState(false), [receipt, setReceipt] = useState<Receipt | null>(null), [error, setError] = useState("");
 useEffect(() => { setWatchKey(sessionStorage.getItem(storageKey) ?? ""); }, [storageKey]);
 useEffect(() => {
  if (!watchKey) return;
  let disposed = false, busy = false;
  async function poll() {
   if (busy) return; busy = true;
   try {
    const response = await apiFetch(`${endpoint}?idempotencyKey=${encodeURIComponent(watchKey)}`, { cache: "no-store" });
   if (!response.ok) { if (!disposed) setError("正在核对请求结果，请勿另行下发。可使用原请求重试。"); return; }
    const next: Receipt = await response.json(); if (disposed) return;
    setReceipt(next); setError(next.refreshError ?? "");
    const terminal = ["success", "starting_failure", "terminated", "timeout", "partially_done"].includes(next.remoteStatus ?? "") || ["failed", "blocked"].includes(next.status);
    if (terminal) { sessionStorage.removeItem(storageKey); setWatchKey(""); }
    if (next.accepted && !terminal && new URLSearchParams(window.location.search).get("flightLaunchKey") !== watchKey) {
     window.location.assign(projectPageHref(projectId, "realtime", { deviceId, autoLive: 1, runId: next.runId, flightLaunchKey: watchKey }));
    }
   } catch { if (!disposed) setError("回执读取失败，正在继续核对，请勿重复下发。"); }
   finally { busy = false; }
  }
  void poll(); const timer = setInterval(() => void poll(), 5000);
  return () => { disposed = true; clearInterval(timer); };
 }, [watchKey, endpoint, storageKey, projectId, deviceId]);
 async function show() {
  setOpen(true); setError(""); setOptions(null); setKey(crypto.randomUUID());
  try { const response = await apiFetch(endpoint, { cache: "no-store" }); if (!response.ok) throw new Error("航线读取失败"); setOptions(await response.json()); }
  catch (cause) { setError(cause instanceof Error ? cause.message : "航线读取失败"); }
 }
 async function launch() {
  setPending(true); setError("");
  sessionStorage.setItem(storageKey, key); setWatchKey(key);
  try {
   const response = await apiFetch(endpoint, { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ waylineResourceId: Number(route), name, waylinePrecisionType: precision, rthAltitude: Number(height), idempotencyKey: key }) });
   const data = await response.json();
   if (!response.ok) {
    if (response.status < 500) { sessionStorage.removeItem(storageKey); setWatchKey(""); }
    throw new Error(data.error?.code === "idempotency_conflict" ? "该请求参数已改变，请重新打开弹窗。" : `下发失败：${data.error?.code ?? response.status}`);
   }
   sessionStorage.setItem(storageKey, key); setWatchKey(key); setReceipt({ ...data, accepted: false }); setOpen(false);
  } catch (cause) { setError(cause instanceof Error ? cause.message : "请求结果未知，请保留当前弹窗重试核对。"); }
  finally { setPending(false); }
 }
 return <>
  <Button size="sm" variant="outline" disabled={pending || Boolean(watchKey)} onClick={() => void show()}>执行航线</Button>
  {receipt && <p role="status" className="basis-full text-sm text-muted-foreground">{messages[receipt.remoteStatus ?? receipt.status] ?? receipt.remoteStatus ?? receipt.status}<span className="inline-block whitespace-pre-line">{receipt.error && `\n${messages[receipt.error] ?? receipt.error}`}</span></p>}
  {!open && error && <p role="alert" className="basis-full text-sm text-destructive">{error}</p>}
  <Dialog open={open} onOpenChange={value => { if (!pending) setOpen(value); }}><DialogContent><DialogHeader><DialogTitle>执行航线</DialogTitle><DialogDescription>由 {deviceName} 执行所选航线。点击起飞后立即向司空下发任务。</DialogDescription></DialogHeader>
   <form className="space-y-4" onSubmit={event => { event.preventDefault(); void launch(); }}>
    <label className="grid gap-1 text-sm">航线<select className="h-9 rounded-md border bg-background px-3" required disabled={pending || !options} value={route} onChange={event => { setRoute(event.target.value); setName(options?.waylines.find(line => String(line.id) === event.target.value)?.name ?? ""); }}><option value="">选择司空航线</option>{options?.waylines.map(line => <option key={line.id} value={line.id}>{line.name}</option>)}</select></label>
    {options?.waylines.length === 0 && <p className="text-sm text-muted-foreground">尚无同步的航线，请先在司空创建航线并同步连接器。</p>}
    <label className="grid gap-1 text-sm">任务名称<Input required maxLength={200} value={name} disabled={pending} onChange={event => setName(event.target.value)}/></label>
    <div className="grid grid-cols-2 gap-3"><label className="grid gap-1 text-sm">定位方式<select className="h-9 rounded-md border bg-background px-3" value={precision} disabled={pending} onChange={event => setPrecision(event.target.value)}><option value="gps">GNSS</option><option value="rtk">RTK</option></select></label><label className="grid gap-1 text-sm">返航高度（米）<Input type="number" required min={20} max={500} step={1} value={height} disabled={pending} onChange={event => setHeight(event.target.value)}/></label></div>
    {options && !options.canExecute && <p className="text-sm text-muted-foreground">当前账号没有飞行操作权限，或项目尚未开启飞行功能。</p>}
    {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
    <DialogFooter><Button type="button" variant="outline" disabled={pending} onClick={() => setOpen(false)}>取消</Button><Button disabled={pending || !options?.canExecute || !route || !name.trim()} type="submit">{pending ? "正在下发…" : "起飞"}</Button></DialogFooter>
   </form>
  </DialogContent></Dialog>
 </>;
}
