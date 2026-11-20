import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError, get } from "./api";

export type QueryState<T> = {
  data: T | null;
  error: ApiError | null;
  loading: boolean;
  reload: () => void;
};

/** Pengambilan data sederhana dengan pembatalan saat path berubah. path null = tidak memuat. */
export function useQuery<T>(path: string | null): QueryState<T> {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [loading, setLoading] = useState(path !== null);
  const [tick, setTick] = useState(0);
  const latest = useRef(0);

  useEffect(() => {
    if (path === null) return;
    const ctrl = new AbortController();
    const id = ++latest.current;
    setLoading(true);
    setError(null);
    get<T>(path, ctrl.signal)
      .then((d) => {
        if (id === latest.current) setData(d);
      })
      .catch((e: unknown) => {
        if (id !== latest.current || (e as Error).name === "AbortError") return;
        setError(e instanceof ApiError ? e : new ApiError(0, null, null));
      })
      .finally(() => {
        if (id === latest.current) setLoading(false);
      });
    return () => ctrl.abort();
  }, [path, tick]);

  const reload = useCallback(() => setTick((t) => t + 1), []);
  return { data, error, loading, reload };
}
