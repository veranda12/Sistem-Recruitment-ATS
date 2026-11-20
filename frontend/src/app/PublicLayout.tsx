import { useEffect, useState } from "react";
import { Link, NavLink, Outlet, useNavigate } from "react-router";
import type { Schemas } from "../lib/api";
import { setOrgTimeZone } from "../lib/format";
import { useSession } from "../lib/session";
import { useQuery } from "../lib/useQuery";
import { Button, cx } from "../ui";

const navCls = ({ isActive }: { isActive: boolean }) =>
  cx("inline-flex min-h-11 items-center px-2 text-[15px] font-medium", isActive ? "text-primary underline underline-offset-8" : "text-ink hover:text-primary");

/** Layout portal karier dan ruang kandidat: navigasi horizontal. */
export function PublicLayout() {
  const org = useQuery<Schemas["PublicOrg"]>("/org");
  const { session, logout } = useSession();
  const nav = useNavigate();
  const [online, setOnline] = useState(navigator.onLine);
  useEffect(() => {
    const on = () => setOnline(true);
    const off = () => setOnline(false);
    window.addEventListener("online", on);
    window.addEventListener("offline", off);
    return () => {
      window.removeEventListener("online", on);
      window.removeEventListener("offline", off);
    };
  }, []);
  useEffect(() => {
    if (org.data) {
      setOrgTimeZone(org.data.timezone);
      document.title = `Karier ${org.data.name}`;
    }
  }, [org.data]);
  const brand = org.data?.brand.displayName ?? "Portal Karier";

  return (
    <div className="flex min-h-dvh flex-col">
      <a href="#konten" className="skip-link">
        Lewati ke konten
      </a>
      <header className="border-b border-line bg-white">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-2 px-4 py-2">
          <Link to="/" className="inline-flex min-h-11 items-center text-lg font-bold text-ink">
            {brand}
          </Link>
          <nav aria-label="Navigasi utama" className="flex flex-wrap items-center gap-1">
            <NavLink to="/" end className={navCls}>
              Lowongan
            </NavLink>
            {session?.user.kind === "candidate" ? (
              <>
                <NavLink to="/akun" end className={navCls}>
                  Lamaran saya
                </NavLink>
                <NavLink to="/akun/ujian" className={navCls}>
                  Ujian
                </NavLink>
                <NavLink to="/akun/profil" className={navCls}>
                  Profil
                </NavLink>
                <NavLink to="/akun/notifikasi" className={navCls}>
                  Kotak masuk
                </NavLink>
              </>
            ) : null}
            {session?.user.kind === "staff" ? (
              <NavLink to="/staf" className={navCls}>
                Back office
              </NavLink>
            ) : null}
            {session ? (
              <Button
                variant="ghost"
                onClick={async () => {
                  await logout();
                  nav("/");
                }}
              >
                Keluar
              </Button>
            ) : (
              <>
                <NavLink to="/masuk" className={navCls}>
                  Masuk
                </NavLink>
                <Link to="/daftar" className="inline-flex min-h-11 items-center rounded-md bg-primary px-4 font-semibold text-white hover:bg-primary-hover">
                  Daftar
                </Link>
              </>
            )}
          </nav>
        </div>
      </header>
      {!online ? (
        <div role="status" className="bg-warning-soft px-4 py-2 text-center text-sm text-warning">
          Anda sedang offline. Perubahan belum dapat dikirim.
        </div>
      ) : null}
      <main id="konten" tabIndex={-1} className="mx-auto w-full max-w-6xl flex-1 px-4 py-8">
        <Outlet />
      </main>
      <footer className="border-t border-line bg-surface">
        <div className="mx-auto max-w-6xl px-4 py-6 text-sm text-ink-2">
          {org.data?.name ?? ""} · Data pribadi diproses sesuai pemberitahuan privasi yang Anda setujui saat mendaftar.
        </div>
      </footer>
    </div>
  );
}
