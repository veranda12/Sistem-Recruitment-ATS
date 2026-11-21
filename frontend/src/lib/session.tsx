import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { ApiError, get, post, setCsrf, type Session } from "./api";

type SessionState = {
  session: Session | null;
  loading: boolean;
  refresh: () => Promise<Session | null>;
  login: (email: string, password: string, code?: string) => Promise<Session>;
  logout: () => Promise<void>;
  can: (perm: string) => boolean;
};

const Ctx = createContext<SessionState | null>(null);

/** Hook dipanggil saat logout/ganti akun agar draft lokal (mis. jawaban ujian) dibersihkan. */
const logoutHooks = new Set<() => Promise<void> | void>();
export function onLogout(fn: () => Promise<void> | void) {
  logoutHooks.add(fn);
  return () => logoutHooks.delete(fn);
}

export function SessionProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(null);
  const [loading, setLoading] = useState(true);

  const apply = useCallback((s: Session | null) => {
    setSession(s);
    setCsrf(s?.csrfToken ?? "");
    return s;
  }, []);

  const refresh = useCallback(async () => {
    try {
      return apply(await get<Session>("/auth/session"));
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) return apply(null);
      throw e;
    } finally {
      setLoading(false);
    }
  }, [apply]);

  useEffect(() => {
    refresh().catch(() => setLoading(false));
  }, [refresh]);

  const login = useCallback(
    async (email: string, password: string, code?: string) => {
      const prev = session?.user.id;
      const s = await post<Session>("/auth/login", { email, password, code });
      if (prev && prev !== s.user.id) await Promise.all([...logoutHooks].map((f) => f()));
      return apply(s)!;
    },
    [apply, session],
  );

  const logout = useCallback(async () => {
    try {
      await post("/auth/logout");
    } finally {
      await Promise.all([...logoutHooks].map((f) => f()));
      apply(null);
    }
  }, [apply]);

  const value = useMemo<SessionState>(
    () => ({ session, loading, refresh, login, logout, can: (p) => !!session?.permissions.includes(p) }),
    [session, loading, refresh, login, logout],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useSession() {
  const v = useContext(Ctx);
  if (!v) throw new Error("useSession di luar SessionProvider");
  return v;
}
