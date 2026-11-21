import { useState } from "react";
import { useSearchParams } from "react-router";
import { ApiError, post } from "../../lib/api";
import { fmtDateTime, getOrgTimeZone } from "../../lib/format";
import { useSession } from "../../lib/session";
import { useQuery } from "../../lib/useQuery";
import { Alert, Button, Field, Input, PageHeader, PageState, useToast } from "../../ui";

type JobRow = { jobId: string; jobTitle: string; applications: number; candidates: number; active: number; withdrawn: number; rejected: number; hired: number; reached: Record<string, number> };
type Report = {
  range: { from: string; to: string; timezone: string };
  definitions: Record<string, string>;
  totals: JobRow;
  jobs: JobRow[];
  durations: { stageKey: string; samples: number; avgHours: number; medianHours: number }[];
};
type Export = { id: string; kind: string; status: string; rowCount: number | null; error: string | null; createdAt: string; expiresAt: string | null };

const stageOrder = ["screening", "ujian", "review", "interview", "persetujuan", "diterima", "ditolak"];
const stageName: Record<string, string> = { screening: "Screening", ujian: "Ujian", review: "Review", interview: "Interview", persetujuan: "Persetujuan", diterima: "Diterima", ditolak: "Ditolak" };
const exportStatus: Record<string, string> = { queued: "Dalam antrean", running: "Diproses", ready: "Siap diunduh", failed: "Gagal", expired: "Kedaluwarsa" };

function today(offsetDays = 0) {
  const d = new Date(Date.now() + offsetDays * 86_400_000);
  return new Intl.DateTimeFormat("en-CA", { timeZone: getOrgTimeZone() }).format(d);
}

export function ReportsPage() {
  const [params, setParams] = useSearchParams();
  const from = params.get("from") ?? today(-89);
  const to = params.get("to") ?? today();
  const q = useQuery<Report>(`/reports/recruitment?from=${from}&to=${to}`);
  const exports = useQuery<{ items: Export[] }>("/reports/exports");
  const { can } = useSession();
  const toast = useToast();
  const [f, setF] = useState({ from, to });
  const [err, setErr] = useState<ApiError | null>(null);

  const requestExport = async (kind: "funnel" | "applications") => {
    setErr(null);
    try {
      await post("/reports/exports", { kind, from, to });
      toast("success", "Ekspor masuk antrean. Muat ulang daftar ekspor beberapa saat lagi.");
      exports.reload();
    } catch (e) {
      setErr(e as ApiError);
    }
  };
  const download = async (id: string) => {
    try {
      const r = await post<{ url: string }>(`/reports/exports/${id}/link`);
      window.location.assign(r.url);
    } catch (e) {
      setErr(e as ApiError);
    }
  };

  const stages = q.data ? stageOrder.filter((s) => q.data!.jobs.some((j) => j.reached[s])) : [];

  return (
    <div className="max-w-6xl">
      <PageHeader title="Laporan rekrutmen" subtitle="Hanya lowongan dan unit dalam akses Anda." />
      <form
        className="mb-6 flex flex-wrap items-end gap-3 rounded-md border border-line bg-surface p-3"
        onSubmit={(e) => {
          e.preventDefault();
          setParams({ from: f.from, to: f.to });
        }}
      >
        <Field label="Dari tanggal" required>
          {({ id }) => <Input id={id} type="date" value={f.from} onChange={(e) => setF({ ...f, from: e.target.value })} />}
        </Field>
        <Field label="Sampai tanggal" required>
          {({ id }) => <Input id={id} type="date" value={f.to} onChange={(e) => setF({ ...f, to: e.target.value })} />}
        </Field>
        <Button type="submit" variant="primary">
          Terapkan
        </Button>
      </form>
      {err ? (
        <div className="mb-4">
          <Alert tone={err.status === 503 ? "warning" : "danger"}>{err.message}</Alert>
        </div>
      ) : null}
      <PageState loading={q.loading} error={q.error} onRetry={q.reload} empty={q.data?.jobs.length === 0} emptyText="Tidak ada lamaran pada rentang ini.">
        {q.data ? (
          <>
            <dl className="mb-6 grid grid-cols-2 gap-4 sm:grid-cols-5">
              {(
                [
                  ["Lamaran", q.data.totals.applications],
                  ["Kandidat unik", q.data.totals.candidates],
                  ["Aktif", q.data.totals.active],
                  ["Ditarik", q.data.totals.withdrawn],
                  ["Diterima", q.data.totals.hired],
                ] as [string, number][]
              ).map(([k, v]) => (
                <div key={k} className="border-l-4 border-primary bg-surface px-3 py-2">
                  <dt className="text-sm text-ink-2">{k}</dt>
                  <dd className="text-[22px] font-bold">{v}</dd>
                </div>
              ))}
            </dl>
            <div className="overflow-x-auto">
              <table className="w-full border-collapse text-left text-[15px]">
                <caption className="mb-2 text-left text-sm text-ink-2">
                  Funnel per lowongan: jumlah lamaran yang pernah mencapai tiap tahap. Kohort {q.data.range.from} sampai {q.data.range.to} ({q.data.range.timezone}).
                </caption>
                <thead>
                  <tr className="border-b border-line text-sm text-ink-2">
                    <th scope="col" className="py-2 pr-4 font-semibold">Lowongan</th>
                    <th scope="col" className="py-2 pr-4 font-semibold">Lamaran</th>
                    {stages.map((s) => (
                      <th key={s} scope="col" className="py-2 pr-4 font-semibold">
                        {stageName[s] ?? s}
                      </th>
                    ))}
                    <th scope="col" className="py-2 pr-4 font-semibold">Ditarik</th>
                  </tr>
                </thead>
                <tbody>
                  {q.data.jobs.map((j) => (
                    <tr key={j.jobId} className="border-b border-line">
                      <th scope="row" className="py-2 pr-4 font-semibold">
                        {j.jobTitle}
                      </th>
                      <td className="py-2 pr-4">{j.applications}</td>
                      {stages.map((s) => (
                        <td key={s} className="py-2 pr-4">
                          {j.reached[s] ?? 0}
                          {j.applications ? <span className="text-[13px] text-ink-2"> ({Math.round(((j.reached[s] ?? 0) / j.applications) * 100)}%)</span> : null}
                        </td>
                      ))}
                      <td className="py-2 pr-4">{j.withdrawn}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {q.data.durations.length ? (
              <section className="mt-6">
                <h2 className="mb-2 text-[18px]">Waktu per tahap (perpindahan yang sudah selesai)</h2>
                <ul className="text-[15px]">
                  {q.data.durations.map((d) => (
                    <li key={d.stageKey}>
                      {stageName[d.stageKey] ?? d.stageKey}: median {d.medianHours.toFixed(1)} jam, rata-rata {d.avgHours.toFixed(1)} jam ({d.samples} lamaran)
                    </li>
                  ))}
                </ul>
              </section>
            ) : null}
            <details className="mt-6 text-sm text-ink-2">
              <summary className="cursor-pointer font-semibold text-ink">Cara menghitung</summary>
              <ul className="mt-2 list-disc pl-5">
                {Object.entries(q.data.definitions).map(([k, v]) => (
                  <li key={k}>{v}</li>
                ))}
              </ul>
            </details>
          </>
        ) : null}
      </PageState>

      {can("reports.export") || can("pii.export") ? (
        <section className="mt-10">
          <h2 className="mb-3 text-[20px]">Ekspor</h2>
          <div className="mb-4 flex flex-wrap gap-2">
            {can("reports.export") ? <Button onClick={() => requestExport("funnel")}>Ekspor funnel (CSV)</Button> : null}
            {can("pii.export") ? <Button onClick={() => requestExport("applications")}>Ekspor data pelamar (CSV)</Button> : null}
            <Button variant="ghost" onClick={exports.reload}>
              Muat ulang daftar
            </Button>
          </div>
          <PageState loading={exports.loading} error={exports.error} empty={exports.data?.items.length === 0} emptyText="Belum ada ekspor.">
            <ul className="divide-y divide-line border-y border-line">
              {exports.data?.items.map((x) => (
                <li key={x.id} className="flex flex-wrap items-center justify-between gap-2 py-3">
                  <span>
                    {x.kind === "funnel" ? "Funnel" : "Data pelamar"} · {fmtDateTime(x.createdAt)} · {exportStatus[x.status] ?? x.status}
                    {x.rowCount !== null ? ` · ${x.rowCount} baris` : ""}
                    {x.error ? ` · ${x.error}` : ""}
                  </span>
                  {x.status === "ready" ? (
                    <Button size="sm" onClick={() => download(x.id)}>
                      Unduh
                    </Button>
                  ) : null}
                </li>
              ))}
            </ul>
          </PageState>
          <p className="mt-2 text-[13px] text-ink-2">Tautan unduh berlaku 60 detik. Berkas ekspor dihapus otomatis setelah 24 jam.</p>
        </section>
      ) : null}
    </div>
  );
}
