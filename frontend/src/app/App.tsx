import { lazy, Suspense, useEffect, type ReactNode } from "react";
import { BrowserRouter, Navigate, Route, Routes, useLocation } from "react-router";
import { onLogout, SessionProvider, useSession } from "../lib/session";
import { ToastProvider } from "../ui";
import { PublicLayout } from "./PublicLayout";

// Bundle dipisah per area: karier publik, akun kandidat, back office.
const Career = lazy(() => import("../features/career/routes"));
const AuthPages = lazy(() => import("../features/auth/routes"));
const Candidate = lazy(() => import("../features/candidate/routes"));
const Backoffice = lazy(() => import("../features/backoffice/routes"));
const Exam = lazy(() => import("../features/exam/routes"));

// Draft ujian lokal dihapus saat logout atau ganti akun (perangkat bersama).
onLogout(
  () =>
    new Promise<void>((resolve) => {
      try {
        const r = indexedDB.deleteDatabase("ats-exam");
        r.onsuccess = r.onerror = r.onblocked = () => resolve();
      } catch {
        resolve();
      }
    }),
);

function Loading() {
  return (
    <p role="status" className="p-8 text-ink-2">
      Memuat halaman...
    </p>
  );
}

/** Rute terlindungi: arahkan ke login dengan parameter kembali (deep link tetap terjaga). */
function RequireKind({ kind, children }: { kind: "candidate" | "staff"; children: ReactNode }) {
  const { session, loading } = useSession();
  const loc = useLocation();
  if (loading) return <Loading />;
  if (!session) return <Navigate to={`/masuk?next=${encodeURIComponent(loc.pathname + loc.search)}`} replace />;
  if (session.user.kind !== kind) return <Navigate to={session.user.kind === "staff" ? "/staf" : "/akun"} replace />;
  return <>{children}</>;
}

function ScrollToTop() {
  const { pathname } = useLocation();
  useEffect(() => window.scrollTo(0, 0), [pathname]);
  return null;
}

export function App() {
  return (
    <BrowserRouter>
      <SessionProvider>
        <ToastProvider>
          <ScrollToTop />
          <Suspense fallback={<Loading />}>
            <Routes>
              <Route
                path="/staf/*"
                element={
                  <RequireKind kind="staff">
                    <Backoffice />
                  </RequireKind>
                }
              />
              <Route
                path="/ujian/*"
                element={
                  <RequireKind kind="candidate">
                    <Exam />
                  </RequireKind>
                }
              />
              <Route element={<PublicLayout />}>
                <Route
                  path="/akun/*"
                  element={
                    <RequireKind kind="candidate">
                      <Candidate />
                    </RequireKind>
                  }
                />
                <Route path="/*" element={<PublicRoutes />} />
              </Route>
            </Routes>
          </Suspense>
        </ToastProvider>
      </SessionProvider>
    </BrowserRouter>
  );
}

function PublicRoutes() {
  const { pathname } = useLocation();
  const authPaths = ["/masuk", "/daftar", "/verifikasi-email", "/lupa-sandi", "/atur-ulang-sandi"];
  return authPaths.some((p) => pathname.startsWith(p)) ? <AuthPages /> : <Career />;
}
