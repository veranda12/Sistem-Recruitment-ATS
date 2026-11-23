import { useEffect, useState } from "react";
import { Link, useParams } from "react-router";
import { ApiError, post, put, type Schemas } from "../../lib/api";
import { fmtDateTime } from "../../lib/format";
import { useSession } from "../../lib/session";
import { useQuery } from "../../lib/useQuery";
import { Alert, Badge, Button, Dialog, Field, Input, PageHeader, PageState, Select, useToast } from "../../ui";

type Def = Schemas["WorkflowDefinition"];
type Stage = Schemas["WorkflowStage"];
type Edge = Schemas["WorkflowEdge"];
type Rule = Schemas["Rule"];
type Issue = Schemas["Issue"];
type Detail = Schemas["WorkflowDetail"];

export function WorkflowsPage() {
  const q = useQuery<{ items: { id: string; key: string; name: string; draftRevision: number; activeVersionNo: number | null }[] }>("/workflows");
  return (
    <div>
      <PageHeader title="Workflow" subtitle="Tahapan proses rekrutmen per lowongan. Lamaran berjalan tetap memakai versi yang berlaku saat melamar." />
      <PageState loading={q.loading} error={q.error} onRetry={q.reload} empty={q.data?.items.length === 0} emptyText="Belum ada workflow.">
        <ul className="divide-y divide-line border-y border-line">
          {q.data?.items.map((w) => (
            <li key={w.id} className="flex flex-wrap items-center justify-between gap-2 py-4">
              <Link to={`/staf/workflow/${w.id}`} className="text-[17px] font-semibold text-primary hover:underline">
                {w.name}
              </Link>
              <span className="text-sm text-ink-2">{w.activeVersionNo ? `Versi aktif ${w.activeVersionNo}` : "Belum diterbitkan"}</span>
            </li>
          ))}
        </ul>
      </PageState>
    </div>
  );
}

// Rule editor: kondisi AND sederhana (field/op/value). Rule bersarang 'any' ditampilkan read-only.
type Cond = { field: string; op: string; value: string };
function ruleToConds(r: Rule | undefined): Cond[] | null {
  if (!r) return [];
  const leafs = r.all ?? (r.field ? [r] : null);
  if (!leafs || leafs.some((l) => !l.field)) return null;
  return leafs.map((l) => ({ field: l.field!, op: l.op!, value: Array.isArray(l.value) ? (l.value as string[]).join(",") : String(l.value) }));
}
function condsToRule(cs: Cond[], fields: Detail["catalog"]["fields"]): Rule | undefined {
  if (cs.length === 0) return undefined;
  const leaf = (c: Cond): Rule => {
    const t = fields[c.field]?.type;
    const value = t === "number" ? Number(c.value) : t === "bool" ? c.value === "true" : c.op === "in" ? c.value.split(",").map((s) => s.trim()) : c.value;
    return { field: c.field, op: c.op as Rule["op"], value };
  };
  return cs.length === 1 ? leaf(cs[0]!) : { all: cs.map(leaf) };
}

const stageTypeLabel: Record<string, string> = { screening: "Screening", interview: "Interview", assessment: "Ujian", approval: "Persetujuan", decision: "Keputusan", terminal: "Tahap akhir" };
const triggerLabel: Record<string, string> = { manual: "Manual oleh staf", event: "Event hasil ujian", approval: "Hasil persetujuan" };

export function WorkflowEditor() {
  const { id = "" } = useParams();
  const q = useQuery<Detail>(`/workflows/${id}`);
  const { can } = useSession();
  const toast = useToast();
  const [def, setDef] = useState<Def | null>(null);
  const [name, setName] = useState("");
  const [rev, setRev] = useState(0);
  const [issues, setIssues] = useState<Issue[]>([]);
  const [dirty, setDirty] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<ApiError | null>(null);
  const [sim, setSim] = useState<{ score: string; manual: boolean; result: string | null }>({ score: "80", manual: false, result: null });
  const [publishOpen, setPublishOpen] = useState(false);
  const [notes, setNotes] = useState("");
  const [rollback, setRollback] = useState<{ id: string; no: number } | null>(null);

  useEffect(() => {
    if (q.data) {
      setDef(q.data.draft);
      setName(q.data.name);
      setRev(q.data.draftRevision);
      setIssues(q.data.issues);
      setDirty(false);
    }
  }, [q.data]);

  const editable = can("workflows.manage");
  const update = (d: Def) => {
    setDef(d);
    setDirty(true);
  };
  const setStage = (i: number, p: Partial<Stage>) => update({ ...def!, stages: def!.stages.map((s, j) => (j === i ? { ...s, ...p } : s)) });
  const setEdge = (i: number, p: Partial<Edge>) => update({ ...def!, edges: def!.edges.map((e, j) => (j === i ? { ...e, ...p } : e)) });

  const wrap = async (fn: () => Promise<void>) => {
    setBusy(true);
    setErr(null);
    try {
      await fn();
    } catch (e) {
      const ae = e as ApiError;
      setErr(ae);
      if (Array.isArray(ae.details)) setIssues(ae.details as Issue[]);
    } finally {
      setBusy(false);
    }
  };

  const save = () =>
    wrap(async () => {
      const r = await put<{ draftRevision: number; issues: Issue[] }>(`/workflows/${id}/draft`, { name, definition: def, draftRevision: rev });
      setRev(r.draftRevision);
      setIssues(r.issues);
      setDirty(false);
      toast(r.issues.length ? "warning" : "success", r.issues.length ? `Draft tersimpan dengan ${r.issues.length} temuan.` : "Draft tersimpan dan valid.");
    });
  const validate = () =>
    wrap(async () => {
      const r = await post<{ valid: boolean; issues: Issue[] }>(`/workflows/${id}/validate`, { definition: def });
      setIssues(r.issues);
      toast(r.valid ? "success" : "warning", r.valid ? "Workflow valid." : `${r.issues.length} temuan perlu diperbaiki.`);
    });
  const simulate = () =>
    wrap(async () => {
      const facts: Record<string, unknown> = { "assessment.status": "released" };
      if (sim.manual) facts["assessment.pending_manual"] = true;
      else if (sim.score !== "") facts["assessment.score"] = Number(sim.score);
      const examStage = def!.stages.find((s) => s.type === "assessment");
      const path = examStage ? def!.edges.find((e) => e.to === examStage.key && e.trigger === "manual") : undefined;
      const steps = [...(path ? [{ kind: "manual", to: path.to }] : []), { kind: "event", event: "assessment.released", facts }];
      const r = await post<{ steps: { stage: string; matched: string; ok: boolean }[] }>(`/workflows/${id}/simulate`, { definition: def, steps });
      const last = r.steps[r.steps.length - 1]!;
      const st = def!.stages.find((s) => s.key === last.stage);
      setSim((s) => ({ ...s, result: `${r.steps.map((x) => def!.stages.find((s2) => s2.key === x.stage)?.name ?? x.stage).join(" > ")}. Berakhir di ${st?.name ?? last.stage} (${last.matched}).` }));
    });
  const publish = () =>
    wrap(async () => {
      const r = await post<{ versionNo: number }>(`/workflows/${id}/publish`, { draftRevision: rev, notes });
      toast("success", `Versi ${r.versionNo} diterbitkan untuk lamaran baru.`);
      setPublishOpen(false);
      setNotes("");
      q.reload();
    });
  const doRollback = () =>
    wrap(async () => {
      await post(`/workflows/${id}/rollback`, { versionId: rollback!.id, reason: notes });
      toast("success", `Versi ${rollback!.no} kembali aktif untuk lamaran baru.`);
      setRollback(null);
      setNotes("");
      q.reload();
    });

  const fields = q.data?.catalog.fields ?? {};
  const opsFor = (f: string) => q.data?.catalog.ops[fields[f]?.type ?? "string"] ?? [];

  return (
    <PageState loading={q.loading} error={q.error} onRetry={q.reload}>
      {def && q.data ? (
        <div className="max-w-5xl">
          <Link to="/staf/workflow" className="text-sm text-primary hover:underline">
            Kembali ke daftar workflow
          </Link>
          <PageHeader
            title={name}
            subtitle={`Revisi draft ${rev}${dirty ? " · ada perubahan belum disimpan" : ""}`}
            actions={
              <>
                <Button onClick={validate} busy={busy}>
                  Validasi
                </Button>
                {editable ? (
                  <Button onClick={save} busy={busy} variant={dirty ? "primary" : "secondary"}>
                    Simpan draft
                  </Button>
                ) : null}
                {can("workflows.publish") ? (
                  <Button onClick={() => setPublishOpen(true)} disabled={dirty} title={dirty ? "Simpan draft sebelum menerbitkan" : undefined}>
                    Terbitkan
                  </Button>
                ) : null}
              </>
            }
          />
          {err && err.code !== "workflow_invalid" ? <div className="mb-4"><Alert tone="danger">{err.message}</Alert></div> : null}
          {issues.length > 0 ? (
            <div className="mb-6">
              <Alert tone="warning" title={`${issues.length} temuan validasi`}>
                <ul className="list-disc pl-5">
                  {issues.map((i, k) => (
                    <li key={k}>
                      {i.message} <span className="text-ink-2">({i.path})</span>
                    </li>
                  ))}
                </ul>
              </Alert>
            </div>
          ) : (
            <div className="mb-6">
              <Alert tone="success">Draft valid dan siap diterbitkan.</Alert>
            </div>
          )}

          <fieldset disabled={!editable} className="flex flex-col gap-8">
            <section>
              <h2 className="mb-3 text-[20px]">Tahapan</h2>
              <Field label="Tahap awal" required>
                {({ id: fid }) => (
                  <Select id={fid} value={def.start} onChange={(e) => update({ ...def, start: e.target.value })}>
                    {def.stages.map((s) => (
                      <option key={s.key} value={s.key}>
                        {s.name}
                      </option>
                    ))}
                  </Select>
                )}
              </Field>
              <ul className="mt-4 flex flex-col gap-3">
                {def.stages.map((s, i) => (
                  <li key={i} className="grid gap-3 rounded-md border border-line p-3 sm:grid-cols-[1fr_1fr_1fr_auto]">
                    <Field label="Kunci" required>{({ id: fid }) => <Input id={fid} value={s.key} onChange={(e) => setStage(i, { key: e.target.value })} />}</Field>
                    <Field label="Nama" required>{({ id: fid }) => <Input id={fid} value={s.name} onChange={(e) => setStage(i, { name: e.target.value })} />}</Field>
                    <Field label="Jenis" required>
                      {({ id: fid }) => (
                        <Select id={fid} value={s.type} onChange={(e) => setStage(i, { type: e.target.value as Stage["type"], outcome: e.target.value === "terminal" ? "rejected" : undefined })}>
                          {(q.data?.catalog.stageTypes ?? []).map((t) => (
                            <option key={t} value={t}>
                              {stageTypeLabel[t] ?? t}
                            </option>
                          ))}
                        </Select>
                      )}
                    </Field>
                    <div className="flex items-end">
                      <Button type="button" variant="ghost" onClick={() => update({ ...def, stages: def.stages.filter((_, j) => j !== i) })}>
                        Hapus
                      </Button>
                    </div>
                    {s.type === "terminal" ? (
                      <Field label="Hasil" required>
                        {({ id: fid }) => (
                          <Select id={fid} value={s.outcome ?? "rejected"} onChange={(e) => setStage(i, { outcome: e.target.value as "hired" | "rejected" })}>
                            <option value="hired">Diterima</option>
                            <option value="rejected">Ditolak</option>
                          </Select>
                        )}
                      </Field>
                    ) : null}
                    {s.type === "assessment" ? (
                      <Field label="Kunci paket ujian" required>{({ id: fid }) => <Input id={fid} value={s.assessmentKey ?? ""} onChange={(e) => setStage(i, { assessmentKey: e.target.value })} />}</Field>
                    ) : null}
                    {s.type === "approval" ? (
                      <>
                        <Field label="Peran penyetuju" required hint="Pisahkan dengan koma, mis. hiring_manager">
                          {({ id: fid, describedBy }) => (
                            <Input
                              id={fid}
                              aria-describedby={describedBy}
                              value={s.approval?.roles.join(",") ?? ""}
                              onChange={(e) => setStage(i, { approval: { roles: e.target.value.split(",").map((x) => x.trim()).filter(Boolean), quorum: s.approval?.quorum ?? 1, allowSelfApproval: s.approval?.allowSelfApproval ?? false } })}
                            />
                          )}
                        </Field>
                        <Field label="Kuorum" required>
                          {({ id: fid }) => (
                            <Input
                              id={fid}
                              inputMode="numeric"
                              value={s.approval?.quorum ?? 1}
                              onChange={(e) => setStage(i, { approval: { roles: s.approval?.roles ?? [], quorum: Number(e.target.value) || 0, allowSelfApproval: s.approval?.allowSelfApproval ?? false } })}
                            />
                          )}
                        </Field>
                      </>
                    ) : null}
                  </li>
                ))}
              </ul>
              <Button type="button" className="mt-3" onClick={() => update({ ...def, stages: [...def.stages, { key: `tahap_${def.stages.length + 1}`, name: "Tahap baru", type: "screening" }] })}>
                Tambah tahap
              </Button>
            </section>

            <section>
              <h2 className="mb-3 text-[20px]">Transisi dan aturan</h2>
              <ul className="flex flex-col gap-3">
                {def.edges.map((e, i) => {
                  const conds = ruleToConds(e.when);
                  return (
                    <li key={i} className="rounded-md border border-line p-3">
                      <div className="grid gap-3 sm:grid-cols-4">
                        <Field label="Dari" required>
                          {({ id: fid }) => (
                            <Select id={fid} value={e.from} onChange={(ev) => setEdge(i, { from: ev.target.value })}>
                              {def.stages.map((s) => (
                                <option key={s.key} value={s.key}>{s.name}</option>
                              ))}
                            </Select>
                          )}
                        </Field>
                        <Field label="Ke" required>
                          {({ id: fid }) => (
                            <Select id={fid} value={e.to} onChange={(ev) => setEdge(i, { to: ev.target.value })}>
                              {def.stages.map((s) => (
                                <option key={s.key} value={s.key}>{s.name}</option>
                              ))}
                            </Select>
                          )}
                        </Field>
                        <Field label="Pemicu" required>
                          {({ id: fid }) => (
                            <Select id={fid} value={e.trigger} onChange={(ev) => setEdge(i, { trigger: ev.target.value as Edge["trigger"], event: ev.target.value === "event" ? "assessment.released" : undefined })}>
                              {(q.data?.catalog.triggers ?? []).map((t) => (
                                <option key={t} value={t}>{triggerLabel[t] ?? t}</option>
                              ))}
                            </Select>
                          )}
                        </Field>
                        <Field label="Label" required>{({ id: fid }) => <Input id={fid} value={e.label} onChange={(ev) => setEdge(i, { label: ev.target.value })} />}</Field>
                        <Field label="Urutan" required>{({ id: fid }) => <Input id={fid} inputMode="numeric" value={e.order} onChange={(ev) => setEdge(i, { order: Number(ev.target.value) || 0 })} />}</Field>
                        <label className="flex min-h-11 items-center gap-2 self-end">
                          <input type="checkbox" className="h-5 w-5" checked={!!e.default} onChange={(ev) => setEdge(i, { default: ev.target.checked, when: ev.target.checked ? undefined : e.when })} />
                          Transisi default (fallback)
                        </label>
                      </div>
                      {!e.default ? (
                        <div className="mt-3">
                          <p className="text-sm font-semibold">Syarat (semua harus terpenuhi; nilai kosong dianggap belum memenuhi)</p>
                          {conds === null ? (
                            <p className="text-sm text-ink-2">Syarat bersarang tidak dapat diedit di formulir ini.</p>
                          ) : (
                            <>
                              {conds.map((c, k) => (
                                <div key={k} className="mt-2 grid gap-2 sm:grid-cols-[2fr_1fr_1fr_auto]">
                                  <Select aria-label={`Field syarat ${k + 1}`} value={c.field} onChange={(ev) => setEdge(i, { when: condsToRule(conds.map((x, m) => (m === k ? { ...x, field: ev.target.value, op: opsFor(ev.target.value)[0] ?? "eq" } : x)), fields) })}>
                                    {Object.keys(fields).map((f) => (
                                      <option key={f} value={f}>{f}</option>
                                    ))}
                                  </Select>
                                  <Select aria-label={`Operator syarat ${k + 1}`} value={c.op} onChange={(ev) => setEdge(i, { when: condsToRule(conds.map((x, m) => (m === k ? { ...x, op: ev.target.value } : x)), fields) })}>
                                    {opsFor(c.field).map((o) => (
                                      <option key={o} value={o}>{o}</option>
                                    ))}
                                  </Select>
                                  <Input aria-label={`Nilai syarat ${k + 1}`} value={c.value} onChange={(ev) => setEdge(i, { when: condsToRule(conds.map((x, m) => (m === k ? { ...x, value: ev.target.value } : x)), fields) })} />
                                  <Button type="button" variant="ghost" onClick={() => setEdge(i, { when: condsToRule(conds.filter((_, m) => m !== k), fields) })}>
                                    Hapus syarat
                                  </Button>
                                </div>
                              ))}
                              <Button type="button" size="sm" className="mt-2" onClick={() => setEdge(i, { when: condsToRule([...conds, { field: "assessment.score", op: "gte", value: "75" }], fields) })}>
                                Tambah syarat
                              </Button>
                            </>
                          )}
                        </div>
                      ) : null}
                      <Button type="button" variant="ghost" className="mt-2" onClick={() => update({ ...def, edges: def.edges.filter((_, j) => j !== i) })}>
                        Hapus transisi
                      </Button>
                    </li>
                  );
                })}
              </ul>
              <Button
                type="button"
                className="mt-3"
                onClick={() => update({ ...def, edges: [...def.edges, { id: `e${Date.now().toString(36)}`, from: def.start, to: def.stages[1]?.key ?? def.start, order: 1, trigger: "manual", label: "Transisi baru" }] })}
              >
                Tambah transisi
              </Button>
            </section>
          </fieldset>

          <section className="mt-8 rounded-md border border-line bg-surface p-4">
            <h2 className="mb-2 text-[20px]">Simulasi hasil ujian</h2>
            <p className="mb-3 text-sm text-ink-2">Memakai evaluator yang sama dengan sistem, tanpa mengubah data.</p>
            <div className="flex flex-wrap items-end gap-3">
              <Field label="Nilai">{({ id: fid }) => <Input id={fid} inputMode="decimal" className="w-28" value={sim.score} disabled={sim.manual} onChange={(e) => setSim({ ...sim, score: e.target.value })} />}</Field>
              <label className="flex min-h-11 items-center gap-2">
                <input type="checkbox" className="h-5 w-5" checked={sim.manual} onChange={(e) => setSim({ ...sim, manual: e.target.checked })} />
                Ada esai belum dinilai
              </label>
              <Button onClick={simulate} busy={busy}>
                Jalankan simulasi
              </Button>
            </div>
            <p aria-live="polite" className="mt-3 text-[15px]">
              {sim.result}
            </p>
          </section>

          <section className="mt-8">
            <h2 className="mb-3 text-[20px]">Versi terbit</h2>
            <ul className="divide-y divide-line border-y border-line">
              {q.data.versions.map((v) => (
                <li key={v.id} className="flex flex-wrap items-center justify-between gap-2 py-3">
                  <span>
                    Versi {v.versionNo} · {fmtDateTime(v.publishedAt)} · {v.publishedByName ?? "Sistem"}
                    {v.notes ? ` · ${v.notes}` : ""}
                  </span>
                  {v.active ? (
                    <Badge status="published">Aktif</Badge>
                  ) : can("workflows.publish") ? (
                    <Button size="sm" onClick={() => setRollback({ id: v.id, no: v.versionNo })}>
                      Aktifkan kembali
                    </Button>
                  ) : null}
                </li>
              ))}
            </ul>
          </section>

          {/* key: muat ulang ringkasan setelah versi baru terbit atau versi lama diaktifkan kembali. */}
          <MigrationSection key={q.data.versions.find((v) => v.active)?.id ?? "none"} workflowId={id} canRun={can("workflows.publish")} />

          <Dialog open={publishOpen} onOpenChange={setPublishOpen} title="Terbitkan versi baru?" description="Versi baru berlaku untuk lamaran baru. Lamaran berjalan tetap memakai versinya sampai dipindahkan lewat bagian Pindahkan lamaran berjalan.">
            {err?.code === "workflow_invalid" || err?.code === "stale_version" ? <Alert tone="danger">{err.message}</Alert> : null}
            <Field label="Catatan rilis">{({ id: fid }) => <Input id={fid} value={notes} onChange={(e) => setNotes(e.target.value)} />}</Field>
            <div className="mt-4 flex justify-end gap-2">
              <Button onClick={() => setPublishOpen(false)}>Batal</Button>
              <Button variant="primary" busy={busy} onClick={publish}>
                Terbitkan
              </Button>
            </div>
          </Dialog>
          <Dialog open={!!rollback} onOpenChange={(o) => !o && setRollback(null)} title={`Aktifkan kembali versi ${rollback?.no ?? ""}?`} description="Hanya berlaku untuk lamaran baru.">
            {err && rollback ? <Alert tone="danger">{err.fieldErrors.reason ?? err.message}</Alert> : null}
            <Field label="Alasan" required>{({ id: fid }) => <Input id={fid} value={notes} onChange={(e) => setNotes(e.target.value)} />}</Field>
            <div className="mt-4 flex justify-end gap-2">
              <Button onClick={() => setRollback(null)}>Batal</Button>
              <Button variant="primary" busy={busy} onClick={doRollback}>
                Aktifkan
              </Button>
            </div>
          </Dialog>
        </div>
      ) : null}
    </PageState>
  );
}

type Plan = {
  from: { id: string; versionNo: number };
  to: { id: string; versionNo: number };
  stages: { fromKey: string; fromName: string; count: number; toKey: string; toName: string; problem?: string; warnings: string[] }[];
  total: number;
  ready: boolean;
  targets: { key: string; name: string }[];
};

/** Memindahkan lamaran aktif dari versi lama ke versi aktif: pilih versi, petakan tahap, pratinjau, lalu jalankan. */
function MigrationSection({ workflowId, canRun }: { workflowId: string; canRun: boolean }) {
  const ov = useQuery<{ versions: { id: string; versionNo: number; activeApplications: number; current: boolean }[]; activeVersionId: string | null }>(`/workflows/${workflowId}/migrations`);
  const toast = useToast();
  const [from, setFrom] = useState("");
  const [mapping, setMapping] = useState<Record<string, string>>({});
  const [plan, setPlan] = useState<Plan | null>(null);
  const [reason, setReason] = useState("");
  const [err, setErr] = useState<ApiError | null>(null);
  const [busy, setBusy] = useState(false);
  const older = (ov.data?.versions ?? []).filter((v) => !v.current && v.activeApplications > 0);

  useEffect(() => {
    if (!from) return setPlan(null);
    let alive = true;
    post<Plan>(`/workflows/${workflowId}/migrations/preview`, { fromVersionId: from, mapping })
      .then((p) => {
        if (!alive) return;
        setPlan(p);
        setErr(null);
      })
      .catch((e: ApiError) => {
        if (alive) setErr(e);
      });
    return () => {
      alive = false;
    };
  }, [workflowId, from, mapping]);

  const run = async () => {
    if (!plan || !ov.data?.activeVersionId) return;
    setBusy(true);
    setErr(null);
    try {
      const r = await post<{ moved: number }>(`/workflows/${workflowId}/migrations`, { fromVersionId: from, mapping, expectedActiveVersionId: ov.data.activeVersionId, reason });
      toast("success", `${r.moved} lamaran dipindahkan ke versi ${plan.to.versionNo}.`);
      setFrom("");
      setMapping({});
      setReason("");
      ov.reload();
    } catch (e) {
      setErr(e as ApiError);
    } finally {
      setBusy(false);
    }
  };

  if (ov.loading || older.length === 0) return null;
  return (
    <section className="mt-8" aria-label="Pindahkan lamaran berjalan">
      <h2 className="mb-1 text-[20px]">Pindahkan lamaran berjalan</h2>
      <p className="mb-3 text-sm text-ink-2">Lamaran aktif pada versi lama tetap memakai aturan versinya. Pindahkan ke versi aktif bila aturan baru perlu berlaku juga untuk mereka.</p>
      <label className="mb-3 flex flex-col gap-1 text-sm font-semibold">
        Versi asal
        <Select value={from} onChange={(e) => { setMapping({}); setFrom(e.target.value); }}>
          <option value="">Pilih versi</option>
          {older.map((v) => (
            <option key={v.id} value={v.id}>
              Versi {v.versionNo} · {v.activeApplications} lamaran aktif
            </option>
          ))}
        </Select>
      </label>
      {err ? <Alert tone="danger" title={Object.values(err.fieldErrors)[0] ?? err.message} /> : null}
      {plan ? (
        <>
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-left text-[15px]">
              <caption className="sr-only">Pemetaan tahap versi {plan.from.versionNo} ke versi {plan.to.versionNo}</caption>
              <thead>
                <tr className="border-b border-line text-sm text-ink-2">
                  <th scope="col" className="py-2 pr-4 font-semibold">Tahap di versi {plan.from.versionNo}</th>
                  <th scope="col" className="py-2 pr-4 font-semibold">Lamaran</th>
                  <th scope="col" className="py-2 font-semibold">Pindah ke tahap di versi {plan.to.versionNo}</th>
                </tr>
              </thead>
              <tbody>
                {plan.stages.map((st) => (
                  <tr key={st.fromKey} className="border-b border-line align-top">
                    <td className="py-3 pr-4">{st.fromName}</td>
                    <td className="py-3 pr-4">{st.count}</td>
                    <td className="py-3">
                      <Select aria-label={`Tujuan untuk ${st.fromName}`} value={mapping[st.fromKey] ?? st.toKey} onChange={(e) => setMapping({ ...mapping, [st.fromKey]: e.target.value })}>
                        {!st.toKey ? <option value="">Pilih tahap</option> : null}
                        {plan.targets.map((t) => (
                          <option key={t.key} value={t.key}>
                            {t.name}
                          </option>
                        ))}
                      </Select>
                      {st.problem ? <p className="mt-1 text-sm text-danger">{st.problem}</p> : null}
                      {st.warnings.map((w) => (
                        <p key={w} className="mt-1 text-sm text-warning">
                          {w}
                        </p>
                      ))}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {canRun ? (
            <div className="mt-3 flex flex-col gap-3">
              <Field label="Alasan" required hint="Dicatat di riwayat setiap lamaran dan di audit.">
                {({ id, describedBy }) => <Input id={id} aria-describedby={describedBy} value={reason} onChange={(e) => setReason(e.target.value)} />}
              </Field>
              <div>
                <Button variant="primary" busy={busy} disabled={!plan.ready} onClick={run}>
                  Pindahkan {plan.total} lamaran
                </Button>
              </div>
            </div>
          ) : null}
        </>
      ) : null}
    </section>
  );
}
