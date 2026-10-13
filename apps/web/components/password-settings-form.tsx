"use client";

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { APIError, apiClient, apiJSON } from "@/lib/api-client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";

export function PasswordSettingsForm() {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  function changeOpen(value: boolean) {
    if (pending) return;
    setCurrentPassword(""); setNewPassword(""); setConfirmPassword(""); setError(""); setOpen(value);
  }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending) return;
    setError("");
    if (newPassword !== confirmPassword) { setError("两次输入的新密码不一致。"); return; }
    if (Array.from(newPassword).length < 8 || new TextEncoder().encode(newPassword).length > 72) { setError("新密码至少需要 8 个字符，且不能超过 72 字节。"); return; }
    if (newPassword === currentPassword) { setError("新密码不能与原密码相同。"); return; }
    setPending(true);
    try {
      await apiJSON<void>("/api/auth/password", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ currentPassword, newPassword }) });
      setCurrentPassword(""); setNewPassword(""); setConfirmPassword("");
      apiClient.invalidateCSRF();
      toast.success("密码已修改，请使用新密码重新登录。");
      router.replace("/login");
    } catch (error) {
      const code = error instanceof APIError ? error.code : "";
      setError(code === "CURRENT_PASSWORD_INVALID" ? "原密码不正确，请重新输入。" : code === "PASSWORD_INPUT_INVALID" ? "新密码至少需要 8 个字符，最多 72 字节，且不能与原密码相同。" : code === "PASSWORD_CHANGE_CONFLICT" ? "密码已在其他请求中修改，请重新登录后再试。" : error instanceof APIError && error.status === 429 ? "尝试次数过多，请稍后重试。" : "密码修改失败，请重试。");
      setPending(false);
    }
  }
  return <Dialog open={open} onOpenChange={changeOpen}>
    <DialogTrigger asChild><Button variant="outline">修改密码</Button></DialogTrigger>
    <DialogContent showCloseButton={!pending}><DialogHeader><DialogTitle>修改密码</DialogTitle><DialogDescription>新密码至少需要 8 个字符，最多 72 字节。修改成功后，所有设备上的旧登录状态将失效，请重新登录。</DialogDescription></DialogHeader>
    <form className="space-y-4" onSubmit={submit}>
      <div className="space-y-2"><Label htmlFor="current-password">原密码</Label><Input id="current-password" type="password" autoComplete="current-password" value={currentPassword} onChange={event => setCurrentPassword(event.target.value)} disabled={pending} required /></div>
      <div className="space-y-2"><Label htmlFor="new-password">新密码</Label><Input id="new-password" type="password" autoComplete="new-password" value={newPassword} onChange={event => setNewPassword(event.target.value)} disabled={pending} required /></div>
      <div className="space-y-2"><Label htmlFor="confirm-password">确认新密码</Label><Input id="confirm-password" type="password" autoComplete="new-password" value={confirmPassword} onChange={event => setConfirmPassword(event.target.value)} disabled={pending} required /></div>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <div className="flex flex-wrap items-center justify-between gap-3"><span className="text-xs text-muted-foreground">按 Enter 提交。</span><div className="flex gap-2"><Button type="button" variant="outline" disabled={pending} onClick={() => changeOpen(false)}>取消</Button><Button type="submit" disabled={pending}>{pending ? "正在修改…" : "确认修改"}</Button></div></div>
    </form>
  </DialogContent></Dialog>;
}
