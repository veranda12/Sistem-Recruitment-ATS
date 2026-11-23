import { useState } from "react";
import { Link, useSearchParams } from "react-router";
import { ApiError, get, post, type Schemas } from "../../lib/api";
import { fmtDate } from "../../lib/format";
import { useQuery } from "../../lib/useQuery";
import { Alert, Button, Dialog, Field, PageState, Select, Textarea, useToast, cx } from "../../ui";

type Card = Schemas["StaffApplication"];
type Board = { job: { id: string; title: string; unitName: string }; stages: { key: string; name: string; type: string }[]; items: Card[]; truncated: boolean };
type Move = { app: Card; to: { key: string; name: string }; version: number };

/** Papan kanban per lowongan: kolom = tahap workflow. Seret kartu untuk memindahkan tahap bila workflow mengizinkan. */
export function BoardView() {
  const [params, setParams] = useSearchParams();
  const jobs = useQuery<{ items: { id: string; title: string; unitName: string; status: string }[] }>("/staff/jobs");
  const options = (jobs.data?.items ?? []).filter((j) => j.status !== "draft");
  const jobId = params.get("lowongan") ?? options[0]?.id ?? "";
  const board = useQuery<Board>(jobId ? `/staff/jobs/${jobId}/board` : null);
  const toast = useToast();
  const [notice, setNotice] = useState<string | null>(null);
  const [move, setMove] = useState<Move | null>(null);
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<ApiError | null>(null);
  const [over, setOver] = useState<string | null>(null);

  const drop = async (appId: string, to: { key: string; name: string; type: string }) => {
    setOver(null);
    setNotice(null);
    const app = board.data?.items.find((a) => a.id === appId);
    if (!app || app.stageKey === to.key) return;
    if (to.type === "terminal") {
      setNotice(`Keputusan akhir (${to.name}) dibuat dari halaman lamaran atau persetujuan, agar alasannya tercatat.`);
      return;
    }
    try {
      const d = await get<Schemas["ApplicationDetail"]>(`/applications/${appId}`);
      if (!d.actions.some((a) => a.kind === "manual" && a.to === to.key)) {
        setNotice(`${app.candidateName} tidak dapat dipindahkan dari ${app.stageName} ke ${to.name} oleh peran Anda. Perpindahan ini diatur workflow.`);
        return;
      }
      setErr(null);
      setReason("");
      setMove({ app, to, version: d.version });
    } catch (e) {
      setNotice((e as ApiError).message);
    }
  };

  const confirm = async () => {
    if (!move) return;
    setBusy(true);
    setErr(null);
    try {
      await post(`/applications/${move.app.id}/transitions`, { kind: "manual", to: move.to.key, reason, expectedVersion: move.version });
      toast("success", `${move.app.candidateName} dipindahkan ke ${move.to.name}.`);
      setMove(null);
      board.reload();
    } catch (e) {
      setErr(e as ApiError);
    } finally {
      setBusy(false);
    }
  };

  return (
    <PageState loading={jobs.loading} error={jobs.error} onRetry={jobs.reload} empty={options.length === 0} emptyText="Belum ada lowongan dalam akses Anda.">
      <div className="mb-4 flex flex-wrap items-end gap-3 rounded-md border border-line bg-surface p-3">
        <label className="flex flex-col gap-1 text-sm font-semibold">
          Lowongan
          <Select value={jobId} onChange={(e) => setParams({ tampilan: "papan", lowongan: e.target.value })}>
            {options.map((j) => (
              <option key={j.id} value={j.id}>
                {j.title} · {j.unitName}
              </option>
            ))}
          </Select>
        </label>
        <p className="text-sm text-ink-2">Seret kartu ke kolom lain untuk memindahkan tahap, atau buka lamaran untuk tindakan lainnya.</p>
      </div>
      {notice ? (
        <div className="mb-4">
          <Alert tone="warning" title={notice} />
        </div>
      ) : null}
      <PageState loading={board.loading} error={board.error} onRetry={board.reload}>
        {board.data ? (
          <>
            {board.data.truncated ? <p className="mb-2 text-sm text-ink-2">Menampilkan 500 lamaran terbaru.</p> : null}
            <div className="flex gap-3 overflow-x-auto pb-3" role="list" aria-label={`Papan lamaran ${board.data.job.title}`}>
              {board.data.stages.map((st) => {
                const cards = board.data!.items.filter((a) => a.stageKey === st.key);
                return (
                  <section
                    key={st.key}
                    role="listitem"
                    aria-label={`${st.name}, ${cards.length} lamaran`}
                    className={cx("flex w-72 shrink-0 flex-col rounded-md border bg-surface", over === st.key ? "border-primary" : "border-line")}
                    onDragOver={(e) => {
                      e.preventDefault();
                      setOver(st.key);
                    }}
                    onDragLeave={() => setOver((o) => (o === st.key ? null : o))}
                    onDrop={(e) => {
                      e.preventDefault();
                      void drop(e.dataTransfer.getData("text/plain"), st);
                    }}
                  >
                    <h2 className="border-b border-line px-3 py-2 text-[15px] font-semibold">
                      {st.name} <span className="font-normal text-ink-2">{cards.length}</span>
                    </h2>
                    <ul className="flex min-h-24 flex-col gap-2 p-2">
                      {cards.map((a) => (
                        <li
                          key={a.id}
                          draggable
                          onDragStart={(e) => e.dataTransfer.setData("text/plain", a.id)}
                          className="cursor-grab rounded-md border border-line bg-white p-3 text-sm"
                        >
                          <Link className="font-semibold text-primary hover:underline" to={`/staf/lamaran/${a.id}`}>
                            {a.candidateName}
                          </Link>
                          {a.shortlisted ? <span className="ml-2 rounded-sm bg-primary-soft px-1.5 py-0.5 text-[12px] font-semibold text-primary">Shortlist</span> : null}
                          <p className="mt-1 text-ink-2">
                            {a.status !== "active" ? `${statusText[a.status] ?? a.status} · ` : ""}
                            Dikirim {fmtDate(a.appliedAt)}
                          </p>
                          <p className="text-ink-2">Recruiter: {a.recruiterName ?? "-"}</p>
                        </li>
                      ))}
                    </ul>
                  </section>
                );
              })}
            </div>
          </>
        ) : null}
      </PageState>
      <Dialog
        open={!!move}
        onOpenChange={(o) => !o && setMove(null)}
        title={move ? `Pindahkan ${move.app.candidateName} ke ${move.to.name}?` : ""}
        description="Tindakan ini dicatat beserta nama Anda dan alasannya."
      >
        {err ? <Alert tone="danger" title={err.message} /> : null}
        <Field label="Catatan" error={err?.fieldErrors.reason}>
          {({ id, invalid }) => <Textarea id={id} invalid={invalid} value={reason} maxLength={1000} onChange={(e) => setReason(e.target.value)} />}
        </Field>
        <div className="mt-4 flex justify-end gap-2">
          <Button onClick={() => setMove(null)}>Batal</Button>
          <Button variant="primary" busy={busy} onClick={confirm}>
            Konfirmasi
          </Button>
        </div>
      </Dialog>
    </PageState>
  );
}

const statusText: Record<string, string> = { withdrawn: "Ditarik", rejected: "Ditolak", hired: "Diterima" };
