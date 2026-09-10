"use client";

import { useCallback, useEffect, useState } from "react";
import { apiJSON } from "./api-client";

export type APIState<T> = { data: T | undefined; error: Error | null; loading: boolean; reload: () => void };

export function useAPI<T>(path: string | null): APIState<T> {
  const [revision, setRevision] = useState(0);
  const [state, setState] = useState<{ path: string | null; revision: number; data?: T; error: Error | null; loading: boolean }>({ path: null, revision: -1, error: null, loading: true });
  const reload = useCallback(() => setRevision((value) => value + 1), []);

  useEffect(() => {
    if (!path) return;
    const controller = new AbortController();
    setState({ path, revision, error: null, loading: true });
    apiJSON<T>(path, { signal: controller.signal }).then(
      (data) => { if (!controller.signal.aborted) setState({ path, revision, data, error: null, loading: false }); },
      (error: unknown) => {
        if (!controller.signal.aborted) setState({ path, revision, error: error instanceof Error ? error : new Error("API_FAILED"), loading: false });
      }
    );
    return () => controller.abort();
  }, [path, revision]);

  // Do not render a previous project's data while the next effect is pending.
  if (!path || state.path !== path || state.revision !== revision) return { data: undefined, error: null, loading: path !== null, reload };
  return { data: state.data, error: state.error, loading: state.loading, reload };
}
