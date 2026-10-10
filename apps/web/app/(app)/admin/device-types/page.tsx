"use client";

import { useState } from "react";
import { APIStateView } from "@/components/api-state";
import { DeviceTypeIcon } from "@/components/device-type-icon";
import { Page } from "@/components/page";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { useAPI } from "@/lib/use-api";

type Definition = Record<string, unknown>;
type Catalog = { drivers: Definition[]; capabilities: Definition[]; streams: Definition[]; actions: Definition[] };
type Tab = "types" | keyof Catalog;
const tabs: [Tab, string][] = [["types", "设备类型"], ["drivers", "驱动"], ["capabilities", "能力"], ["streams", "数据通道"], ["actions", "操作"]];
const names: Record<string, string> = { active: "启用", retired: "已停用", disabled: "停用", aircraft: "飞行器", dock: "机巢", camera: "摄像头", sensor: "传感器", unknown: "未知", read: "读取", command: "控制", stream: "数据流", workflow: "工作流", low: "低", medium: "中", high: "高", critical: "关键", telemetry: "遥测", video: "视频", audio: "音频", events: "事件" };
const text = (value: unknown): string => value == null || value === "" ? "—" : Array.isArray(value) ? value.map(text).join("、") || "—" : typeof value === "object" ? JSON.stringify(value) : String(value);
const label = (value: unknown) => names[text(value)] ?? text(value);
function profileCount(row: Definition) {
  const profile = row.capabilityProfile as Record<string, unknown> | undefined;
  if (!profile) return 0;
  const capabilities = typeof profile.capabilities === "object" && profile.capabilities !== null ? profile.capabilities : profile;
  return Object.keys(capabilities).length;
}
const driver = (row: Definition) => `${text(row.driverKey)}@v${text(row.driverVersion).replace(/^v/, "")}`;
const columns: Record<Tab, { title: string; value: (row: Definition) => string }[]> = {
  types: [ { title: "名称", value: row => text(row.displayName) }, { title: "类型键 / 版本", value: row => `${text(row.typeKey)}@v${text(row.version).replace(/^v/, "")}` }, { title: "分类 / 型号", value: row => `${label(row.category)}${row.model ? `
型号：${text(row.model)}` : ""}${row.vendor ? `
厂商：${text(row.vendor)}` : ""}` }, { title: "驱动", value: driver }, { title: "能力", value: row => `${profileCount(row)} 项` }, { title: "状态", value: row => label(row.status) } ],
  drivers: [ { title: "名称", value: row => text(row.displayName) }, { title: "驱动键", value: row => text(row.driverKey) }, { title: "版本", value: row => text(row.version) }, { title: "协议", value: row => text((row.manifest as Definition)?.protocols) }, { title: "状态", value: row => label(row.status) } ],
  capabilities: [ { title: "能力键", value: row => text(row.code) }, { title: "类型", value: row => label(row.kind) }, { title: "风险", value: row => label(row.risk) }, { title: "来源驱动", value: driver }, { title: "驱动状态", value: row => label(row.driverStatus) } ],
  streams: [ { title: "通道键", value: row => text(row.channelKey) }, { title: "数据类型", value: row => label(row.dataType) }, { title: "关联能力", value: row => text(row.capabilityCode) }, { title: "来源驱动", value: driver }, { title: "单位", value: row => text(row.unit) } ],
  actions: [ { title: "名称", value: row => text(row.label) }, { title: "操作键", value: row => text(row.key) }, { title: "类型", value: row => label(row.kind) }, { title: "关联能力", value: row => text(row.capabilityCode) }, { title: "来源", value: () => "平台操作目录" } ],
};

export default function DeviceTypesPage() {
  const types = useAPI<Definition[]>("/api/admin/device-types");
  const catalog = useAPI<Catalog>("/api/admin/device-types/catalog");
  const [tab, setTab] = useState<Tab>("types");
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<Definition | null>(null);
  const rows = tab === "types" ? types.data ?? [] : catalog.data?.[tab] ?? [];
  const filtered = rows.filter(row => JSON.stringify(row).toLowerCase().includes(search.trim().toLowerCase()));
  const heading = tabs.find(([key]) => key === tab)?.[1] ?? "设备类型";
  function table() {
    return <div className="space-y-3"><p className="text-xs text-muted-foreground">{filtered.length} 条{search.trim() ? `匹配记录，共 ${rows.length} 条` : "记录"}</p><div className="overflow-x-auto rounded-xl border"><table className="w-full text-left text-sm"><thead className="bg-muted/50"><tr>{columns[tab].map(column => <th key={column.title} className="whitespace-nowrap px-4 py-3 font-medium">{column.title}</th>)}<th className="px-4 py-3 text-right">详情</th></tr></thead><tbody className="divide-y">{filtered.map((row, index) => <tr key={`${tab}-${index}`} className="cursor-pointer hover:bg-muted/40" onClick={() => setSelected(row)}>{columns[tab].map((column, i) => <td key={column.title} className={`px-4 py-3 ${i === 0 ? "font-medium" : "text-muted-foreground"}`}><span className="flex items-center gap-2">{tab === "types" && i === 0 ? <DeviceTypeIcon name={text(row.icon)} className="size-4 shrink-0" /> : null}<span className="whitespace-pre-line break-words">{column.value(row)}</span></span></td>)}<td className="px-4 py-3 text-right"><Button size="sm" variant="ghost" aria-label={`查看 ${text(row.displayName ?? row.code ?? row.channelKey ?? row.label)} 详情`} onClick={event => { event.stopPropagation(); setSelected(row); }}>查看</Button></td></tr>)}{!filtered.length ? <tr><td colSpan={columns[tab].length + 1} className="p-10 text-center text-muted-foreground">{search.trim() ? "没有匹配的定义。" : "暂无定义。"}</td></tr> : null}</tbody></table></div></div>;
  }
  return <Page title="设备类型" description="查阅平台设备类型及关联的驱动、能力、数据通道和操作定义。">
    <div role="tablist" aria-label="设备目录" className="flex flex-wrap gap-1 border-b">{tabs.map(([key, title]) => <button key={key} id={`catalog-tab-${key}`} role="tab" aria-selected={tab === key} aria-controls={`catalog-panel-${key}`} onClick={() => { setTab(key); setSelected(null); }} className={`border-b-2 px-4 py-3 text-sm font-medium ${tab === key ? "border-primary text-primary" : "border-transparent text-muted-foreground hover:text-foreground"}`}>{title}</button>)}</div>
    <Input aria-label="搜索目录" value={search} onChange={event => setSearch(event.target.value)} placeholder="搜索名称、键、驱动或定义字段…" className="w-full" />
    <p className="text-xs text-muted-foreground">目录展示声明的定义；具体设备是否可用，还取决于连接状态、固件、配置和当前用户权限。点击记录查看关联键和参数定义。</p>
    <div role="tabpanel" id={`catalog-panel-${tab}`} aria-labelledby={`catalog-tab-${tab}`}>{tab === "types" ? <APIStateView state={types}>{() => table()}</APIStateView> : <APIStateView state={catalog}>{() => table()}</APIStateView>}</div>
    <Dialog open={!!selected} onOpenChange={open => { if (!open) setSelected(null); }}><DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-3xl"><DialogHeader><DialogTitle><span className="inline-flex max-w-full flex-wrap items-center gap-x-3 gap-y-1"><span>{heading}</span><span>{text(selected?.displayName ?? selected?.code ?? selected?.channelKey ?? selected?.label)}</span></span></DialogTitle><DialogDescription>目录定义与关联字段，仅供查阅。</DialogDescription></DialogHeader><pre className="overflow-x-auto whitespace-pre-wrap break-all rounded-lg bg-muted p-4 text-xs leading-6">{JSON.stringify(selected, null, 2)}</pre><div className="flex justify-end"><Button variant="outline" onClick={() => setSelected(null)}>关闭</Button></div></DialogContent></Dialog>
  </Page>;
}
