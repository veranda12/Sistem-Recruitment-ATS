import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router";
import { ApiError, del, get, patch, post } from "../../../lib/api";
import { Alert, Button, cx, Dialog, Input, PageHeader, PageState, Select, Textarea } from "../../../ui";
import {
  changeKind,
  changeLabel,
  currentValue,
  displayValue,
  effective,
  fieldLabel,
  groups,
  humanLocator,
  inProgress,
  isoOf,
  needsCheck,
  profileDiff,
  sectionTitle,
  similarEntry,
  statusText,
  type Evidence,
  type Group,
  type PeriodValue,
  type Profile,
  type Run,
  type Suggestion,
} from "./model";

type EntryChoice = { mode: "add" | "skip" | "replace"; targetIndex?: number };

export default function CvReviewPage() {
  const { id = "" } = useParams();
  const [run, setRun] = useState<Run | null>(null);
  const [profile, setProfile] = useState<Profile | null>(null);
  const [loadErr, setLoadErr] = useState<ApiError | null>(null);
  const [actionErr, setActionErr] = useState<ApiError | null>(null);
  const [focusId, setFocusId] = useState<string | null>(null);
  const [sourceOpen, setSourceOpen] = useState(false);
  const [entryChoice, setEntryChoice] = useState<Record<string, EntryChoice>>({});
  const [conflict, setConflict] = useState<{ before: Profile; after: Profile } | null>(null);
  const [busy, setBusy] = useState(false);
  const errRef = useRef<HTMLDivElement>(null);

  const load = useCallback(async () => {
    try {
      const [r, p] = await Promise.all([get<Run>(`/me/cv-parses/${id}`), profile ? Promise.resolve(profile) : get<Profile>("/me/profile")]);
      setRun(r);
      setProfile(p);
      setLoadErr(null);
    } catch (e) {
      setLoadErr(e as ApiError);
    }
  }, [id, profile]);

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  // Progres diperbarui berkala selama masih diproses; refresh halaman melanjutkan dari server.
  useEffect(() => {
    if (!run || !inProgress(run)) return;
    const t = setTimeout(load, 1500);
    return () => clearTimeout(t);
  }, [run, load]);

  useEffect(() => {
    if (actionErr) errRef.current?.focus();
  }, [actionErr]);

  const gs = useMemo(() => (run ? groups(run) : []), [run]);
  const doubtful = useMemo(() => (run ? run.suggestions.filter(needsCheck) : []), [run]);
  const evidenceById = useMemo(() => new Map((run?.evidence ?? []).map((e) => [e.id, e])), [run]);
  const focus = run?.suggestions.find((s) => s.id === focusId) ?? null;

  // Pilihan default per entri: lewati bila mirip entri profil (hindari duplikat), selain itu tambahkan.
  useEffect(() => {
    if (!run || !profile) return;
    setEntryChoice((cur) => {
      const next = { ...cur };
      for (const g of gs) {
        if ((g.section === "experience" || g.section === "education") && !next[g.key]) {
          const sim = similarEntry(profile, g);
          next[g.key] = sim ? { mode: "skip", targetIndex: sim.index } : { mode: "add" };
        }
      }
      return next;
    });
  }, [gs, profile, run]);

  const decide = async (decisions: { suggestionId: string; action: string; value?: unknown }[]) => {
    if (!run) return false;
    setActionErr(null);
    try {
      const r = await patch<Run>(`/me/cv-parses/${run.id}/review`, { reviewVersion: run.reviewVersion, decisions });
      setRun(r);
      return true;
    } catch (e) {
      const ae = e as ApiError;
      setActionErr(ae);
      if (ae.code === "stale_review") load();
      return false;
    }
  };

  const apply = async (prof: Profile) => {
    if (!run) return;
    setBusy(true);
    setActionErr(null);
    const entries: Record<string, { mode: string; targetId?: string }> = {};
    for (const [k, c] of Object.entries(entryChoice)) {
      const list: { id?: string }[] = k.startsWith("experience") ? prof.experiences : prof.education;
      const target = c.targetIndex !== undefined ? list[c.targetIndex]?.id : undefined;
      entries[k] = c.mode === "replace" && target ? { mode: "replace", targetId: target } : { mode: c.mode === "replace" ? "add" : c.mode };
    }
    try {
      await post(`/me/cv-parses/${run.id}/apply`, { reviewVersion: run.reviewVersion, profileVersion: prof.version, entries });
      setConflict(null);
      const [r, p] = await Promise.all([get<Run>(`/me/cv-parses/${run.id}`), get<Profile>("/me/profile")]);
      setRun(r);
      setProfile(p);
      window.scrollTo(0, 0);
    } catch (e) {
      const ae = e as ApiError;
      if (ae.code === "profile_changed") {
        const after = await get<Profile>("/me/profile");
        setConflict({ before: prof, after });
      } else {
        setActionErr(ae);
      }
    } finally {
      setBusy(false);
    }
  };

  if (loadErr) return <PageState loading={false} error={loadErr} onRetry={load}>{null}</PageState>;
  if (!run || !profile) return <PageState loading error={null} onRetry={load}>{null}</PageState>;

  const decided = run.suggestions.filter((s) => s.status !== "pending").length;
  const accepted = run.suggestions.filter((s) => s.status === "accepted" || s.status === "edited").length;
  const reviewable = run.status === "ready_for_review" || run.status === "partial";
  const failedPages = run.pages.filter((p) => p.status === "failed");

  const showSource = (s: Suggestion) => {
    setFocusId(s.id);
    if (window.matchMedia("(max-width: 1023px)").matches) setSourceOpen(true);
  };

  return (
    <div>
      <Link to="/akun/profil" className="text-sm text-primary hover:underline">
        Kembali ke profil
      </Link>
      <PageHeader title="Periksa data dari CV" subtitle={<span aria-live="polite">{statusText(run)}</span>} />

      <div ref={errRef} tabIndex={-1} className="outline-none">
        {actionErr ? (
          <div className="mb-4">
            <Alert tone="danger" title="Pilihan belum tersimpan">
              {actionErr.code === "validation_failed" ? Object.values(actionErr.fieldErrors).join(" ") : actionErr.message}
            </Alert>
          </div>
        ) : null}
      </div>

      {inProgress(run) ? <Progress run={run} /> : null}

      {run.status === "failed" ? (
        <Alert
          tone="danger"
          title="CV tidak dapat dibaca"
          action={
            <span className="flex flex-wrap gap-2">
              {run.failure?.transient ? (
                <Button onClick={() => post(`/me/documents/${run.documentId}/parse?retry=1`).then(load).catch((e) => setActionErr(e as ApiError))}>Coba lagi</Button>
              ) : null}
              <Link to="/akun/profil" className="inline-flex min-h-11 items-center font-semibold text-primary underline">
                Unggah ulang atau isi manual
              </Link>
            </span>
          }
        >
          {run.failure?.message}
        </Alert>
      ) : null}

      {run.status === "applied" ? <Applied run={run} /> : null}
      {run.status === "cancelled" ? <Alert tone="info" title="Pembacaan dibatalkan">Dokumen sumber sudah dihapus. Unggah CV lain atau isi profil secara manual.</Alert> : null}

      {reviewable ? (
        <>
          {failedPages.length > 0 ? (
            <div className="mb-4">
              <Alert tone="warning" title="Sebagian halaman tidak terbaca">
                {failedPages.map((p) => `Halaman ${p.no}: ${p.message ?? "tidak dapat dibaca"}`).join(" ")} Data dari halaman lain tetap dapat diperiksa di bawah. Lengkapi sisanya secara manual.
              </Alert>
            </div>
          ) : null}
          {conflict ? (
            <ConflictPanel conflict={conflict} busy={busy} onApply={() => apply(conflict.after)} />
          ) : null}

          {doubtful.length > 0 ? (
            <nav aria-label="Data yang perlu diperiksa" className="mb-6 border-l-4 border-warning bg-warning-soft px-4 py-3">
              <p className="font-semibold">{doubtful.length} data perlu diperiksa lebih dulu</p>
              <ul className="mt-1 list-disc pl-5 text-sm">
                {doubtful.slice(0, 8).map((s) => (
                  <li key={s.id}>
                    <a href={`#s-${s.id}`} className="text-primary underline">
                      {sectionTitle[s.section]}: {fieldLabel(s)}
                    </a>{" "}
                    <span className="text-ink-2">({[...s.readDoubts, ...s.assocDoubts].join("; ") || "nilai tidak ditemukan"})</span>
                  </li>
                ))}
              </ul>
            </nav>
          ) : null}

          <div className="grid gap-8 lg:grid-cols-[minmax(0,1fr)_minmax(0,440px)]">
            <div className="flex flex-col gap-8">
              <div className="flex flex-wrap items-center gap-2 text-sm">
                <Button
                  size="sm"
                  onClick={() =>
                    decide(run.suggestions.filter((s) => s.status === "pending" && !needsCheck(s)).map((s) => ({ suggestionId: s.id, action: "accept" })))
                  }
                  disabled={!run.suggestions.some((s) => s.status === "pending" && !needsCheck(s))}
                >
                  Terima semua yang tidak perlu diperiksa
                </Button>
                <span className="text-ink-2">
                  {decided} dari {run.suggestions.length} sudah diputuskan
                </span>
              </div>
              {gs.map((g) => (
                <GroupView
                  key={g.key}
                  g={g}
                  profile={profile}
                  focusId={focusId}
                  choice={entryChoice[g.key]}
                  onChoice={(c) => setEntryChoice((x) => ({ ...x, [g.key]: c }))}
                  onDecide={decide}
                  onSource={showSource}
                />
              ))}
            </div>
            <aside aria-label="Sumber di CV" className="hidden lg:block">
              <div className="sticky top-4">
                <SourceViewer run={run} s={focus} evidenceById={evidenceById} />
              </div>
            </aside>
          </div>

          <div className="sticky bottom-0 mt-8 flex flex-wrap items-center justify-between gap-3 border-t border-line bg-white py-3">
            <p className="text-sm text-ink-2">
              {accepted} usulan akan disimpan. Usulan yang belum diputuskan atau ditolak tidak mengubah profil.
            </p>
            <span className="flex gap-2">
              <Button
                variant="ghost"
                onClick={async () => {
                  await del(`/me/cv-parses/${run.id}`).catch((e) => setActionErr(e as ApiError));
                  load();
                }}
              >
                Abaikan hasil
              </Button>
              <Button variant="primary" busy={busy} disabled={accepted === 0} onClick={() => apply(profile)}>
                Simpan pilihan ke profil
              </Button>
            </span>
          </div>

          <Dialog open={sourceOpen} onOpenChange={setSourceOpen} title="Sumber di CV">
            <SourceViewer run={run} s={focus} evidenceById={evidenceById} />
            <div className="mt-4 flex justify-end">
              <Button onClick={() => setSourceOpen(false)}>Tutup</Button>
            </div>
          </Dialog>
        </>
      ) : null}
    </div>
  );
}

function Progress({ run }: { run: Run }) {
  const pct = run.pagesTotal ? Math.round((run.pagesDone / run.pagesTotal) * 100) : undefined;
  return (
    <div className="mb-6 flex flex-col gap-2">
      <div
        role="progressbar"
        aria-label="Progres pembacaan CV"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={pct}
        aria-valuetext={statusText(run)}
        className="h-2 w-full overflow-hidden rounded-sm bg-surface"
      >
        <div className={cx("h-full bg-primary transition-all", pct === undefined && "w-1/3 animate-pulse")} style={pct !== undefined ? { width: `${Math.max(pct, 5)}%` } : undefined} />
      </div>
      <p className="text-sm text-ink-2">Anda boleh menutup halaman ini; pembacaan tetap berjalan dan dapat dilanjutkan dari halaman profil.</p>
    </div>
  );
}

function Applied({ run }: { run: Run }) {
  const r = run.applyResult;
  const added = r ? Object.entries(r.added).map(([k, n]) => `${n} ${sectionTitle[k]?.toLowerCase() ?? k}`) : [];
  return (
    <Alert tone="success" title="Profil diperbarui">
      {r && r.updatedFields.length > 0 ? `Kolom diperbarui: ${r.updatedFields.length}. ` : null}
      {added.length > 0 ? `Ditambahkan: ${added.join(", ")}. ` : null}
      <Link to="/akun/profil" className="font-semibold text-primary underline">
        Lihat profil
      </Link>
    </Alert>
  );
}

function ConflictPanel({ conflict, busy, onApply }: { conflict: { before: Profile; after: Profile }; busy: boolean; onApply: () => void }) {
  const diff = profileDiff(conflict.before, conflict.after);
  return (
    <div className="mb-6">
      <Alert
        tone="warning"
        title="Profil berubah sejak Anda mulai memeriksa"
        action={
          <Button variant="primary" busy={busy} onClick={onApply}>
            Terapkan pilihan ke profil terbaru
          </Button>
        }
      >
        <p>Pilihan Anda tetap tersimpan. Perubahan di profil:</p>
        <ul className="list-disc pl-5">
          {diff.length ? diff.map((d) => <li key={d}>{d}</li>) : <li>Versi profil berubah.</li>}
        </ul>
      </Alert>
    </div>
  );
}

function GroupView({
  g,
  profile,
  focusId,
  choice,
  onChoice,
  onDecide,
  onSource,
}: {
  g: Group;
  profile: Profile;
  focusId: string | null;
  choice?: EntryChoice;
  onChoice: (c: EntryChoice) => void;
  onDecide: (d: { suggestionId: string; action: string; value?: unknown }[]) => Promise<boolean>;
  onSource: (s: Suggestion) => void;
}) {
  const entry = g.section === "experience" || g.section === "education";
  const sim = entry ? similarEntry(profile, g) : null;
  return (
    <section aria-labelledby={`g-${g.key}`} className="flex flex-col gap-3">
      <div className="flex flex-wrap items-baseline justify-between gap-2 border-b border-line pb-2">
        <h2 id={`g-${g.key}`} className="text-[18px]">
          {g.title}
          {entry ? <span className="ml-2 text-sm font-normal text-ink-2">{sectionTitle[g.section]}</span> : null}
        </h2>
        <Button size="sm" variant="ghost" onClick={() => onDecide(g.items.filter((s) => s.raw !== "").map((s) => ({ suggestionId: s.id, action: "accept" })))}>
          Terima semua di bagian ini
        </Button>
      </div>
      {entry && choice ? (
        <div className="grid gap-1 sm:max-w-md">
          <label htmlFor={`m-${g.key}`} className="text-sm font-semibold">
            Simpan entri ini sebagai
          </label>
          {sim ? <p className="text-[13px] text-ink-2">Mirip entri di profil: {sim.label}.</p> : null}
          <Select id={`m-${g.key}`} value={choice.mode} onChange={(e) => onChoice({ ...choice, mode: e.target.value as EntryChoice["mode"] })}>
            <option value="add">Tambahkan sebagai entri baru</option>
            {sim ? <option value="replace">Ganti entri lama dengan data dari CV</option> : null}
            <option value="skip">{sim ? "Pertahankan entri lama, lewati yang ini" : "Lewati entri ini"}</option>
          </Select>
        </div>
      ) : null}
      <ul className="flex flex-col divide-y divide-line">
        {g.items.map((s) => (
          <SuggestionRow key={s.id} s={s} profile={profile} focused={focusId === s.id} onDecide={onDecide} onSource={onSource} />
        ))}
      </ul>
    </section>
  );
}

const decisionLabel: Record<string, string> = { accepted: "Diterima", edited: "Diubah", rejected: "Ditolak", pending: "Belum diputuskan" };

function SuggestionRow({
  s,
  profile,
  focused,
  onDecide,
  onSource,
}: {
  s: Suggestion;
  profile: Profile;
  focused: boolean;
  onDecide: (d: { suggestionId: string; action: string; value?: unknown }[]) => Promise<boolean>;
  onSource: (s: Suggestion) => void;
}) {
  const [editing, setEditing] = useState(false);
  const cur = currentValue(profile, s);
  const value = displayValue(s);
  const doubts = [...s.readDoubts, ...s.assocDoubts];
  const kind = changeKind(profile, s);
  const label = fieldLabel(s);
  return (
    <li id={`s-${s.id}`} className={cx("flex flex-col gap-2 py-3 scroll-mt-4", focused && "bg-primary-soft/40")}>
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0 flex-1">
          <p className="text-[13px] font-semibold text-ink-2">{label}</p>
          <p className={cx("whitespace-pre-line break-words text-[15px]", s.status === "rejected" && "text-ink-2 line-through")}>{value || <em className="text-ink-2">Tidak ditemukan di CV</em>}</p>
          {s.section === "contact" && s.field === "email" ? (
            <p className="text-[13px] text-ink-2">Email akun Anda ({profile.email}) tidak berubah. Usulan ini menjadi email kontak.</p>
          ) : cur !== null ? (
            <p className="text-[13px] text-ink-2">
              Profil saat ini: {cur || "kosong"} · {changeLabel[kind]}
            </p>
          ) : null}
          {doubts.length > 0 ? (
            <p className="mt-1 text-[13px] font-semibold text-warning">
              Perlu diperiksa: {doubts.join("; ")}
            </p>
          ) : null}
        </div>
        <span className="text-[13px] font-semibold text-ink-2" aria-live="polite">
          {decisionLabel[s.status]}
        </span>
      </div>
      {editing ? (
        <EditForm
          s={s}
          onCancel={() => setEditing(false)}
          onSave={async (v) => {
            if (await onDecide([{ suggestionId: s.id, action: "edit", value: v }])) setEditing(false);
          }}
        />
      ) : (
        <div className="flex flex-wrap gap-2" role="group" aria-label={`Keputusan untuk ${label}`}>
          <Button size="sm" variant={s.status === "accepted" ? "primary" : "secondary"} aria-pressed={s.status === "accepted"} disabled={s.raw === "" && s.status !== "edited"} onClick={() => onDecide([{ suggestionId: s.id, action: "accept" }])}>
            Terima
          </Button>
          <Button size="sm" variant={s.status === "edited" ? "primary" : "secondary"} aria-pressed={s.status === "edited"} onClick={() => setEditing(true)}>
            Ubah
          </Button>
          <Button size="sm" variant={s.status === "rejected" ? "primary" : "secondary"} aria-pressed={s.status === "rejected"} onClick={() => onDecide([{ suggestionId: s.id, action: "reject" }])}>
            Tolak
          </Button>
          {s.evidenceIds.length > 0 ? (
            <Button size="sm" variant="ghost" onClick={() => onSource(s)} aria-label={`Lihat sumber ${label}`}>
              Lihat sumber
            </Button>
          ) : null}
        </div>
      )}
    </li>
  );
}

function EditForm({ s, onSave, onCancel }: { s: Suggestion; onSave: (v: unknown) => void; onCancel: () => void }) {
  const v = effective(s);
  const label = fieldLabel(s);
  const [text, setText] = useState(() => (s.field === "phone" ? (v as { e164?: string })?.e164 ?? s.raw : s.field === "url" ? (v as { target?: string })?.target ?? s.raw : typeof v === "string" || typeof v === "number" ? String(v) : s.raw));
  const per = (s.field === "period" ? v : null) as PeriodValue | null;
  const [start, setStart] = useState(isoOf(per?.start));
  const [end, setEnd] = useState(isoOf(per?.end));
  const [current, setCurrent] = useState(per?.current ?? false);
  const lang = (s.field === "language" ? v : null) as { name: string; level: string } | null;
  const [lname, setLname] = useState(lang?.name ?? "");
  const [llevel, setLlevel] = useState(lang?.level ?? "");
  const firstRef = useRef<HTMLInputElement & HTMLTextAreaElement & HTMLSelectElement>(null);
  useEffect(() => firstRef.current?.focus(), []);

  const submit = () => {
    switch (s.field) {
      case "period":
        return onSave({ start, end: current ? "" : end, current });
      case "gpa":
        return onSave(Number(text.replace(",", ".")));
      case "language":
        return onSave({ name: lname, level: llevel });
    }
    onSave(text);
  };

  return (
    <form
      className="grid gap-2 rounded-md border border-line p-3"
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      {s.field === "period" ? (
        <div className="grid gap-2 sm:grid-cols-3">
          <label className="text-sm">
            Mulai (tahun atau tahun-bulan)
            <Input ref={firstRef} inputMode="numeric" placeholder="2022-02" value={start} onChange={(e) => setStart(e.target.value)} />
          </label>
          <label className="text-sm">
            Selesai
            <Input inputMode="numeric" placeholder="2024" value={end} disabled={current} onChange={(e) => setEnd(e.target.value)} />
          </label>
          <label className="flex items-center gap-2 self-end pb-3 text-sm">
            <input type="checkbox" className="h-5 w-5" checked={current} onChange={(e) => setCurrent(e.target.checked)} />
            Masih berlangsung
          </label>
        </div>
      ) : s.field === "level" ? (
        <label className="text-sm">
          {label}
          <Select value={text} onChange={(e) => setText(e.target.value)}>
            {["SMA", "SMK", "D3", "D4", "S1", "S2", "S3", "Lainnya"].map((l) => (
              <option key={l}>{l}</option>
            ))}
          </Select>
        </label>
      ) : s.field === "language" ? (
        <div className="grid gap-2 sm:grid-cols-2">
          <label className="text-sm">
            Bahasa
            <Input ref={firstRef} value={lname} onChange={(e) => setLname(e.target.value)} />
          </label>
          <label className="text-sm">
            Tingkat
            <Input value={llevel} onChange={(e) => setLlevel(e.target.value)} />
          </label>
        </div>
      ) : s.field === "summary" || s.field === "description" ? (
        <label className="text-sm">
          {label}
          <Textarea value={text} onChange={(e) => setText(e.target.value)} />
        </label>
      ) : (
        <label className="text-sm">
          {label}
          <Input
            ref={firstRef}
            value={text}
            type={s.field === "email" ? "email" : s.field === "url" ? "url" : s.field === "phone" ? "tel" : "text"}
            inputMode={s.field === "phone" ? "tel" : s.field === "gpa" ? "decimal" : undefined}
            autoComplete="off"
            onChange={(e) => setText(e.target.value)}
          />
        </label>
      )}
      <div className="flex gap-2">
        <Button size="sm" type="submit" variant="primary">
          Simpan perubahan
        </Button>
        <Button size="sm" type="button" onClick={onCancel}>
          Batal
        </Button>
      </div>
    </form>
  );
}

/** Preview sumber: gambar halaman dengan sorotan kotak bukti, atau kutipan + lokasi untuk Word. Bukan HTML dokumen aktif. */
function SourceViewer({ run, s, evidenceById }: { run: Run; s: Suggestion | null; evidenceById: Map<string, Evidence> }) {
  if (!s) {
    return <p className="rounded-md border border-dashed border-line p-6 text-sm text-ink-2">Pilih "Lihat sumber" pada sebuah usulan untuk melihat bagian CV yang mendukungnya.</p>;
  }
  const evs = s.evidenceIds.map((id) => evidenceById.get(id)).filter((e): e is Evidence => !!e);
  if (evs.length === 0) return <p className="text-sm text-ink-2">Usulan ini tidak memiliki bukti.</p>;
  const pageNo = evs[0]!.page;
  const page = run.pages.find((p) => p.no === pageNo);
  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm font-semibold">
        {fieldLabel(s)}: {displayValue(s)}
      </p>
      {evs.map((e) => (
        <blockquote key={e.id} className="border-l-4 border-primary bg-surface px-3 py-2 text-sm">
          <p className="whitespace-pre-line">{e.snippet}</p>
          <footer className="mt-1 text-[12px] text-ink-2">
            {e.source === "docx" ? humanLocator(e.locator) : `Halaman ${e.page}${e.source === "ocr" ? ", dibaca dari gambar" : ""}`}
          </footer>
        </blockquote>
      ))}
      {page?.hasImage && page.scale ? <PageCrop run={run} pageNo={pageNo} scale={page.scale} evs={evs.filter((e) => e.page === pageNo && e.bbox)} /> : null}
    </div>
  );
}


/** Potongan CV yang diperbesar di sekitar bukti (dengan konteks), ditambah opsi halaman penuh. */
function PageCrop({ run, pageNo, scale, evs }: { run: Run; pageNo: number; scale: number; evs: Evidence[] }) {
  const [natural, setNatural] = useState<{ w: number; h: number } | null>(null);
  const [full, setFull] = useState(false);
  const src = `/api/v1/me/cv-parses/${run.id}/pages/${pageNo}/image`;
  const boxes = evs.map((e) => e.bbox!.map((v) => v * scale) as [number, number, number, number]);
  const region = (() => {
    if (!natural || boxes.length === 0) return null;
    const x0 = Math.max(0, Math.min(...boxes.map((b) => b[0])) - 24);
    const x1 = Math.min(natural.w, Math.max(...boxes.map((b) => b[2])) + 24);
    const y0 = Math.max(0, Math.min(...boxes.map((b) => b[1])) - 40);
    const y1 = Math.min(natural.h, Math.max(...boxes.map((b) => b[3])) + 40);
    // lebar minimal 45% halaman agar teks sekitar ikut terbaca
    const minW = natural.w * 0.45;
    const cx0 = x1 - x0 < minW ? Math.max(0, Math.min(natural.w - minW, (x0 + x1) / 2 - minW / 2)) : x0;
    const cx1 = x1 - x0 < minW ? cx0 + minW : x1;
    return { x0: cx0, x1: cx1, y0, y1 };
  })();
  const mark = (b: [number, number, number, number], r: { x0: number; x1: number; y0: number; y1: number }, i: number) => (
    <span
      key={i}
      aria-hidden
      className="absolute rounded-sm border-2 border-primary bg-primary/15"
      style={{
        left: `${((b[0] - 3 - r.x0) / (r.x1 - r.x0)) * 100}%`,
        top: `${((b[1] - 3 - r.y0) / (r.y1 - r.y0)) * 100}%`,
        width: `${((b[2] - b[0] + 6) / (r.x1 - r.x0)) * 100}%`,
        height: `${((b[3] - b[1] + 6) / (r.y1 - r.y0)) * 100}%`,
      }}
    />
  );
  return (
    <div className="flex flex-col gap-2">
      {/* gambar tersembunyi hanya untuk mengetahui ukuran asli preview */}
      {!natural ? <img src={src} alt="" className="hidden" onLoad={(ev) => setNatural({ w: ev.currentTarget.naturalWidth, h: ev.currentTarget.naturalHeight })} /> : null}
      {region ? (
        <div
          className="relative w-full overflow-hidden rounded-md border border-line bg-white"
          style={{ aspectRatio: `${region.x1 - region.x0} / ${region.y1 - region.y0}` }}
        >
          <img
            src={src}
            alt={`Potongan halaman ${pageNo} CV di sekitar bagian sumber`}
            className="absolute max-w-none"
            style={{
              width: `${(natural!.w / (region.x1 - region.x0)) * 100}%`,
              left: `${(-region.x0 / (region.x1 - region.x0)) * 100}%`,
              top: `${(-region.y0 / (region.y1 - region.y0)) * 100}%`,
            }}
          />
          {boxes.map((b, i) => mark(b, region, i))}
        </div>
      ) : null}
      <div>
        <Button size="sm" variant="ghost" aria-expanded={full} onClick={() => setFull((f) => !f)}>
          {full ? "Sembunyikan halaman penuh" : "Lihat halaman penuh"}
        </Button>
      </div>
      {full && natural ? (
        <div className="relative overflow-hidden rounded-md border border-line">
          <img src={src} alt={`Halaman ${pageNo} CV dengan bagian sumber ditandai`} className="block w-full" />
          {boxes.map((b, i) => mark(b, { x0: 0, x1: natural.w, y0: 0, y1: natural.h }, i))}
        </div>
      ) : null}
    </div>
  );
}
