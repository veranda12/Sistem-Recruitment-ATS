import { useState } from "react";
import { Link, NavLink, Navigate, Route, Routes, useNavigate } from "react-router";
import { useSession } from "../../lib/session";
import { Button, cx } from "../../ui";
import { ApplicationDetail, ApplicationsPage } from "./applications";
import { StaffCvResult } from "./cvresults";
import { CalendarPage } from "./calendar";
import { TemplatesPage } from "./templates";
import { AuditPage, CommsPage, SettingsPage, UsersPage } from "./admin";
import { BanksPage } from "./banks";
import { EssayGrading, ScheduleMonitor, SchedulesPage } from "./exams";
import { ReportsPage } from "./reports";
import { SecurityPage } from "./security";
import { RequisitionsPage } from "./interviews";
import { JobsPage } from "./jobs";
import { WorkflowEditor, WorkflowsPage } from "./workflows";

type NavItem = { to: string; label: string; perm: string };
const roleLabel: Record<string, string> = { super_admin: "Super Admin", hr_admin: "HR Admin", recruiter: "Recruiter", hiring_manager: "Hiring Manager", examiner: "Examiner" };

const items: NavItem[] = [
  { to: "/staf/lamaran", label: "Lamaran", perm: "applications.read" },
  { to: "/staf/kalender", label: "Kalender interview", perm: "applications.read" },
  { to: "/staf/lowongan", label: "Lowongan", perm: "jobs.read" },
  { to: "/staf/permintaan", label: "Permintaan tenaga kerja", perm: "requisitions.read" },
  { to: "/staf/workflow", label: "Workflow", perm: "workflows.read" },
  { to: "/staf/ujian", label: "Ujian", perm: "exams.manage" },
  { to: "/staf/bank-soal", label: "Bank soal", perm: "exams.manage" },
  { to: "/staf/laporan", label: "Laporan", perm: "reports.read" },
  { to: "/staf/pengguna", label: "Pengguna", perm: "users.manage" },
  { to: "/staf/pengaturan", label: "Pengaturan", perm: "settings.manage" },
  { to: "/staf/komunikasi", label: "Komunikasi", perm: "settings.manage" },
  { to: "/staf/template-pesan", label: "Template pesan", perm: "settings.manage" },
  { to: "/staf/audit", label: "Audit", perm: "audit.read" },
];

export default function BackofficeRoutes() {
  const { session, can, logout } = useSession();
  const nav = useNavigate();
  const [menu, setMenu] = useState(false);
  const mustEnroll = !!session?.mfa.required && !session.mfa.enabled;
  const visible = mustEnroll ? [] : items.filter((i) => can(i.perm));
  const first = visible[0]?.to;

  const sidebar = (
    <nav aria-label="Menu back office" className="flex flex-col gap-1">
      {[...visible, { to: "/staf/keamanan", label: "Keamanan akun", perm: "" }].map((i) => (
        <NavLink
          key={i.to}
          to={i.to}
          onClick={() => setMenu(false)}
          className={({ isActive }) => cx("flex min-h-11 items-center rounded-md px-3 font-medium", isActive ? "bg-primary-soft text-primary" : "text-ink hover:bg-white")}
        >
          {i.label}
        </NavLink>
      ))}
    </nav>
  );

  return (
    <div className="flex min-h-dvh flex-col bg-white">
      <a href="#konten" className="skip-link">
        Lewati ke konten
      </a>
      <header className="flex items-center justify-between gap-2 border-b border-line px-4 py-2">
        <div className="flex items-center gap-2">
          <Button variant="ghost" className="lg:hidden" aria-expanded={menu} aria-controls="menu-staf" onClick={() => setMenu((m) => !m)}>
            Menu
          </Button>
          <Link to="/staf" className="text-lg font-bold">
            Back office rekrutmen
          </Link>
        </div>
        <div className="flex items-center gap-2 text-sm">
          <span className="hidden text-ink-2 sm:inline">
            {session?.user.name} · {session?.roles.map((r) => roleLabel[r] ?? r).join(", ")}
          </span>
          <Button
            variant="ghost"
            onClick={async () => {
              await logout();
              nav("/masuk");
            }}
          >
            Keluar
          </Button>
        </div>
      </header>
      <div className="flex flex-1">
        <aside className="hidden w-56 shrink-0 border-r border-line bg-surface p-3 lg:block">{sidebar}</aside>
        {menu ? (
          <div id="menu-staf" className="fixed inset-x-0 top-[61px] z-30 border-b border-line bg-surface p-3 lg:hidden">
            {sidebar}
          </div>
        ) : null}
        <main id="konten" tabIndex={-1} className="min-w-0 flex-1 px-4 py-6 lg:px-8">
          {mustEnroll ? (
            <SecurityPage forced />
          ) : (
          <Routes>
            <Route index element={first ? <Navigate to={first} replace /> : <NoAccess />} />
            <Route path="keamanan" element={<SecurityPage />} />
            <Route path="lamaran" element={<ApplicationsPage />} />
            <Route path="lamaran/:id" element={<ApplicationDetail />} />
            <Route path="lamaran/:id/cv/:runId" element={<StaffCvResult />} />
            <Route path="kalender" element={<CalendarPage />} />
            <Route path="template-pesan" element={<TemplatesPage />} />
            <Route path="lowongan" element={<JobsPage />} />
            <Route path="permintaan" element={<RequisitionsPage />} />
            <Route path="workflow" element={<WorkflowsPage />} />
            <Route path="workflow/:id" element={<WorkflowEditor />} />
            <Route path="ujian" element={<SchedulesPage />} />
            <Route path="bank-soal" element={<BanksPage />} />
            <Route path="ujian/:id" element={<ScheduleMonitor />} />
            <Route path="penilaian/:id" element={<EssayGrading />} />
            <Route path="laporan" element={<ReportsPage />} />
            <Route path="pengguna" element={<UsersPage />} />
            <Route path="pengaturan" element={<SettingsPage />} />
            <Route path="komunikasi" element={<CommsPage />} />
            <Route path="audit" element={<AuditPage />} />
          </Routes>
          )}
        </main>
      </div>
    </div>
  );
}

function NoAccess() {
  return <p className="text-ink-2">Peran Anda belum memiliki menu di back office. Hubungi pengelola akses.</p>;
}
