"use client";

import { Suspense, type ReactNode } from "react";
import { usePathname, useSearchParams } from "next/navigation";
import { scopedPageQuery } from "@/lib/page-routes";
import { useAPI } from "@/lib/use-api";
import { APIStateView } from "@/components/api-state";

export type PageQuery = Pick<URLSearchParams, "get" | "getAll" | "has" | "toString">;
export function positiveParam(query: PageQuery, key = "projectId"): number | null {
  const value = query.get(key);
  if (!value || !/^[1-9]\d*$/.test(value)) return null;
  const id = Number(value);
  return Number.isSafeInteger(id) && id <= 2147483647 ? id : null;
}
export function uuidParam(query: PageQuery, key: string): string | null {
  const value = query.get(key);
  return value && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value) ? value : null;
}
type Props<T> = { endpoint: (query: PageQuery) => string | null; children: (data: T, query: PageQuery, reload: () => void) => ReactNode };
export function usePageQuery() {
  const search = useSearchParams();
  const pathname = usePathname();
  return scopedPageQuery(pathname, search.toString());
}
function Content<T>({ endpoint, children }: Props<T>) {
  const query = usePageQuery();
  const path = endpoint(query);
  const state = useAPI<T>(path);
  if (!path) return <p className="p-4 text-sm text-destructive" role="alert">无法打开此页面，请从列表重新进入。</p>;
  return <APIStateView state={state}>{(data) => children(data, query, state.reload)}</APIStateView>;
}
export function StaticAPIPage<T>(props: Props<T>) {
  return <Suspense fallback={<p className="p-4 text-sm text-muted-foreground" role="status">正在加载…</p>}><Content {...props} /></Suspense>;
}
