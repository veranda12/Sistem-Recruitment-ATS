import { useState } from "react";
import { ApiError, post, type Schemas } from "../../lib/api";
import { fmtDateTime, getOrgTimeZone } from "../../lib/format";
import { useSession } from "../../lib/session";
import { useQuery } from "../../lib/useQuery";
import { Alert, Button, Dialog, Field, Input, PageHeader, PageState, Select, Textarea, useToast } from "../../ui";

export type Interview = Schemas["Interview"];
export type Evaluation = Schemas["Evaluation"];

const modeLabel: Record<string, string> = { onsite: "Tatap muka", video: "Video", phone: "Telepon" };
const recLabel: Record<string, string> = { lanjut: "Lanjut", pertimbangkan: "Pertimbangkan", tidak_lanjut: "Tidak lanjut" };
const zones = ["Asia/Jakarta", "Asia/Makassar", "Asia/Jayapura"];

/** Waktu interview ditampilkan di zona interview itu sendiri, bukan zona perangkat. */
function inZone(iso: string, tz: string) {
  const z: Record<string, string> = { "Asia/Jakarta": "WIB", "Asia/Makassar": "WITA", "Asia/Jayapura": "WIT" };
  return (
    new Intl.DateTimeFormat("id-ID", { dateStyle: "medium", timeStyle: "short", timeZone: tz }).format(new Date(iso)) + " " + (z[tz] ?? tz)
  );
}

/** Mengubah input datetime-local (tanpa zona) menjadi waktu absolut di zona IANA yang dipilih. */
export function zonedToISO(local: string, tz: string): string {
  const [d, t] = local.split("T");
  const [y, mo, da] = d!.split("-").map(Number) as [number, number, number];
  const [h, mi] = t!.split(":").map(Number) as [number, number];
  const guess = Date.UTC(y, mo - 1, da, h, mi);
  const parts = new Intl.DateTimeFormat("en-US", { timeZone: tz, hourCycle: "h23", year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" })
    .formatToParts(new Date(guess))
    .reduce<Record<string, string>>((a, p) => ((a[p.type] = p.value), a), {});
  const asZone = Date.UTC(+parts.year!, +parts.month! - 1, +parts.day!, +parts.hour!, +parts.minute!);
  return new Date(guess - (asZone - guess)).toISOString();
}

type Staff = { id: string; displayName: string };

export function InterviewSection({ applicationId, interviews, evaluations, canSchedule, onChange }: {
  applicationId: string;
  interviews: Interview[];
  evaluations: Evaluation[];
  canSchedule: boolean;
  onChange: () => void;
}) {
  const { session } = useSession();
  const toast = useToast();
  const staff = useQuery<{ items: Staff[] }>(canSchedule ? "/staff/interviewers" : null);
  const [open, setOpen] = useState<null | { mode: "new" } | { mode: "reschedule" | "cancel" | "evaluate"; iv: Interview }>(null);
  const [form, setForm] = useState({ local: "", tz: getOrgTimeZone(), duration: "60", mode: "video", location: "", ivs: [] as string[], reason: "", rating: "4", rec: "lanjut", notes: "" });
  const [err, setErr] = useState<ApiError | null>(null);
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    if (!open) return;
    setBusy(true);
    setErr(null);
    try {
      if (open.mode === "new") {
        await post(`/applications/${applicationId}/interviews`, {
          startsAt: zonedToISO(form.local, form.tz), durationMinutes: Number(form.duration), timezone: form.tz, mode: form.mode, location: form.location, interviewerIds: form.ivs,
        });
      } else if (open.mode === "reschedule") {
        await post(`/interviews/${open.iv.id}/reschedule`, { startsAt: zonedToISO(form.local, form.tz), durationMinutes: Number(form.duration), timezone: form.tz, version: open.iv.version, reason: form.reason });
      } else if (open.mode === "cancel") {
        await post(`/interviews/${open.iv.id}/cancel`, { version: open.iv.version, reason: form.reason });
      } else {
        await post(`/interviews/${open.iv.id}/evaluations`, { rating: Number(form.rating), recommendation: form.rec, notes: form.notes });
      }
      toast("success", "Tersimpan. Kandidat menerima pemberitahuan bila jadwal berubah.");
      setOpen(null);
      onChange();
    } catch (e) {
      setErr(e as ApiError);
    } finally {
      setBusy(false);
    }
  };
  const fe = err?.fieldErrors ?? {};
  const needTime = open && (open.mode === "new" || open.mode === "reschedule");

  return (
    <section className="mt-6">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <h3 className="font-semibold">Interview</h3>
        {canSchedule ? (
          <Button size="sm" onClick={() => { setErr(null); setForm({ ...form, local: "", ivs: [] }); setOpen({ mode: "new" }); }}>
            Jadwalkan interview
          </Button>
        ) : null}
      </div>
      {interviews.length === 0 ? <p className="text-ink-2">Belum ada interview.</p> : null}
      <ul className="flex flex-col gap-3">
        {interviews.map((iv) => {
          const mine = session && iv.interviewerIds.includes(session.user.id);
          return (
            <li key={iv.id} className="rounded-md border border-line p-3">
              <p className="font-semibold">
                {inZone(iv.startsAt, iv.timezone)} · {modeLabel[iv.mode]} {iv.status !== "scheduled" ? `· ${iv.status === "cancelled" ? "Dibatalkan" : "Selesai"}` : ""}
              </p>
              <p className="text-sm text-ink-2">
                {iv.location || "-"} · Interviewer: {iv.interviewerNames.join(", ")}
              </p>
              <div className="mt-2 flex flex-wrap gap-2">
                {iv.status === "scheduled" && canSchedule ? (
                  <>
                    <Button size="sm" onClick={() => { setErr(null); setForm({ ...form, tz: iv.timezone, reason: "" }); setOpen({ mode: "reschedule", iv }); }}>
                      Jadwalkan ulang
                    </Button>
                    <Button size="sm" variant="ghost" onClick={() => { setErr(null); setForm({ ...form, reason: "" }); setOpen({ mode: "cancel", iv }); }}>
                      Batalkan
                    </Button>
                  </>
                ) : null}
                {mine && iv.status !== "cancelled" ? (
                  <Button size="sm" onClick={() => { setErr(null); setOpen({ mode: "evaluate", iv }); }}>
                    Isi penilaian
                  </Button>
                ) : null}
              </div>
            </li>
          );
        })}
      </ul>
      {evaluations.length ? (
        <div className="mt-3">
          <h4 className="text-sm font-semibold">Penilaian interviewer</h4>
          <ul className="text-[15px]">
            {evaluations.map((e) => (
              <li key={e.id}>
                {e.evaluatorName}: {e.rating}/5, {recLabel[e.recommendation]}
                {e.notes ? ` · ${e.notes}` : ""} <span className="text-[13px] text-ink-2">({fmtDateTime(e.createdAt)})</span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      <Dialog
        open={!!open}
        onOpenChange={(o) => !o && setOpen(null)}
        title={open?.mode === "new" ? "Jadwalkan interview" : open?.mode === "reschedule" ? "Jadwalkan ulang interview" : open?.mode === "cancel" ? "Batalkan interview?" : "Penilaian interview"}
      >
        {err ? <Alert tone="danger">{Object.values(fe)[0] ?? err.message}</Alert> : null}
        <div className="flex flex-col gap-3">
          {needTime ? (
            <>
              <div className="grid gap-3 sm:grid-cols-2">
                <Field label="Tanggal dan jam" required>{({ id }) => <Input id={id} type="datetime-local" value={form.local} onChange={(e) => setForm({ ...form, local: e.target.value })} />}</Field>
                <Field label="Zona waktu" required>
                  {({ id }) => (
                    <Select id={id} value={form.tz} onChange={(e) => setForm({ ...form, tz: e.target.value })}>
                      {zones.map((z) => (
                        <option key={z} value={z}>
                          {z}
                        </option>
                      ))}
                    </Select>
                  )}
                </Field>
              </div>
              <Field label="Durasi (menit)" required>{({ id }) => <Input id={id} inputMode="numeric" value={form.duration} onChange={(e) => setForm({ ...form, duration: e.target.value })} />}</Field>
            </>
          ) : null}
          {open?.mode === "new" ? (
            <>
              <Field label="Jenis" required>
                {({ id }) => (
                  <Select id={id} value={form.mode} onChange={(e) => setForm({ ...form, mode: e.target.value })}>
                    {Object.entries(modeLabel).map(([k, v]) => (
                      <option key={k} value={k}>
                        {v}
                      </option>
                    ))}
                  </Select>
                )}
              </Field>
              <Field label="Lokasi atau tautan">{({ id }) => <Input id={id} value={form.location} onChange={(e) => setForm({ ...form, location: e.target.value })} />}</Field>
              <fieldset>
                <legend className="text-sm font-semibold">Interviewer (wajib)</legend>
                <div className="mt-1 flex max-h-40 flex-col overflow-y-auto">
                  {staff.data?.items.map((s) => (
                    <label key={s.id} className="flex min-h-11 items-center gap-3">
                      <input
                        type="checkbox"
                        className="h-5 w-5"
                        checked={form.ivs.includes(s.id)}
                        onChange={(e) => setForm({ ...form, ivs: e.target.checked ? [...form.ivs, s.id] : form.ivs.filter((x) => x !== s.id) })}
                      />
                      {s.displayName}
                    </label>
                  ))}
                </div>
              </fieldset>
            </>
          ) : null}
          {open?.mode === "reschedule" || open?.mode === "cancel" ? (
            <Field label="Alasan" required>{({ id }) => <Textarea id={id} className="min-h-11" value={form.reason} onChange={(e) => setForm({ ...form, reason: e.target.value })} />}</Field>
          ) : null}
          {open?.mode === "evaluate" ? (
            <>
              <Field label="Nilai (1-5)" required>
                {({ id }) => (
                  <Select id={id} value={form.rating} onChange={(e) => setForm({ ...form, rating: e.target.value })}>
                    {[1, 2, 3, 4, 5].map((n) => (
                      <option key={n}>{n}</option>
                    ))}
                  </Select>
                )}
              </Field>
              <Field label="Rekomendasi" required>
                {({ id }) => (
                  <Select id={id} value={form.rec} onChange={(e) => setForm({ ...form, rec: e.target.value })}>
                    {Object.entries(recLabel).map(([k, v]) => (
                      <option key={k} value={k}>
                        {v}
                      </option>
                    ))}
                  </Select>
                )}
              </Field>
              <Field label="Catatan">{({ id }) => <Textarea id={id} value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} />}</Field>
            </>
          ) : null}
        </div>
        <div className="mt-4 flex justify-end gap-2">
          <Button onClick={() => setOpen(null)}>Kembali</Button>
          <Button variant={open?.mode === "cancel" ? "danger" : "primary"} busy={busy} onClick={submit}>
            {open?.mode === "cancel" ? "Batalkan interview" : "Simpan"}
          </Button>
        </div>
      </Dialog>
    </section>
  );
}

type Req = {
  id: string;
  title: string;
  headcount: number;
  employmentType: string;
  status: string;
  unitName: string;
  locationName: string;
  requestedBy: string;
  requestedByName: string;
  hiringManagerName: string | null;
  version: number;
  decisionNote: string;
  createdAt: string;
};
const reqStatus: Record<string, string> = { draft: "Draft", submitted: "Menunggu persetujuan", approved: "Disetujui", rejected: "Ditolak", cancelled: "Dibatalkan" };

export function RequisitionsPage() {
  const q = useQuery<{ items: Req[] }>("/requisitions");
  const org = useQuery<{ units: { code: string; name: string }[]; locations: { code: string; name: string; city: string }[] }>("/org");
  const { session, can } = useSession();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ unitCode: "", locationCode: "", title: "", headcount: "1", employmentType: "full_time", justification: "" });
  const [note, setNote] = useState("");
  const [err, setErr] = useState<ApiError | null>(null);

  const act = async (r: Req, action: string) => {
    setErr(null);
    try {
      await post(`/requisitions/${r.id}/${action}`, { version: r.version, note });
      toast("success", "Status permintaan diperbarui.");
      setNote("");
      q.reload();
    } catch (e) {
      setErr(e as ApiError);
    }
  };
  const create = async () => {
    setErr(null);
    try {
      await post("/requisitions", { ...form, headcount: Number(form.headcount) });
      toast("success", "Permintaan tenaga kerja dibuat sebagai draft.");
      setOpen(false);
      q.reload();
    } catch (e) {
      setErr(e as ApiError);
    }
  };
  return (
    <div className="max-w-5xl">
      <PageHeader
        title="Permintaan tenaga kerja"
        subtitle="Requisition internal, terpisah dari lowongan yang dipublikasikan."
        actions={can("requisitions.manage") ? <Button variant="primary" onClick={() => setOpen(true)}>Ajukan permintaan</Button> : null}
      />
      {err && !open ? (
        <div className="mb-4">
          <Alert tone="danger">{Object.values(err.fieldErrors)[0] ?? err.message}</Alert>
        </div>
      ) : null}
      <PageState loading={q.loading} error={q.error} onRetry={q.reload} empty={q.data?.items.length === 0} emptyText="Belum ada permintaan tenaga kerja.">
        <ul className="divide-y divide-line border-y border-line">
          {q.data?.items.map((r) => (
            <li key={r.id} className="py-4">
              <p className="font-semibold">
                {r.title} · {r.headcount} orang
              </p>
              <p className="text-sm text-ink-2">
                {r.unitName} · {r.locationName} · diajukan {r.requestedByName} · {reqStatus[r.status]}
                {r.decisionNote ? ` · catatan: ${r.decisionNote}` : ""}
              </p>
              <div className="mt-2 flex flex-wrap items-center gap-2">
                {r.status === "draft" && r.requestedBy === session?.user.id ? (
                  <Button size="sm" onClick={() => act(r, "submit")}>
                    Ajukan untuk persetujuan
                  </Button>
                ) : null}
                {r.status === "submitted" && can("requisitions.approve") && r.requestedBy !== session?.user.id ? (
                  <>
                    <Input aria-label={`Catatan keputusan untuk ${r.title}`} className="max-w-xs" value={note} onChange={(e) => setNote(e.target.value)} placeholder="Catatan keputusan" />
                    <Button size="sm" variant="primary" onClick={() => act(r, "approve")}>
                      Setujui
                    </Button>
                    <Button size="sm" onClick={() => act(r, "reject")}>
                      Tolak
                    </Button>
                  </>
                ) : null}
              </div>
            </li>
          ))}
        </ul>
      </PageState>
      <Dialog open={open} onOpenChange={setOpen} title="Ajukan permintaan tenaga kerja">
        {err && open ? <Alert tone="danger">{Object.values(err.fieldErrors)[0] ?? err.message}</Alert> : null}
        <div className="flex flex-col gap-3">
          <Field label="Posisi" required>{({ id }) => <Input id={id} value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} />}</Field>
          <Field label="Unit" required>
            {({ id }) => (
              <Select id={id} value={form.unitCode} onChange={(e) => setForm({ ...form, unitCode: e.target.value })}>
                <option value="">Pilih unit</option>
                {org.data?.units.map((u) => (
                  <option key={u.code} value={u.code}>
                    {u.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="Lokasi" required>
            {({ id }) => (
              <Select id={id} value={form.locationCode} onChange={(e) => setForm({ ...form, locationCode: e.target.value })}>
                <option value="">Pilih lokasi</option>
                {org.data?.locations.map((l) => (
                  <option key={l.code} value={l.code}>
                    {l.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="Jumlah kebutuhan" required>{({ id }) => <Input id={id} inputMode="numeric" value={form.headcount} onChange={(e) => setForm({ ...form, headcount: e.target.value })} />}</Field>
          <Field label="Alasan kebutuhan">{({ id }) => <Textarea id={id} value={form.justification} onChange={(e) => setForm({ ...form, justification: e.target.value })} />}</Field>
        </div>
        <div className="mt-4 flex justify-end gap-2">
          <Button onClick={() => setOpen(false)}>Batal</Button>
          <Button variant="primary" onClick={create}>
            Simpan draft
          </Button>
        </div>
      </Dialog>
    </div>
  );
}
