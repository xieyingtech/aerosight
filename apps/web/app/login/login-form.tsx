"use client";

import { useActionState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { APIError, getSession, login } from "@/lib/api-client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function LoginForm() {
  const router = useRouter();
  useEffect(() => {
    const controller = new AbortController();
    getSession(controller.signal).then(() => { if (!controller.signal.aborted) router.replace("/projects"); }).catch(() => {});
    return () => controller.abort();
  }, [router]);
  const [state, action, pending] = useActionState(async (_: { error?: string }, form: FormData): Promise<{ error?: string }> => {
    try { await login(String(form.get("username") ?? ""), String(form.get("password") ?? "")); router.replace("/projects"); return {}; }
    catch (error) { return { error: error instanceof APIError && error.status === 401 ? "邮箱、手机号或密码错误" : "登录失败，请稍后重试。" }; }
  }, {});

  return (
    <form action={action} className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="username">邮箱或手机号</Label>
        <Input autoComplete="username" id="username" name="username" required />
      </div>
      <div className="space-y-2">
        <Label htmlFor="password">密码</Label>
        <Input autoComplete="current-password" id="password" name="password" required type="password" />
      </div>
      {state.error ? <p className="text-sm text-destructive">{state.error}</p> : null}
      <Button className="w-full" disabled={pending} size="lg" type="submit">
        登录
      </Button>
    </form>
  );
}
