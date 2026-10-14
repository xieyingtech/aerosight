"use client";

import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { APIError, apiJSON, type SessionUser } from "@/lib/api-client";
import { useUpdateSessionUser } from "@/components/session-provider";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";

export function NameSettingsForm({ name: initialName, projectId }: { name: string; projectId?: number }) {
  const [name, setName] = useState(initialName);
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const updateUser = useUpdateSessionUser();
  const label = projectId ? "项目名称" : "显示名称";
  const id = projectId ? "project-name" : "profile-name";
  function changeOpen(value: boolean) {
    if (pending) return;
    setName(initialName); setError(""); setOpen(value);
  }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending) return;
    const value = name.trim();
    if (!value || Array.from(value).length > 100) { setError(`请输入 1–100 个字符的${label}。`); return; }
    setPending(true); setError("");
    try {
      const result = await apiJSON<{ user?: SessionUser }>(projectId ? `/api/projects/${projectId}` : "/api/profile", { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name: value }) });
      setName(value); setOpen(false);
      if (result.user) updateUser(result.user);
      toast.success(`${label}已保存。`);
      if (projectId) window.dispatchEvent(new Event("aerosight:projects-updated"));
    } catch (error) {
      setError(error instanceof APIError && error.code === "PROJECT_ACCESS_DENIED" ? "你没有修改此项目名称的权限。" : `${label}保存失败，请重试。`);
    } finally { setPending(false); }
  }
  return <Dialog open={open} onOpenChange={changeOpen}>
    <DialogTrigger asChild><Button variant="outline">修改{label}</Button></DialogTrigger>
    <DialogContent showCloseButton={!pending}><DialogHeader><DialogTitle>修改{label}</DialogTitle><DialogDescription>{projectId ? "修改后，项目列表和导航会同步更新。" : "显示名称用于展示你的身份，登录仍使用原邮箱或手机号。"}</DialogDescription></DialogHeader>
    <form className="space-y-4" onSubmit={submit}>
      <div className="space-y-2"><Label htmlFor={id}>{label}</Label><Input id={id} value={name} disabled={pending} onChange={event => setName(event.target.value)} required aria-invalid={!!error} aria-describedby={error ? `${id}-error` : undefined} /></div>
      {error && <p id={`${id}-error`} role="alert" className="text-sm text-destructive">{error}</p>}
      <div className="flex flex-wrap items-center justify-between gap-3"><span className="text-xs text-muted-foreground">按 Enter 保存。</span><div className="flex gap-2"><Button variant="outline" disabled={pending} type="button" onClick={() => changeOpen(false)}>取消</Button><Button disabled={pending || name.trim() === initialName} type="submit">{pending ? "正在保存…" : `保存${label}`}</Button></div></div>
    </form>
  </DialogContent></Dialog>;
}
