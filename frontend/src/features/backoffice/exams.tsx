import { useState } from "react";
import { Link, useParams } from "react-router";
import { ApiError, post, type Schemas } from "../../lib/api";
import { fmtDateTime } from "../../lib/format";
import { useSession } from "../../lib/session";
import { useQuery } from "../../lib/useQuery";
import { Alert, Button, Dialog, Field, Input, PageHeader, PageState, Textarea, useToast } from "../../ui";

type Schedule = { id: string; name: string; opensAt: string; closesAt: string; packageKey: string; packageName: string; participantCount: number };
type Participant = {
  participantId: string;
  participantStatus: string;
  extraMinutes: number;
  candidateName: string;
  attemptId: string | null;
  attemptStatus: string | null;
  startedAt: string | null;
  deadlineAt: string | null;
  lastSeenAt: string | null;
  revision: number | null;
  answered: number;
  totalItems: number;
  score: number | null;
  essayPending: number | null;
  resultStatus: string | null;
  passed: boolean | null;
};

const pStatus: Record<string, string> = { invited: "Belum mulai", started: "Mengerjakan", submitted: "Sudah kirim", expired: "Waktu habis", cancelled: "Dibatalkan" };
const rStatus: Record<string, string> = { pending: "Menunggu penilaian esai", graded: "Siap dirilis", released: "Dirilis" };

export function SchedulesPage() {
  const q = useQuery<{ items: Schedule[] }>("/exam-schedules");
  return (
    <div>
      <PageHeader title="Jadwal ujian" />
      <PageState loading={q.loading} error={q.error} onRetry={q.reload} empty={q.data?.items.length === 0} emptyText="Belum ada jadwal ujian.">
        <ul className="divide-y divide-line border-y border-line">
          {q.data?.items.map((s) => (
            <li key={s.id} className="flex flex-wrap items-center justify-between gap-2 py-4">
              <div>
                <Link to={`/staf/ujian/${s.id}`} className="text-[17px] font-semibold text-primary hover:underline">
                  {s.name}
                </Link>
                <p className="text-sm text-ink-2">
                  {s.packageName} · {fmtDateTime(s.opensAt)} sampai {fmtDateTime(s.closesAt)}
                </p>
              </div>
              <span className="text-sm text-ink-2">{s.participantCount} peserta</span>
            </li>
          ))}
        </ul>
      </PageState>
    </div>
  );
}

export function ScheduleMonitor() {
  const { id = "" } = useParams();
  const q = useQuery<{ schedule: Schedule & { durationMinutes: number; passScore: number }; participants: Participant[]; serverTime: string }>(`/exam-schedules/${id}/monitor`);
  const apps = useQuery<{ items: Schemas["StaffApplication"][] }>("/applications?stage=ujian&status=active&limit=100");
  const { can } = useSession();
  const toast = useToast();
  const [inviteOpen, setInviteOpen] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [extra, setExtra] = useState("0");
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<ApiError | null>(null);
  const [releaseOpen, setReleaseOpen] = useState(false);

  const invited = new Set(q.data?.participants.map((p) => p.candidateName) ?? []);
  const invite = async () => {
    setBusy(true);
    setErr(null);
    try {
      const r = await post<{ results: { applicationId: string; outcome: string }[] }>(`/exam-schedules/${id}/invitations`, {
        applicationIds: [...selected],
        extraMinutes: Number(extra) || 0,
        reason,
      });
      const ok = r.results.filter((x) => x.outcome === "invited").length;
      const skipped = r.results.length - ok;
      toast(ok ? "success" : "warning", `${ok} peserta diundang${skipped ? `, ${skipped} dilewati (sudah diundang, di luar akses, atau bukan tahap ujian)` : ""}.`);
      setInviteOpen(false);
      setSelected(new Set());
      q.reload();
    } catch (e) {
      setErr(e as ApiError);
    } finally {
      setBusy(false);
    }
  };
  const release = async () => {
    setBusy(true);
    setErr(null);
    try {
      const r = await post<{ released: number }>(`/exam-schedules/${id}/release`);
      toast("success", `${r.released} hasil dirilis. Tahap lamaran diperbarui otomatis.`);
      setReleaseOpen(false);
      q.reload();
    } catch (e) {
      setErr(e as ApiError);
    } finally {
      setBusy(false);
    }
  };
  const serverNow = q.data ? Date.parse(q.data.serverTime) : 0;
  const onlineRecently = (p: Participant) => p.lastSeenAt && serverNow - Date.parse(p.lastSeenAt) < 90_000;

  return (
    <PageState loading={q.loading} error={q.error} onRetry={q.reload}>
      {q.data ? (
        <div className="max-w-6xl">
          <Link to="/staf/ujian" className="text-sm text-primary hover:underline">
            Kembali ke jadwal
          </Link>
          <PageHeader
            title={q.data.schedule.name}
            subtitle={`${q.data.schedule.packageName} · ${q.data.schedule.durationMinutes} menit · ambang lulus ${q.data.schedule.passScore}`}
            actions={
              <>
                <Button onClick={q.reload}>Perbarui</Button>
                <Button onClick={() => { setErr(null); setInviteOpen(true); }}>Undang peserta</Button>
                {can("results.release") ? (
                  <Button variant="primary" onClick={() => { setErr(null); setReleaseOpen(true); }}>
                    Rilis nilai
                  </Button>
                ) : null}
              </>
            }
          />
          <p className="mb-3 text-sm text-ink-2">
            "Terakhir aktif" berasal dari sinyal perangkat (perkiraan). "Jawaban diterima" hanya menghitung jawaban yang sudah tersimpan di server.
          </p>
          {q.data.participants.length === 0 ? (
            <p className="rounded-md border border-dashed border-line px-4 py-8 text-center text-ink-2">Belum ada peserta pada jadwal ini.</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full border-collapse text-left text-[15px]">
                <caption className="sr-only">Monitoring peserta</caption>
                <thead>
                  <tr className="border-b border-line text-sm text-ink-2">
                    <th scope="col" className="py-2 pr-4 font-semibold">Peserta</th>
                    <th scope="col" className="py-2 pr-4 font-semibold">Status</th>
                    <th scope="col" className="py-2 pr-4 font-semibold">Jawaban diterima</th>
                    <th scope="col" className="py-2 pr-4 font-semibold">Terakhir aktif</th>
                    <th scope="col" className="py-2 pr-4 font-semibold">Nilai</th>
                    <th scope="col" className="py-2 font-semibold">Hasil</th>
                  </tr>
                </thead>
                <tbody>
                  {q.data.participants.map((p) => (
                    <tr key={p.participantId} className="border-b border-line">
                      <td className="py-3 pr-4">
                        {p.candidateName}
                        {p.extraMinutes ? <span className="block text-[13px] text-ink-2">+{p.extraMinutes} menit akomodasi</span> : null}
                      </td>
                      <td className="py-3 pr-4">{pStatus[p.participantStatus] ?? p.participantStatus}</td>
                      <td className="py-3 pr-4">{p.attemptId ? `${p.answered} / ${p.totalItems}` : "-"}</td>
                      <td className="py-3 pr-4">
                        {p.lastSeenAt ? fmtDateTime(p.lastSeenAt) : "-"}
                        {p.participantStatus === "started" ? <span className="block text-[13px] text-ink-2">{onlineRecently(p) ? "Terhubung" : "Tidak ada sinyal"}</span> : null}
                      </td>
                      <td className="py-3 pr-4">{p.score ?? (p.essayPending ? `${p.essayPending} esai belum dinilai` : "-")}</td>
                      <td className="py-3">
                        {p.resultStatus ? rStatus[p.resultStatus] : "-"}
                        {p.attemptId && p.resultStatus && can("exams.grade") ? (
                          <Link className="block text-sm text-primary underline" to={`/staf/penilaian/${p.attemptId}`}>
                            Penilaian esai
                          </Link>
                        ) : null}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          <Dialog open={inviteOpen} onOpenChange={setInviteOpen} title="Undang peserta" description="Hanya lamaran aktif di tahap Ujian dalam akses Anda yang dapat diundang.">
            {err ? <Alert tone="danger">{err.fieldErrors.extraMinutes ?? err.message}</Alert> : null}
            <PageState loading={apps.loading} error={apps.error} empty={apps.data?.items.length === 0} emptyText="Tidak ada lamaran di tahap Ujian.">
              <fieldset className="flex max-h-64 flex-col gap-1 overflow-y-auto">
                <legend className="mb-2 text-sm font-semibold">Pilih lamaran</legend>
                {apps.data?.items.map((a) => (
                  <label key={a.id} className="flex min-h-11 items-center gap-3">
                    <input
                      type="checkbox"
                      className="h-5 w-5"
                      checked={selected.has(a.id)}
                      onChange={(e) => {
                        const n = new Set(selected);
                        if (e.target.checked) n.add(a.id);
                        else n.delete(a.id);
                        setSelected(n);
                      }}
                    />
                    <span>
                      {a.candidateName} · {a.jobTitle}
                      {invited.has(a.candidateName) ? <span className="text-[13px] text-ink-2"> (mungkin sudah diundang)</span> : null}
                    </span>
                  </label>
                ))}
              </fieldset>
            </PageState>
            <div className="mt-3 grid gap-3 sm:grid-cols-2">
              <Field label="Akomodasi waktu (menit)" hint="0 bila tidak ada.">
                {({ id: fid, describedBy }) => <Input id={fid} aria-describedby={describedBy} inputMode="numeric" value={extra} onChange={(e) => setExtra(e.target.value)} />}
              </Field>
              <Field label="Alasan akomodasi" required={Number(extra) > 0}>
                {({ id: fid }) => <Textarea id={fid} className="min-h-11" value={reason} onChange={(e) => setReason(e.target.value)} />}
              </Field>
            </div>
            <div className="mt-4 flex justify-end gap-2">
              <Button onClick={() => setInviteOpen(false)}>Batal</Button>
              <Button variant="primary" busy={busy} disabled={selected.size === 0} onClick={invite}>
                Undang {selected.size || ""}
              </Button>
            </div>
          </Dialog>

          <Dialog
            open={releaseOpen}
            onOpenChange={setReleaseOpen}
            title="Rilis nilai?"
            description="Nilai yang dirilis memicu perpindahan tahap lamaran sesuai workflow. Peserta dengan esai belum dinilai diarahkan ke penilaian manual."
          >
            {err ? <Alert tone="danger">{err.message}</Alert> : null}
            <div className="mt-4 flex justify-end gap-2">
              <Button onClick={() => setReleaseOpen(false)}>Batal</Button>
              <Button variant="primary" busy={busy} onClick={release}>
                Rilis nilai
              </Button>
            </div>
          </Dialog>
        </div>
      ) : null}
    </PageState>
  );
}

type Review = {
  attemptId: string;
  participantCode: string;
  status: string;
  items: { itemId: string; number: number; stem: string; weight: number; rubric: string; answer: { text?: string } | null }[];
  gradeHistory: { id: number; itemId: string; points: number; assessorName: string; reason: string; createdAt: string }[];
  result: { score: number | null; essayPending: number; status: string } | null;
};

export function EssayGrading() {
  const { id = "" } = useParams();
  const q = useQuery<Review>(`/exam-attempts/${id}/review`);
  const toast = useToast();
  const [points, setPoints] = useState<Record<string, string>>({});
  const [reasons, setReasons] = useState<Record<string, string>>({});
  const [err, setErr] = useState<ApiError | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const grade = async (itemId: string) => {
    setBusy(itemId);
    setErr(null);
    try {
      await post(`/exam-attempts/${id}/essay-grades`, { itemId, points: Number(points[itemId]), reason: reasons[itemId] ?? "" });
      toast("success", "Nilai esai tersimpan.");
      q.reload();
    } catch (e) {
      setErr(e as ApiError);
    } finally {
      setBusy(null);
    }
  };

  return (
    <PageState loading={q.loading} error={q.error} onRetry={q.reload}>
      {q.data ? (
        <div className="max-w-3xl">
          <PageHeader
            title={`Penilaian esai ${q.data.participantCode}`}
            subtitle={`Identitas peserta disembunyikan. Nilai saat ini: ${q.data.result?.score ?? "belum lengkap"}${q.data.result?.status === "released" ? " (sudah dirilis, perubahan dicatat sebagai regrade)" : ""}`}
          />
          {err ? (
            <div className="mb-4">
              <Alert tone="danger">{err.fieldErrors.points ?? err.fieldErrors.reason ?? err.message}</Alert>
            </div>
          ) : null}
          {q.data.items.length === 0 ? <p className="text-ink-2">Ujian ini tidak memiliki soal esai.</p> : null}
          {q.data.items.map((it) => {
            const hist = q.data!.gradeHistory.filter((g) => g.itemId === it.itemId);
            return (
              <section key={it.itemId} className="mb-8 border-b border-line pb-6">
                <h2 className="text-[18px]">Soal {it.number}</h2>
                <p className="mt-1 whitespace-pre-line">{it.stem}</p>
                <p className="mt-2 text-sm text-ink-2">Rubrik: {it.rubric || "-"} · Bobot maksimal {it.weight}</p>
                <div className="mt-3 rounded-md bg-surface p-3 whitespace-pre-line">{it.answer?.text || "Tidak dijawab."}</div>
                {hist.length ? (
                  <ul className="mt-3 text-sm">
                    {hist.map((g) => (
                      <li key={g.id}>
                        {fmtDateTime(g.createdAt)} · {g.assessorName}: {g.points}
                        {g.reason ? ` (${g.reason})` : ""}
                      </li>
                    ))}
                  </ul>
                ) : null}
                <div className="mt-3 grid gap-3 sm:grid-cols-[140px_1fr_auto] sm:items-end">
                  <Field label="Nilai" required>
                    {({ id: fid }) => <Input id={fid} inputMode="decimal" value={points[it.itemId] ?? ""} onChange={(e) => setPoints({ ...points, [it.itemId]: e.target.value })} />}
                  </Field>
                  <Field label={hist.length ? "Alasan regrade" : "Catatan"} required={hist.length > 0}>
                    {({ id: fid }) => <Input id={fid} value={reasons[it.itemId] ?? ""} onChange={(e) => setReasons({ ...reasons, [it.itemId]: e.target.value })} />}
                  </Field>
                  <Button variant="primary" busy={busy === it.itemId} disabled={!points[it.itemId]} onClick={() => grade(it.itemId)}>
                    Simpan nilai
                  </Button>
                </div>
              </section>
            );
          })}
        </div>
      ) : null}
    </PageState>
  );
}
