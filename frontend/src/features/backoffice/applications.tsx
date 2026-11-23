import { useState } from "react";
import { Link, useParams, useSearchParams } from "react-router";
import { ApiError, get, post, type Schemas } from "../../lib/api";
import { fmtDate, fmtDateTime, statusLabel } from "../../lib/format";
import { useSession } from "../../lib/session";
import { useQuery } from "../../lib/useQuery";
import { kindLabel, openDocument, scanLabel } from "../candidate/documents";
import { InterviewSection } from "./interviews";
import { BoardView } from "./board";
import { Alert, Badge, Button, DL, Dialog, Field, PageHeader, PageState, Select, Textarea, useToast } from "../../ui";

type List = { items: Schemas["StaffApplication"][]; nextCursor: string | null };

export function ApplicationsPage() {
  const [params] = useSearchParams();
  const { can } = useSession();
  // Papan butuh daftar lowongan (jobs.read) dan isi lamarannya (applications.read).
  const canBoard = can("jobs.read") && can("applications.read");
  const board = params.get("tampilan") === "papan" && canBoard;
  return (
    <div>
      <PageHeader title="Lamaran" subtitle="Hanya lamaran dalam unit atau lowongan yang ditugaskan kepada Anda." />
      {canBoard ? (
        <nav aria-label="Tampilan lamaran" className="mb-4 flex gap-2">
          <Link to="/staf/lamaran" aria-current={!board ? "page" : undefined} className={viewTab(!board)}>
            Daftar
          </Link>
          <Link to="/staf/lamaran?tampilan=papan" aria-current={board ? "page" : undefined} className={viewTab(board)}>
            Papan
          </Link>
        </nav>
      ) : null}
      {board ? <BoardView /> : <ApplicationList />}
    </div>
  );
}

const viewTab = (active: boolean) =>
  `inline-flex min-h-11 items-center rounded-md border px-4 font-medium ${active ? "border-primary bg-primary-soft text-primary" : "border-line text-ink hover:bg-surface"}`;

function ApplicationList() {
  const [params, setParams] = useSearchParams();
  const status = params.get("status") ?? "";
  // Penanganan: semua, shortlist, atau lamaran yang ditugaskan ke pengguna ini.
  const handling = params.get("shortlisted") === "true" ? "shortlisted" : params.get("recruiter") === "me" ? "mine" : "";
  const filters: Record<string, string> = {};
  if (status) filters.status = status;
  if (handling === "shortlisted") filters.shortlisted = "true";
  if (handling === "mine") filters.recruiter = "me";
  const qs = new URLSearchParams(filters).toString();
  const setFilter = (next: Record<string, string>) => {
    setExtra([]);
    setCursor(undefined);
    setParams(Object.fromEntries(Object.entries(next).filter(([, v]) => v)));
  };
  const q = useQuery<List>(`/applications?${qs}`);
  const [extra, setExtra] = useState<Schemas["StaffApplication"][]>([]);
  const [cursor, setCursor] = useState<string | null | undefined>(undefined);
  const next = cursor === undefined ? q.data?.nextCursor : cursor;
  const rows = [...(q.data?.items ?? []), ...extra];
  const [more, setMore] = useState(false);

  return (
    <div>
      <div className="mb-4 flex flex-wrap items-end gap-3 rounded-md border border-line bg-surface p-3">
        <label className="flex flex-col gap-1 text-sm font-semibold">
          Status
          <Select value={status} onChange={(e) => setFilter({ ...filters, status: e.target.value })}>
            <option value="">Semua status</option>
            {["active", "withdrawn", "rejected", "hired"].map((s) => (
              <option key={s} value={s}>
                {statusLabel[s]}
              </option>
            ))}
          </Select>
        </label>
        <label className="flex flex-col gap-1 text-sm font-semibold">
          Penanganan
          <Select
            value={handling}
            onChange={(e) =>
              setFilter({ status, ...(e.target.value === "shortlisted" ? { shortlisted: "true" } : e.target.value === "mine" ? { recruiter: "me" } : {}) })
            }
          >
            <option value="">Semua lamaran</option>
            <option value="shortlisted">Shortlist</option>
            <option value="mine">Ditugaskan ke saya</option>
          </Select>
        </label>
      </div>
      <PageState
        loading={q.loading}
        error={q.error}
        onRetry={q.reload}
        empty={rows.length === 0}
        emptyText={status || handling ? "Tidak ada lamaran yang cocok dengan filter ini." : "Belum ada lamaran pada lowongan Anda."}
      >
        {/* Desktop: tabel padat. Mobile: susunan label/nilai. */}
        <div className="hidden overflow-x-auto md:block">
          <table className="w-full border-collapse text-left text-[15px]">
            <caption className="sr-only">Daftar lamaran</caption>
            <thead>
              <tr className="border-b border-line text-sm text-ink-2">
                <th scope="col" className="py-2 pr-4 font-semibold">Kandidat</th>
                <th scope="col" className="py-2 pr-4 font-semibold">Lowongan</th>
                <th scope="col" className="py-2 pr-4 font-semibold">Tahap</th>
                <th scope="col" className="py-2 pr-4 font-semibold">Status</th>
                <th scope="col" className="py-2 pr-4 font-semibold">Recruiter</th>
                <th scope="col" className="py-2 font-semibold">Dikirim</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((a) => (
                <tr key={a.id} className="border-b border-line">
                  <td className="py-3 pr-4">
                    <Link className="font-semibold text-primary hover:underline" to={`/staf/lamaran/${a.id}`}>
                      {a.candidateName}
                    </Link>
                    {a.shortlisted ? <span className="ml-2 rounded-sm bg-primary-soft px-1.5 py-0.5 text-[13px] font-semibold text-primary">Shortlist</span> : null}
                  </td>
                  <td className="py-3 pr-4">{a.jobTitle}</td>
                  <td className="py-3 pr-4">{a.stageName}</td>
                  <td className="py-3 pr-4">
                    <Badge status={a.status}>{statusLabel[a.status]}</Badge>
                  </td>
                  <td className="py-3 pr-4">{a.recruiterName ?? "-"}</td>
                  <td className="py-3">{fmtDate(a.appliedAt)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <ul className="flex flex-col gap-3 md:hidden">
          {rows.map((a) => (
            <li key={a.id} className="rounded-md border border-line p-4">
              <Link className="text-[16px] font-semibold text-primary" to={`/staf/lamaran/${a.id}`}>
                {a.candidateName}
              </Link>
              <div className="mt-2">
                <DL
                  items={[
                    ["Lowongan", a.jobTitle],
                    ["Tahap", a.stageName],
                    ["Status", statusLabel[a.status] ?? a.status],
                    ["Recruiter", a.recruiterName ?? "-"],
                    ["Shortlist", a.shortlisted ? "Ya" : "Tidak"],
                    ["Dikirim", fmtDate(a.appliedAt)],
                  ]}
                />
              </div>
            </li>
          ))}
        </ul>
        {next ? (
          <div className="mt-4">
            <Button
              busy={more}
              onClick={async () => {
                setMore(true);
                try {
                  const d = await get<List>(`/applications?${qs}${qs ? "&" : ""}cursor=${encodeURIComponent(next)}`);
                  setExtra((x) => [...x, ...d.items]);
                  setCursor(d.nextCursor);
                } finally {
                  setMore(false);
                }
              }}
            >
              Muat lebih banyak
            </Button>
          </div>
        ) : null}
      </PageState>
    </div>
  );
}

type Action = Schemas["ApplicationDetail"]["actions"][number];

const cvStatusLabel: Record<string, string> = { ready_for_review: "belum disimpan kandidat", partial: "sebagian terbaca", applied: "diterapkan ke profil" };

export function ApplicationDetail() {
  const { id = "" } = useParams();
  const q = useQuery<Schemas["ApplicationDetail"]>(`/applications/${id}`);
  const toast = useToast();
  const { can } = useSession();
  const [action, setAction] = useState<Action | null>(null);
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<ApiError | null>(null);
  const [revealOpen, setRevealOpen] = useState(false);
  const [nik, setNik] = useState<string | null>(null);
  const d = q.data;
  const [assignOpen, setAssignOpen] = useState(false);
  const [assignee, setAssignee] = useState("");
  const recruiters = useQuery<{ items: { id: string; displayName: string }[] }>(assignOpen ? `/applications/${id}/assignable-recruiters` : null);

  const toggleShortlist = async () => {
    if (!d) return;
    setErr(null);
    try {
      await post(`/applications/${id}/shortlist`, { shortlisted: !d.shortlisted, expectedVersion: d.version });
      toast("success", d.shortlisted ? "Dilepas dari shortlist." : "Ditambahkan ke shortlist.");
      q.reload();
    } catch (e) {
      setErr(e as ApiError);
    }
  };
  const assign = async () => {
    if (!d) return;
    setBusy(true);
    setErr(null);
    try {
      await post(`/applications/${id}/recruiter`, { recruiterUserId: assignee || null, expectedVersion: d.version });
      toast("success", assignee ? "Recruiter ditugaskan." : "Penugasan recruiter dilepas.");
      setAssignOpen(false);
      q.reload();
    } catch (e) {
      setErr(e as ApiError);
    } finally {
      setBusy(false);
    }
  };

  const run = async () => {
    if (!d || !action) return;
    setBusy(true);
    setErr(null);
    try {
      if (action.kind === "approve" || action.kind === "decline") {
        await post(`/applications/${id}/approvals`, { decision: action.kind === "approve" ? "approve" : "reject", comment: reason, expectedVersion: d.version });
      } else {
        await post(`/applications/${id}/transitions`, { kind: action.kind, to: action.to, reason, expectedVersion: d.version });
      }
      toast("success", "Perubahan tersimpan.");
      setAction(null);
      setReason("");
      q.reload();
    } catch (e) {
      setErr(e as ApiError);
    } finally {
      setBusy(false);
    }
  };

  const reveal = async () => {
    setBusy(true);
    setErr(null);
    try {
      const r = await post<{ nik: string }>(`/applications/${id}/reveal-identity`, { reason });
      setNik(r.nik);
      setRevealOpen(false);
      setReason("");
    } catch (e) {
      setErr(e as ApiError);
    } finally {
      setBusy(false);
    }
  };

  const stageName = (k: string | null) => d?.stages.find((s) => s.key === k)?.name ?? k ?? "-";
  const triggerLabel: Record<string, string> = { apply: "Melamar", manual: "Dipindahkan", reject: "Ditolak", withdraw: "Ditarik kandidat", event: "Hasil ujian", approval: "Persetujuan" };
  const p = d?.snapshot.profile;
  const docs = d?.documents ?? [];

  return (
    <PageState loading={q.loading} error={q.error} onRetry={q.reload}>
      {d && p ? (
        <div className="max-w-5xl">
          <Link to="/staf/lamaran" className="text-sm text-primary hover:underline">
            Kembali ke daftar lamaran
          </Link>
          <PageHeader
            title={d.candidate.name}
            subtitle={
              <span className="inline-flex flex-wrap items-center gap-2">
                {d.jobTitle} · {d.unitName} · <Badge status={d.status}>{statusLabel[d.status]}</Badge> Tahap: {d.stage.name}
              </span>
            }
          />
          {d.actions.length > 0 ? (
            <section aria-label="Tindakan" className="mb-6 flex flex-wrap gap-2">
              {d.actions.map((a, i) => (
                <Button
                  key={i}
                  variant={a.kind === "reject" || a.kind === "decline" ? "secondary" : i === 0 ? "primary" : "secondary"}
                  onClick={() => {
                    setErr(null);
                    setReason("");
                    setAction(a);
                  }}
                >
                  {a.kind === "manual" ? `${a.label}: ke ${stageName(a.to ?? null)}` : a.label}
                </Button>
              ))}
            </section>
          ) : (
            <div className="mb-6">
              <p className="text-ink-2">{d.status === "active" ? "Tidak ada tindakan yang tersedia untuk peran Anda pada tahap ini." : "Lamaran sudah tidak aktif."}</p>
              {(d.status === "hired" || d.status === "rejected") && can("applications.transition") ? (
                d.decisionNotifiedAt ? (
                  <p className="mt-2 text-sm text-success">Pemberitahuan keputusan sudah dikirim {fmtDateTime(d.decisionNotifiedAt)}.</p>
                ) : (
                  <Button
                    className="mt-2"
                    variant="primary"
                    onClick={async () => {
                      try {
                        await post(`/applications/${id}/decision-notice`);
                        toast("success", "Pemberitahuan keputusan dikirim ke kandidat.");
                        q.reload();
                      } catch (e) {
                        setErr(e as ApiError);
                      }
                    }}
                  >
                    Kirim pemberitahuan keputusan
                  </Button>
                )
              ) : null}
            </div>
          )}

          <section aria-label="Penanganan" className="mb-6 flex flex-wrap items-center gap-x-4 gap-y-2 rounded-md border border-line bg-surface px-4 py-3 text-[15px]">
            <span>
              Recruiter: <span className="font-semibold">{d.recruiter?.name ?? "Belum ditugaskan"}</span>
            </span>
            {d.canAssign ? (
              <Button
                size="sm"
                onClick={() => {
                  setErr(null);
                  setAssignee(d.recruiter?.id ?? "");
                  setAssignOpen(true);
                }}
              >
                Ubah recruiter
              </Button>
            ) : null}
            <span className="text-ink-2">·</span>
            <span>{d.shortlisted ? "Masuk shortlist" : "Belum di-shortlist"}</span>
            {d.canShortlist ? (
              <Button size="sm" onClick={toggleShortlist}>
                {d.shortlisted ? "Lepas dari shortlist" : "Tambahkan ke shortlist"}
              </Button>
            ) : null}
          </section>
          {err && !action && !revealOpen && !assignOpen ? (
            <div className="mb-4">
              <Alert tone="danger" title={err.message} />
            </div>
          ) : null}

          <div className="grid gap-8 lg:grid-cols-[3fr_2fr]">
            <section>
              <h2 className="mb-3 text-[20px]">Data saat melamar</h2>
              <p className="mb-3 text-sm text-ink-2">Salinan profil pada {fmtDateTime(d.snapshotAt)}. Perubahan profil setelahnya tidak memengaruhi data ini.</p>
              <DL
                items={[
                  ["Email", d.snapshot.email],
                  [
                    "NIK",
                    <span className="inline-flex flex-wrap items-center gap-2">
                      {nik ?? d.snapshot.nikMasked}
                      {d.canRevealIdentity && !nik ? (
                        <Button size="sm" onClick={() => setRevealOpen(true)}>
                          Tampilkan NIK
                        </Button>
                      ) : null}
                    </span>,
                  ],
                  ["Telepon", p.phone || "-"],
                  ["Tanggal lahir", fmtDate(p.birthDate)],
                  ["Domisili", p.city || "-"],
                  ["Pendidikan", p.education.map((e) => `${e.level} ${e.institution}${e.major ? `, ${e.major}` : ""}`).join("; ") || "-"],
                  ["Pengalaman", p.experiences.map((e) => `${e.title} di ${e.company}`).join("; ") || "-"],
                  ["Dikirim", fmtDateTime(d.appliedAt)],
                ]}
              />
              <div className="mt-6">
                <h3 className="mb-2 font-semibold">Dokumen kandidat</h3>
                {docs.length === 0 ? (
                  <p className="text-ink-2">Kandidat belum mengunggah dokumen.</p>
                ) : (
                  <ul className="text-[15px]">
                    {docs.map((doc) => (
                      <li key={doc.id} className="flex flex-wrap items-center gap-2 py-1">
                        {kindLabel[doc.kind]}: {doc.fileName} · {scanLabel[doc.scanStatus] ?? doc.scanStatus}
                        {doc.scanStatus === "clean" ? (
                          <Button size="sm" onClick={() => openDocument(doc.id).catch((e: ApiError) => setErr(e))}>
                            Unduh
                          </Button>
                        ) : null}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
              {d.cvParses.length > 0 ? (
                <div className="mt-6">
                  <h3 className="mb-2 font-semibold">Hasil pembacaan CV</h3>
                  <ul className="text-[15px]">
                    {d.cvParses.map((r) => (
                      <li key={r.id} className="flex flex-wrap items-center gap-2 py-1">
                        {r.documentName} · {cvStatusLabel[r.status] ?? r.status}
                        <Link className="text-sm text-primary underline" to={`/staf/lamaran/${d.id}/cv/${r.id}`}>
                          Lihat hasil dan sumbernya
                        </Link>
                      </li>
                    ))}
                  </ul>
                </div>
              ) : null}
              <InterviewSection
                applicationId={d.id}
                interviews={d.interviews}
                evaluations={d.evaluations}
                canSchedule={d.canSchedule}
                onChange={q.reload}
              />
              {d.otherApplications.length > 0 || d.hiddenOtherApplications > 0 ? (
                <div className="mt-6">
                  <h3 className="mb-2 font-semibold">Lamaran lain kandidat ini</h3>
                  <ul className="list-disc pl-5 text-[15px]">
                    {d.otherApplications.map((o) => (
                      <li key={o.id}>
                        <Link className="text-primary underline" to={`/staf/lamaran/${o.id}`}>
                          {o.jobTitle}
                        </Link>{" "}
                        ({statusLabel[o.status] ?? o.status}, {fmtDate(o.appliedAt)})
                      </li>
                    ))}
                    {d.hiddenOtherApplications > 0 ? <li>{d.hiddenOtherApplications} lamaran di luar akses Anda</li> : null}
                  </ul>
                </div>
              ) : null}
            </section>
            <section>
              <h2 className="mb-3 text-[20px]">Riwayat tahap</h2>
              <ol className="border-l-2 border-line pl-5">
                {d.history.map((h) => (
                  <li key={h.id} className="relative pb-5">
                    <span className="absolute -left-[27px] top-1.5 h-3 w-3 rounded-full border-2 border-primary bg-white" aria-hidden />
                    <p className="font-semibold">
                      {triggerLabel[h.trigger] ?? h.trigger}: {h.fromStage ? `${stageName(h.fromStage)} ke ` : ""}
                      {stageName(h.toStage)}
                      {h.toStatus !== "active" ? ` (${statusLabel[h.toStatus] ?? h.toStatus})` : ""}
                    </p>
                    <p className="text-sm text-ink-2">
                      {fmtDateTime(h.createdAt)} · {h.actorName ?? (h.actorKind === "system" ? "Sistem" : "-")}
                    </p>
                    {h.reason ? <p className="mt-1 text-[15px]">Alasan: {h.reason}</p> : null}
                  </li>
                ))}
              </ol>
              {d.approvals.length > 0 ? (
                <>
                  <h3 className="mb-2 mt-4 font-semibold">Persetujuan</h3>
                  <ul className="text-[15px]">
                    {d.approvals.map((a) => (
                      <li key={a.id}>
                        {a.approverName}: {a.decision === "approve" ? "Setuju" : "Tidak setuju"} · {fmtDateTime(a.createdAt)}
                        {a.invalidatedAt ? " (tidak berlaku)" : ""}
                      </li>
                    ))}
                  </ul>
                </>
              ) : null}
            </section>
          </div>

          <Dialog
            open={!!action}
            onOpenChange={(o) => !o && setAction(null)}
            title={action?.kind === "manual" ? `Pindahkan ke ${stageName(action.to ?? null)}?` : action?.kind === "reject" ? "Tolak lamaran?" : (action?.label ?? "")}
            description="Tindakan ini dicatat beserta nama Anda dan alasannya."
          >
            {err ? (
              <Alert tone="danger" title={err.message} action={err.code === "stale_version" ? <Button onClick={() => { setAction(null); q.reload(); }}>Muat ulang</Button> : null} />
            ) : null}
            <Field label={action?.kind === "reject" ? "Alasan penolakan" : "Catatan"} required={action?.kind === "reject"} error={err?.fieldErrors.reason}>
              {({ id: fid, invalid }) => <Textarea id={fid} invalid={invalid} value={reason} maxLength={1000} onChange={(e) => setReason(e.target.value)} />}
            </Field>
            <div className="mt-4 flex justify-end gap-2">
              <Button onClick={() => setAction(null)}>Batal</Button>
              <Button variant={action?.kind === "reject" ? "danger" : "primary"} busy={busy} onClick={run}>
                Konfirmasi
              </Button>
            </div>
          </Dialog>

          <Dialog open={assignOpen} onOpenChange={setAssignOpen} title="Ubah recruiter" description="Recruiter penanggung jawab lamaran ini. Perubahan dicatat di audit.">
            {err && assignOpen ? <Alert tone="danger">{err.fieldErrors.recruiterUserId ?? err.message}</Alert> : null}
            <PageState loading={recruiters.loading} error={recruiters.error} onRetry={recruiters.reload}>
              <Field label="Recruiter">
                {({ id: fid }) => (
                  <Select id={fid} value={assignee} onChange={(e) => setAssignee(e.target.value)}>
                    <option value="">Tanpa recruiter</option>
                    {recruiters.data?.items.map((r) => (
                      <option key={r.id} value={r.id}>
                        {r.displayName}
                      </option>
                    ))}
                  </Select>
                )}
              </Field>
            </PageState>
            <div className="mt-4 flex justify-end gap-2">
              <Button onClick={() => setAssignOpen(false)}>Batal</Button>
              <Button variant="primary" busy={busy} onClick={assign}>
                Simpan
              </Button>
            </div>
          </Dialog>

          <Dialog open={revealOpen} onOpenChange={setRevealOpen} title="Tampilkan NIK" description="Akses ke NIK lengkap dicatat di audit bersama alasan Anda.">
            {err && revealOpen ? <Alert tone="danger">{err.fieldErrors.reason ?? err.message}</Alert> : null}
            <Field label="Alasan" required hint="Minimal 10 karakter.">
              {({ id: fid, describedBy }) => <Textarea id={fid} aria-describedby={describedBy} value={reason} onChange={(e) => setReason(e.target.value)} />}
            </Field>
            <div className="mt-4 flex justify-end gap-2">
              <Button onClick={() => setRevealOpen(false)}>Batal</Button>
              <Button variant="primary" busy={busy} onClick={reveal}>
                Tampilkan
              </Button>
            </div>
          </Dialog>
        </div>
      ) : null}
    </PageState>
  );
}
