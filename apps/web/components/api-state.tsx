"use client";

import type { ReactNode } from "react";
import { APIError } from "@/lib/api-client";
import type { APIState } from "@/lib/use-api";
import { Button } from "@/components/ui/button";

export function APIStateView<T>({ state, children }: { state: APIState<T>; children: (data: T) => ReactNode }) {
  if (state.loading) return <p className="p-4 text-sm text-muted-foreground" role="status">正在加载…</p>;
  if (state.error) {
    const status = state.error instanceof APIError ? state.error.status : 0;
    const message = status === 401 ? "登录已失效，请重新登录。" : status === 403 ? "你没有访问此内容的权限。" : status === 404 ? "内容不存在或已无法访问。" : "加载失败，请重试。";
    return <div className="space-y-3 p-4" role="alert"><p className="text-sm text-destructive">{message}</p><Button onClick={state.reload} variant="outline">重试</Button></div>;
  }
  if (state.data === undefined) return null;
  return children(state.data);
}
