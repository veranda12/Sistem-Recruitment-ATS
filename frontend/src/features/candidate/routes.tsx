import { lazy, Suspense, useEffect, useState } from "react";
import { Link, Route, Routes, useNavigate, useParams, useSearchParams } from "react-router";
import { ApiError, post, put, type Schemas } from "../../lib/api";
import { fmtDate, fmtDateTime, statusLabel } from "../../lib/format";
import { useSession } from "../../lib/session";
import { useQuery } from "../../lib/useQuery";
import { DocumentsSection } from "./documents";
import { CvImport } from "./cv/CvImport";
import { fmtPartial } from "./cv/model";

const CvReviewPage = lazy(() => import("./cv/CvReview"));
import { Alert, Badge, Button, Dialog, Field, Input, PageHeader, PageState, Select, Textarea, useToast } from "../../ui";

export default function CandidateRoutes() {
  return (
    <Routes>
      <Route index element={<MyApplications />} />
      <Route path="profil" element={<ProfilePage />} />
      <Route
        path="profil/cv/:id"
        element={
          <Suspense fallback={<p role="status">Memuat...</p>}>
            <CvReviewPage />
          </Suspense>
        }
      />
      <Route path="lamaran/:id" element={<ApplicationPage />} />
      <Route path="notifikasi" element={<Inbox />} />
      <Route path="ujian" element={<ExamInvitations />} />
    </Routes>
  );
}

function VerifyBanner() {
  const { session } = useSession();
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  if (!session || session.user.emailVerified) return null;
  return (
    <div className="mb-6">
      <Alert
        tone="warning"
        title="Email belum diverifikasi"
        action={
          <Button
            busy={busy}
            onClick={async () => {
              setBusy(true);
              try {
                await post("/auth/resend-verification");
                toast("success", "Tautan verifikasi dikirim ulang.");
              } catch (e) {
                toast("danger", (e as ApiError).message);
              } finally {
                setBusy(false);
              }
            }}
          >
            Kirim ulang tautan
          </Button>
        }
      >
        Anda perlu memverifikasi email sebelum melamar.
      </Alert>
    </div>
  );
}

function MyApplications() {
  const q = useQuery<{ items: Schemas["MyApplication"][] }>("/me/applications");
  return (
    <div>
      <PageHeader title="Lamaran saya" subtitle="Status setiap lamaran yang sudah Anda kirim." />
      <VerifyBanner />
      <PageState loading={q.loading} error={q.error} onRetry={q.reload} empty={q.data?.items.length === 0} emptyText="Anda belum mengirim lamaran.">
        <ul className="divide-y divide-line border-y border-line">
          {q.data?.items.map((a) => (
            <li key={a.id} className="flex flex-wrap items-center justify-between gap-3 py-4">
              <div>
                <Link to={`/akun/lamaran/${a.id}`} className="text-[17px] font-semibold text-primary hover:underline">
                  {a.jobTitle}
                </Link>
                <p className="text-sm text-ink-2">
                  Dikirim {fmtDate(a.appliedAt)} · Tahap: {a.stageName}
                </p>
              </div>
              <Badge status={a.status}>{statusLabel[a.status]}</Badge>
            </li>
          ))}
        </ul>
      </PageState>
      <p className="mt-6">
        <Link to="/" className="font-semibold text-primary underline">
          Cari lowongan lain
        </Link>
      </p>
    </div>
  );
}

const fieldLabel: Record<string, string> = { phone: "Nomor telepon", birthDate: "Tanggal lahir", city: "Kota domisili", education: "Riwayat pendidikan" };
type ProfileInput = Schemas["ProfileInput"];
type Edu = Schemas["Education"];
type Exp = Schemas["Experience"];

function ProfilePage() {
  const q = useQuery<Schemas["Profile"]>("/me/profile");
  const toast = useToast();
  const [form, setForm] = useState<ProfileInput | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<ApiError | null>(null);
  const [saved, setSaved] = useState<Schemas["Profile"] | null>(null);
  const profile = saved ?? q.data;

  useEffect(() => {
    if (q.data && !form) {
      const { fullName, phone, birthDate, city, address, summary, education, experiences, certifications, contactEmail, skills, languages, links, projects, version } = q.data;
      setForm({
        fullName,
        phone,
        birthDate,
        city,
        address,
        summary,
        education,
        experiences,
        certifications,
        contactEmail: contactEmail ?? "",
        skills: skills ?? [],
        languages: languages ?? [],
        links: links ?? [],
        projects: projects ?? [],
        version,
      });
    }
  }, [q.data, form]);

  const fe = err?.fieldErrors ?? {};
  const set = <K extends keyof ProfileInput>(k: K, v: ProfileInput[K]) => setForm((f) => (f ? { ...f, [k]: v } : f));
  const setEdu = (i: number, patch: Partial<Edu>) => set("education", form!.education.map((e, j) => (j === i ? { ...e, ...patch } : e)));
  const setExp = (i: number, patch: Partial<Exp>) => set("experiences", form!.experiences.map((e, j) => (j === i ? { ...e, ...patch } : e)));
  const num = (s: string) => (s === "" ? null : Number(s));

  const save = async () => {
    if (!form) return;
    setBusy(true);
    setErr(null);
    try {
      const p = await put<Schemas["Profile"]>("/me/profile", form);
      setSaved(p);
      setForm((f) => (f ? { ...f, version: p.version } : f));
      toast("success", "Profil tersimpan.");
    } catch (e) {
      setErr(e as ApiError);
      window.scrollTo(0, 0);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="max-w-3xl">
      <PageHeader title="Profil" subtitle="Profil diisi sekali dan disalin ke setiap lamaran saat Anda melamar. Lamaran yang sudah terkirim tidak berubah." />
      <VerifyBanner />
      <PageState loading={q.loading} error={q.error} onRetry={q.reload}>
        {form && profile ? (
          <form
            noValidate
            className="flex flex-col gap-6"
            onSubmit={(e) => {
              e.preventDefault();
              save();
            }}
          >
            {err ? (
              <Alert
                tone="danger"
                title={err.code === "stale_version" ? "Profil berubah di tempat lain" : err.message}
                action={
                  err.code === "stale_version" ? (
                    <Button
                      onClick={() => {
                        setForm(null);
                        setSaved(null);
                        setErr(null);
                        q.reload();
                      }}
                    >
                      Muat versi terbaru
                    </Button>
                  ) : null
                }
              >
                {err.code === "stale_version" ? err.message : null}
              </Alert>
            ) : null}
            {!profile.complete ? (
              <Alert tone="warning" title="Profil belum lengkap">
                Lengkapi: {profile.missing.map((m) => fieldLabel[m] ?? m).join(", ")}.
              </Alert>
            ) : (
              <Alert tone="success" title="Profil lengkap">Anda dapat melamar lowongan.</Alert>
            )}

            <CvImport />

            <section className="flex flex-col gap-4">
              <h2 className="text-[20px]">Identitas</h2>
              <p className="text-sm text-ink-2">
                Email: {profile.email} · NIK: {profile.identity.status === "linked" ? profile.identity.nikMasked : "sedang diverifikasi manual oleh tim rekrutmen"}
              </p>
              <Field label="Nama lengkap" required error={fe.fullName}>
                {({ id, describedBy, invalid }) => <Input id={id} aria-describedby={describedBy} invalid={invalid} value={form.fullName} onChange={(e) => set("fullName", e.target.value)} />}
              </Field>
              <div className="grid gap-4 sm:grid-cols-2">
                <Field label="Nomor telepon" required error={fe.phone}>
                  {({ id, describedBy, invalid }) => (
                    <Input id={id} aria-describedby={describedBy} invalid={invalid} inputMode="tel" autoComplete="tel" value={form.phone} onChange={(e) => set("phone", e.target.value)} />
                  )}
                </Field>
                <Field label="Tanggal lahir" required error={fe.birthDate}>
                  {({ id, describedBy, invalid }) => (
                    <Input id={id} aria-describedby={describedBy} invalid={invalid} type="date" value={form.birthDate ?? ""} onChange={(e) => set("birthDate", e.target.value || null)} />
                  )}
                </Field>
                <Field label="Kota domisili" required error={fe.city}>
                  {({ id, describedBy, invalid }) => <Input id={id} aria-describedby={describedBy} invalid={invalid} value={form.city} onChange={(e) => set("city", e.target.value)} />}
                </Field>
              </div>
              <Field label="Email kontak" hint="Opsional. Email akun untuk masuk tidak berubah." error={fe.contactEmail}>
                {({ id, describedBy, invalid }) => (
                  <Input id={id} aria-describedby={describedBy} invalid={invalid} type="email" autoComplete="email" value={form.contactEmail ?? ""} onChange={(e) => set("contactEmail", e.target.value)} />
                )}
              </Field>
              <Field label="Alamat" error={fe.address}>
                {({ id, invalid }) => <Textarea id={id} invalid={invalid} value={form.address} onChange={(e) => set("address", e.target.value)} />}
              </Field>
              <Field label="Ringkasan diri" error={fe.summary}>
                {({ id, invalid }) => <Textarea id={id} invalid={invalid} value={form.summary} onChange={(e) => set("summary", e.target.value)} />}
              </Field>
            </section>

            <section className="flex flex-col gap-4">
              <h2 className="text-[20px]">Pendidikan</h2>
              {form.education.map((ed, i) => (
                <fieldset key={i} className="grid gap-3 rounded-md border border-line p-4 sm:grid-cols-2">
                  <legend className="px-1 text-sm font-semibold">Pendidikan {i + 1}</legend>
                  <Field label="Jenjang" required error={fe[`education.${i}.level`]}>
                    {({ id }) => (
                      <Select id={id} value={ed.level} onChange={(e) => setEdu(i, { level: e.target.value as Edu["level"] })}>
                        {["SMA", "SMK", "D3", "D4", "S1", "S2", "S3", "Lainnya"].map((l) => (
                          <option key={l}>{l}</option>
                        ))}
                      </Select>
                    )}
                  </Field>
                  <Field label="Institusi" required error={fe[`education.${i}.institution`]}>
                    {({ id, invalid }) => <Input id={id} invalid={invalid} value={ed.institution} onChange={(e) => setEdu(i, { institution: e.target.value })} />}
                  </Field>
                  <Field label="Jurusan">{({ id }) => <Input id={id} value={ed.major} onChange={(e) => setEdu(i, { major: e.target.value })} />}</Field>
                  <Field label="IPK / nilai rata-rata" error={fe[`education.${i}.gpa`]}>
                    {({ id, invalid }) => <Input id={id} invalid={invalid} inputMode="decimal" value={ed.gpa ?? ""} onChange={(e) => setEdu(i, { gpa: num(e.target.value) })} />}
                  </Field>
                  <Field label="Tahun masuk">{({ id }) => <Input id={id} inputMode="numeric" value={ed.startYear ?? ""} onChange={(e) => setEdu(i, { startYear: num(e.target.value) })} />}</Field>
                  <Field label="Tahun lulus" error={fe[`education.${i}.endYear`]}>
                    {({ id, invalid }) => <Input id={id} invalid={invalid} inputMode="numeric" value={ed.endYear ?? ""} onChange={(e) => setEdu(i, { endYear: num(e.target.value) })} />}
                  </Field>
                  <div className="sm:col-span-2">
                    <Button type="button" variant="ghost" onClick={() => set("education", form.education.filter((_, j) => j !== i))}>
                      Hapus pendidikan {i + 1}
                    </Button>
                  </div>
                </fieldset>
              ))}
              <div>
                <Button type="button" onClick={() => set("education", [...form.education, { level: "SMA", institution: "", major: "", startYear: null, endYear: null, gpa: null }])}>
                  Tambah pendidikan
                </Button>
              </div>
            </section>

            <section className="flex flex-col gap-4">
              <h2 className="text-[20px]">Pengalaman kerja</h2>
              {form.experiences.length === 0 ? <p className="text-ink-2">Belum ada pengalaman. Boleh dikosongkan untuk lulusan baru.</p> : null}
              {form.experiences.map((ex, i) => (
                <fieldset key={i} className="grid gap-3 rounded-md border border-line p-4 sm:grid-cols-2">
                  <legend className="px-1 text-sm font-semibold">Pengalaman {i + 1}</legend>
                  <Field label="Perusahaan" required error={fe[`experiences.${i}.company`]}>
                    {({ id, invalid }) => <Input id={id} invalid={invalid} value={ex.company} onChange={(e) => setExp(i, { company: e.target.value })} />}
                  </Field>
                  <Field label="Jabatan" required>{({ id }) => <Input id={id} value={ex.title} onChange={(e) => setExp(i, { title: e.target.value })} />}</Field>
                  <Field label="Mulai" required hint={precisionHint(ex.startDate, ex.startPrecision)} error={fe[`experiences.${i}.startDate`]}>
                    {({ id, invalid, describedBy }) => (
                      <Input id={id} aria-describedby={describedBy} invalid={invalid} type="date" value={ex.startDate} onChange={(e) => setExp(i, { startDate: e.target.value, startPrecision: "day" })} />
                    )}
                  </Field>
                  <Field label="Selesai" hint={ex.isCurrent ? "Masih bekerja di sini." : precisionHint(ex.endDate, ex.endPrecision)} error={fe[`experiences.${i}.endDate`]}>
                    {({ id, invalid, describedBy }) => (
                      <Input
                        id={id}
                        aria-describedby={describedBy}
                        invalid={invalid}
                        type="date"
                        disabled={ex.isCurrent}
                        value={ex.isCurrent ? "" : (ex.endDate ?? "")}
                        onChange={(e) => setExp(i, { endDate: e.target.value || null, endPrecision: "day" })}
                      />
                    )}
                  </Field>
                  <label className="flex items-center gap-2 text-sm">
                    <input type="checkbox" className="h-5 w-5" checked={!!ex.isCurrent} onChange={(e) => setExp(i, { isCurrent: e.target.checked, endDate: e.target.checked ? null : ex.endDate })} />
                    Masih bekerja di sini
                  </label>
                  <Field label="Lokasi">{({ id }) => <Input id={id} value={ex.location ?? ""} onChange={(e) => setExp(i, { location: e.target.value })} />}</Field>
                  <div className="sm:col-span-2">
                    <Field label="Uraian tugas">{({ id }) => <Textarea id={id} value={ex.description} onChange={(e) => setExp(i, { description: e.target.value })} />}</Field>
                  </div>
                  <div className="sm:col-span-2">
                    <Button type="button" variant="ghost" onClick={() => set("experiences", form.experiences.filter((_, j) => j !== i))}>
                      Hapus pengalaman {i + 1}
                    </Button>
                  </div>
                </fieldset>
              ))}
              <div>
                <Button
                  type="button"
                  onClick={() =>
                    set("experiences", [...form.experiences, { company: "", title: "", location: "", startDate: "", startPrecision: "day", endDate: null, endPrecision: "day", isCurrent: false, description: "" }])
                  }
                >
                  Tambah pengalaman
                </Button>
              </div>
            </section>

            <ExtrasSection form={form} set={set} fe={fe} />

            <DocumentsSection />

            <div className="sticky bottom-0 border-t border-line bg-white py-3">
              <Button type="submit" variant="primary" busy={busy}>
                Simpan profil
              </Button>
            </div>
          </form>
        ) : null}
      </PageState>
    </div>
  );
}

/** Keterangan presisi tanggal dari CV: kolom tanggal menampilkan hari pertama, CV hanya menyebut bulan/tahun. */
function precisionHint(date: string | null | undefined, precision?: string): string | undefined {
  if (!date || !precision || precision === "day") return undefined;
  const [y = 0, m] = date.split("-").map(Number);
  return `Dari CV: ${fmtPartial({ year: y, month: m, precision: precision as "month" | "year" })} (tanggal tidak tertulis).`;
}

function ExtrasSection({ form, set, fe }: { form: ProfileInput; set: <K extends keyof ProfileInput>(k: K, v: ProfileInput[K]) => void; fe: Record<string, string> }) {
  const langs = form.languages ?? [];
  const links = form.links ?? [];
  const projects = form.projects ?? [];
  const [skillText, setSkillText] = useState((form.skills ?? []).join(", "));
  return (
    <section className="flex flex-col gap-4">
      <h2 className="text-[20px]">Keahlian, bahasa, dan tautan</h2>
      <Field label="Keahlian" hint="Pisahkan dengan koma, misalnya: Microsoft Excel, Forklift, K3." error={fe.skills}>
        {({ id, describedBy }) => (
          <Input
            id={id}
            aria-describedby={describedBy}
            value={skillText}
            onChange={(e) => {
              setSkillText(e.target.value);
              set("skills", e.target.value.split(",").map((x) => x.trim()).filter(Boolean));
            }}
          />
        )}
      </Field>
      <fieldset className="flex flex-col gap-3">
        <legend className="text-sm font-semibold">Bahasa</legend>
        {langs.map((l, i) => (
          <div key={i} className="grid gap-2 sm:grid-cols-[1fr_1fr_auto] sm:items-end">
            <Field label={`Bahasa ${i + 1}`}>{({ id }) => <Input id={id} value={l.name} onChange={(e) => set("languages", langs.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))} />}</Field>
            <Field label="Tingkat">{({ id }) => <Input id={id} value={l.level} onChange={(e) => set("languages", langs.map((x, j) => (j === i ? { ...x, level: e.target.value } : x)))} />}</Field>
            <Button type="button" variant="ghost" onClick={() => set("languages", langs.filter((_, j) => j !== i))}>
              Hapus bahasa {i + 1}
            </Button>
          </div>
        ))}
        <div>
          <Button type="button" onClick={() => set("languages", [...langs, { name: "", level: "" }])}>
            Tambah bahasa
          </Button>
        </div>
      </fieldset>
      <fieldset className="flex flex-col gap-3">
        <legend className="text-sm font-semibold">Tautan profesional</legend>
        {links.map((l, i) => (
          <div key={i} className="grid gap-2 sm:grid-cols-[1fr_2fr_auto] sm:items-end">
            <Field label={`Label ${i + 1}`}>{({ id }) => <Input id={id} value={l.label} onChange={(e) => set("links", links.map((x, j) => (j === i ? { ...x, label: e.target.value } : x)))} />}</Field>
            <Field label="Alamat" error={fe[`links.${i}.url`]}>
              {({ id, invalid }) => (
                <Input id={id} invalid={invalid} type="url" placeholder="https://" value={l.url} onChange={(e) => set("links", links.map((x, j) => (j === i ? { ...x, url: e.target.value } : x)))} />
              )}
            </Field>
            <Button type="button" variant="ghost" onClick={() => set("links", links.filter((_, j) => j !== i))}>
              Hapus tautan {i + 1}
            </Button>
          </div>
        ))}
        <div>
          <Button type="button" onClick={() => set("links", [...links, { label: "", url: "" }])}>
            Tambah tautan
          </Button>
        </div>
      </fieldset>
      <fieldset className="flex flex-col gap-3">
        <legend className="text-sm font-semibold">Proyek</legend>
        {projects.map((p, i) => (
          <div key={i} className="grid gap-2">
            <Field label={`Nama proyek ${i + 1}`}>{({ id }) => <Input id={id} value={p.name} onChange={(e) => set("projects", projects.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))} />}</Field>
            <Field label="Uraian">{({ id }) => <Textarea id={id} value={p.description} onChange={(e) => set("projects", projects.map((x, j) => (j === i ? { ...x, description: e.target.value } : x)))} />}</Field>
            <div>
              <Button type="button" variant="ghost" onClick={() => set("projects", projects.filter((_, j) => j !== i))}>
                Hapus proyek {i + 1}
              </Button>
            </div>
          </div>
        ))}
        <div>
          <Button type="button" onClick={() => set("projects", [...projects, { name: "", description: "" }])}>
            Tambah proyek
          </Button>
        </div>
      </fieldset>
    </section>
  );
}

function ApplicationPage() {
  const { id = "" } = useParams();
  const [params] = useSearchParams();
  const q = useQuery<Schemas["Timeline"]>(`/me/applications/${id}/timeline`);
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<ApiError | null>(null);

  const withdraw = async () => {
    if (!q.data) return;
    setBusy(true);
    setErr(null);
    try {
      await post(`/me/applications/${id}/withdraw`, { reason, version: q.data.version });
      setOpen(false);
      toast("success", "Lamaran ditarik.");
      q.reload();
    } catch (e) {
      setErr(e as ApiError);
    } finally {
      setBusy(false);
    }
  };

  return (
    <PageState loading={q.loading} error={q.error} onRetry={q.reload}>
      {q.data ? (
        <div className="max-w-3xl">
          <Link to="/akun" className="text-sm text-primary hover:underline">
            Kembali ke lamaran saya
          </Link>
          <PageHeader
            title={q.data.jobTitle}
            subtitle={
              <span className="inline-flex flex-wrap items-center gap-2">
                <Badge status={q.data.status}>{statusLabel[q.data.status]}</Badge> Tahap saat ini: {q.data.stageName}
              </span>
            }
          />
          {params.get("baru") === "1" ? (
            <div className="mb-6">
              <Alert tone="success" title="Lamaran terkirim">
                Konfirmasi juga dikirim ke email dan kotak masuk Anda.
              </Alert>
            </div>
          ) : null}
          <h2 className="mb-3 text-[20px]">Riwayat</h2>
          <ol className="border-l-2 border-line pl-5">
            {q.data.items.map((it, i) => (
              <li key={i} className="relative pb-5">
                <span className="absolute -left-[27px] top-1.5 h-3 w-3 rounded-full border-2 border-primary bg-white" aria-hidden />
                <p className="font-semibold">{it.status === "withdrawn" ? "Lamaran ditarik" : it.stageName}</p>
                <p className="text-sm text-ink-2">{fmtDateTime(it.at)}</p>
              </li>
            ))}
          </ol>
          {q.data.status === "active" ? (
            <div className="mt-6 border-t border-line pt-6">
              <Button variant="secondary" onClick={() => setOpen(true)}>
                Tarik lamaran
              </Button>
            </div>
          ) : null}
          <Dialog open={open} onOpenChange={setOpen} title="Tarik lamaran?" description="Lamaran yang ditarik tidak dapat diaktifkan kembali untuk lowongan ini.">
            {err ? <Alert tone="danger">{err.message}</Alert> : null}
            <Field label="Alasan">{({ id }) => <Textarea id={id} value={reason} maxLength={500} onChange={(e) => setReason(e.target.value)} />}</Field>
            <div className="mt-4 flex justify-end gap-2">
              <Button onClick={() => setOpen(false)}>Batal</Button>
              <Button variant="danger" busy={busy} onClick={withdraw}>
                Tarik lamaran
              </Button>
            </div>
          </Dialog>
        </div>
      ) : null}
    </PageState>
  );
}

function Inbox() {
  const q = useQuery<{ items: Schemas["Notification"][] }>("/me/notifications");
  return (
    <div className="max-w-3xl">
      <PageHeader title="Kotak masuk" />
      <PageState loading={q.loading} error={q.error} onRetry={q.reload} empty={q.data?.items.length === 0} emptyText="Belum ada pemberitahuan.">
        <ul className="divide-y divide-line border-y border-line">
          {q.data?.items.map((n) => (
            <li key={n.id} className="py-4">
              <p className="font-semibold">{n.subject}</p>
              <p className="text-sm text-ink-2">{fmtDateTime(n.createdAt)}</p>
              <p className="mt-2 whitespace-pre-line text-[15px]">{n.body}</p>
            </li>
          ))}
        </ul>
      </PageState>
    </div>
  );
}

type Invitation = {
  id: string;
  status: string;
  extraMinutes: number;
  scheduleName: string;
  opensAt: string;
  closesAt: string;
  durationMinutes: number;
  packageName: string;
  jobTitle: string;
  attemptId: string | null;
  attemptStatus: string | null;
};

const invStatus: Record<string, string> = { invited: "Belum dimulai", started: "Sedang berjalan", submitted: "Sudah dikirim", expired: "Waktu habis", cancelled: "Dibatalkan" };

function ExamInvitations() {
  const q = useQuery<{ items: Invitation[]; serverTime: string }>("/me/exam-invitations");
  const nav = useNavigate();
  const [busy, setBusy] = useState<string | null>(null);
  const [err, setErr] = useState<ApiError | null>(null);
  const start = async (inv: Invitation) => {
    setBusy(inv.id);
    setErr(null);
    try {
      const a = await post<{ id: string }>(`/exam-invitations/${inv.id}/start`);
      nav(`/ujian/${a.id}`);
    } catch (e) {
      setErr(e as ApiError);
    } finally {
      setBusy(null);
    }
  };
  const serverNow = q.data ? Date.parse(q.data.serverTime) : 0;
  return (
    <div className="max-w-3xl">
      <PageHeader title="Ujian" subtitle="Kerjakan dengan koneksi stabil. Jawaban disimpan otomatis dan waktu dihitung oleh server." />
      {err ? (
        <div className="mb-4">
          <Alert tone="danger">{err.message}</Alert>
        </div>
      ) : null}
      <PageState loading={q.loading} error={q.error} onRetry={q.reload} empty={q.data?.items.length === 0} emptyText="Belum ada undangan ujian.">
        <ul className="divide-y divide-line border-y border-line">
          {q.data?.items.map((inv) => {
            const open = serverNow >= Date.parse(inv.opensAt) && serverNow < Date.parse(inv.closesAt);
            return (
              <li key={inv.id} className="flex flex-wrap items-center justify-between gap-3 py-4">
                <div>
                  <p className="text-[17px] font-semibold">{inv.packageName}</p>
                  <p className="text-sm text-ink-2">
                    {inv.jobTitle} · {inv.scheduleName} · {inv.durationMinutes + inv.extraMinutes} menit
                  </p>
                  <p className="text-sm text-ink-2">
                    Dibuka {fmtDateTime(inv.opensAt)} sampai {fmtDateTime(inv.closesAt)} · {invStatus[inv.status] ?? inv.status}
                  </p>
                </div>
                {inv.status === "started" && inv.attemptId ? (
                  <Button variant="primary" onClick={() => nav(`/ujian/${inv.attemptId}`)}>
                    Lanjutkan ujian
                  </Button>
                ) : inv.status === "invited" ? (
                  <Button variant="primary" disabled={!open} busy={busy === inv.id} onClick={() => start(inv)}>
                    {open ? "Mulai ujian" : "Belum dibuka"}
                  </Button>
                ) : inv.attemptId ? (
                  <Link className="font-semibold text-primary underline" to={`/ujian/${inv.attemptId}`}>
                    Lihat bukti penerimaan
                  </Link>
                ) : null}
              </li>
            );
          })}
        </ul>
      </PageState>
    </div>
  );
}
