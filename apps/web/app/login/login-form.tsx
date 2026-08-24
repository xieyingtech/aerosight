"use client";

import { useActionState } from "react";
import { loginAction } from "@/app/actions";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function LoginForm() {
  const [state, action, pending] = useActionState(loginAction, {});

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
