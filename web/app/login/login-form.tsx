"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { browserApi } from "@/lib/browser-api";
import { t } from "@/lib/i18n";

export function LoginForm() {
  const router = useRouter();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);

  return (
    <form
      className="mt-5 space-y-4"
      onSubmit={async (event) => {
        event.preventDefault();
        setPending(true);
        setError("");
        const key = username.includes("@") ? "email" : "phone";
        const response = await browserApi("/api/auth/login", {
          method: "POST",
          body: JSON.stringify({ [key]: username, password })
        });
        setPending(false);
        if (!response.ok) {
          const body = await response.json().catch(() => ({ message: "errors.generic" }));
          setError(t(body.message));
          return;
        }
        router.push("/projects");
        router.refresh();
      }}
    >
      <label className="block text-sm">
        <span className="font-medium">邮箱或手机号</span>
        <input className="mt-1 h-10 w-full rounded-md border border-slate-300 px-3" value={username} onChange={(event) => setUsername(event.target.value)} />
      </label>
      <label className="block text-sm">
        <span className="font-medium">密码</span>
        <input className="mt-1 h-10 w-full rounded-md border border-slate-300 px-3" type="password" value={password} onChange={(event) => setPassword(event.target.value)} />
      </label>
      {error ? <p className="text-sm text-red-600">{error}</p> : null}
      <button className="h-10 w-full rounded-md bg-sky-700 text-sm font-medium text-white disabled:opacity-60" disabled={pending} type="submit">
        登录
      </button>
    </form>
  );
}
