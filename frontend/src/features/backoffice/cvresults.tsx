import { useMemo, useState } from "react";
import { Link, useParams } from "react-router";
import { type Schemas } from "../../lib/api";
import { fmtDateTime } from "../../lib/format";
import { useQuery } from "../../lib/useQuery";
import { Alert, Button, Dialog, PageHeader, PageState } from "../../ui";
import {
  displayValue,
  fieldLabel,
  groups,
  needsCheck,
  type Run,
  type Suggestion,
} from "../candidate/cv/model";
import { SourceViewer } from "../candidate/cv/SourceViewer";

// Keputusan kandidat atas setiap usulan, dilihat staf apa adanya (baca saja).
const decisionLabel: Record<string, string> = {
  accepted: "Diterima kandidat",
  edited: "Diubah kandidat",
  rejected: "Ditolak kandidat",
  pending: "Belum diputuskan",
};

const runStatusText: Record<string, string> = {
  ready_for_review: "Belum disimpan kandidat ke profil",
  partial: "Sebagian halaman tidak terbaca",
  applied: "Sudah diterapkan kandidat ke profil",
};

/** Hasil pembacaan CV kandidat untuk staf: usulan, keputusan kandidat, dan bukti di CV. Akses dicatat di audit. */
export function StaffCvResult() {
  const { id = "", runId = "" } = useParams();
  const app = useQuery<Schemas["ApplicationDetail"]>(`/applications/${id}`);
  const q = useQuery<Run>(`/cv-parses/${runId}`);
  const [focus, setFocus] = useState<Suggestion | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const run = q.data;
  const evidenceById = useMemo(
    () => new Map((run?.evidence ?? []).map((e) => [e.id, e])),
    [run],
  );
  const summary = app.data?.cvParses.find((r) => r.id === runId);

  const show = (s: Suggestion) => {
    setFocus(s);
    if (!window.matchMedia("(min-width: 1024px)").matches) setDialogOpen(true);
  };

  return (
    <PageState
      loading={q.loading || app.loading}
      error={q.error ?? app.error}
      onRetry={() => {
        q.reload();
        app.reload();
      }}
    >
      {run && app.data ? (
        <div className="max-w-6xl">
          <Link
            to={`/staf/lamaran/${id}`}
            className="text-sm text-primary hover:underline"
          >
            Kembali ke lamaran {app.data.candidate.name}
          </Link>
          <PageHeader
            title="Hasil pembacaan CV"
            subtitle={`${app.data.candidate.name} · ${summary?.documentName ?? "CV"} · dibaca ${fmtDateTime(run.finishedAt ?? run.createdAt)}`}
          />
          <div className="mb-4">
            <Alert tone="info" title={runStatusText[run.status] ?? run.status}>
              Usulan dibaca otomatis dari CV dan diperiksa sendiri oleh
              kandidat. Nilai yang dipakai untuk lamaran tetap "Data saat
              melamar" di halaman lamaran.
            </Alert>
          </div>
          {run.pages.some((p) => p.status === "failed") ? (
            <div className="mb-4">
              <Alert tone="warning" title="Sebagian halaman tidak terbaca">
                {run.pages
                  .filter((p) => p.status === "failed")
                  .map(
                    (p) => `Halaman ${p.no}: ${p.message ?? "tidak terbaca"}`,
                  )
                  .join(" ")}
              </Alert>
            </div>
          ) : null}
          <div className="grid gap-6 lg:grid-cols-[3fr_2fr]">
            <div className="flex flex-col gap-6">
              {groups(run).map((g) => (
                <section key={g.key} aria-label={g.title}>
                  <h2 className="mb-2 text-[18px]">{g.title}</h2>
                  <ul className="divide-y divide-line border-y border-line">
                    {g.items.map((s) => (
                      <li
                        key={s.id}
                        className={`flex flex-wrap items-start justify-between gap-3 py-3 ${focus?.id === s.id ? "bg-surface" : ""}`}
                      >
                        <div className="min-w-0">
                          <p className="text-sm text-ink-2">{fieldLabel(s)}</p>
                          <p className="whitespace-pre-line">
                            {displayValue(s)}
                          </p>
                          {needsCheck(s) ? (
                            <p className="mt-1 text-[13px] text-warning">
                              {[...s.readDoubts, ...s.assocDoubts].join(
                                " · ",
                              ) || "Perlu diperiksa"}
                            </p>
                          ) : null}
                        </div>
                        <div className="flex shrink-0 flex-col items-end gap-1">
                          <span className="text-sm text-ink-2">
                            {decisionLabel[s.status] ?? s.status}
                          </span>
                          {s.evidenceIds.length ? (
                            <Button
                              size="sm"
                              variant="ghost"
                              aria-label={`Lihat sumber ${fieldLabel(s)}`}
                              onClick={() => show(s)}
                            >
                              Lihat sumber
                            </Button>
                          ) : null}
                        </div>
                      </li>
                    ))}
                  </ul>
                </section>
              ))}
              {run.suggestions.length === 0 ? (
                <p className="text-ink-2">
                  Tidak ada data yang terbaca dari CV ini.
                </p>
              ) : null}
            </div>
            <aside className="hidden lg:block">
              <div className="sticky top-4">
                <SourceViewer
                  run={run}
                  s={focus}
                  evidenceById={evidenceById}
                  imageBase={`/api/v1/cv-parses/${run.id}`}
                />
              </div>
            </aside>
          </div>
          <Dialog
            open={dialogOpen}
            onOpenChange={setDialogOpen}
            title="Sumber di CV"
          >
            <SourceViewer
              run={run}
              s={focus}
              evidenceById={evidenceById}
              imageBase={`/api/v1/cv-parses/${run.id}`}
            />
            <div className="mt-4 flex justify-end">
              <Button onClick={() => setDialogOpen(false)}>Tutup</Button>
            </div>
          </Dialog>
        </div>
      ) : null}
    </PageState>
  );
}
