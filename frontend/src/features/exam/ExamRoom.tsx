import { useCallback, useEffect, useMemo, useReducer, useRef, useState } from "react";
import { Link, useParams } from "react-router";
import { ApiError, api, get, newIdempotencyKey } from "../../lib/api";
import { fmtDateTime, fmtTime } from "../../lib/format";
import { useSession } from "../../lib/session";
import { Alert, Button, Dialog, cx } from "../../ui";
import { AutosaveQueue, type SaveStatus, type Value } from "./autosave";
import { clockOffset, deleteDraft, fmtRemaining, loadDraft, remainingMs, saveDraft } from "./draftStore";
import { makeSender, type AttemptDTO, type ExamItem, type Receipt } from "./transport";

const letters = "ABCDEFGHIJ";

function statusText(s: SaveStatus): { text: string; tone: string } {
  switch (s.kind) {
    case "saving":
      return { text: "Menyimpan...", tone: "text-ink-2" };
    case "saved":
      return { text: `Tersimpan pukul ${fmtTime(s.at)}`, tone: "text-success" };
    case "dirty":
      return { text: "Perubahan belum tersimpan", tone: "text-ink-2" };
    case "offline":
      return { text: `Menunggu koneksi (${s.pending} jawaban hanya di perangkat)`, tone: "text-warning" };
    case "conflict":
      return { text: "Ada jawaban yang berbeda dengan server", tone: "text-danger" };
    case "final":
      return { text: "Ujian sudah selesai", tone: "text-ink-2" };
    case "error":
      return { text: s.message, tone: "text-danger" };
    default:
      return { text: "Belum ada perubahan", tone: "text-ink-2" };
  }
}

export function ExamRoom() {
  const { id = "" } = useParams();
  const { session } = useSession();
  const userId = session!.user.id;
  const [dto, setDto] = useState<AttemptDTO | null>(null);
  const [loadErr, setLoadErr] = useState<ApiError | null>(null);
  const [receipt, setReceipt] = useState<Receipt | null>(null);
  const [finalMsg, setFinalMsg] = useState<string>("");
  const [, rerender] = useReducer((x: number) => x + 1, 0);
  const [status, setStatus] = useState<SaveStatus>({ kind: "idle" });
  const [idx, setIdx] = useState(0);
  const [flagged, setFlagged] = useState<Set<string>>(new Set());
  const [navOpen, setNavOpen] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [submitErr, setSubmitErr] = useState("");
  const [now, setNow] = useState(Date.now());
  const [announce, setAnnounce] = useState("");
  const offset = useRef(0);
  const revision = useRef(0);
  const submitKey = useRef<string>("");
  const queueRef = useRef<AutosaveQueue | null>(null);
  const announced = useRef(new Set<number>());

  const resume = useCallback(async () => {
    const sent = Date.now();
    const d = await get<AttemptDTO>(`/exam-attempts/${id}`);
    offset.current = clockOffset(d.serverTime, sent, Date.now());
    revision.current = d.revision;
    return d;
  }, [id]);

  // Muat attempt, pulihkan draft lokal, siapkan antrean.
  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        const d = await resume();
        if (!alive) return;
        if (d.status !== "in_progress") {
          setReceipt(d.receipt);
          await deleteDraft(userId, id);
          setDto(d);
          return;
        }
        const draft = await loadDraft(userId, id);
        submitKey.current = draft?.submitKey ?? newIdempotencyKey("submit");
        const fl = new Set(draft?.flagged ?? []);
        setFlagged(fl);
        const expiresAt = Date.parse(d.deadlineAt) + 60 * 60 * 1000;
        const q = new AutosaveQueue({
          send: makeSender(id, (ack) => (revision.current = ack.revision)),
          onStatus: (s) => {
            setStatus(s);
            rerender();
            if (s.kind === "final") setFinalMsg(s.message);
          },
          onPersist: (snap) => void saveDraft({ ...snap, userId, attemptId: id, submitKey: submitKey.current, flagged: [...flRef.current], expiresAt }),
        });
        q.load(d.answers);
        if (draft) q.restore(draft);
        queueRef.current = q;
        setDto(d);
      } catch (e) {
        if (alive) setLoadErr(e as ApiError);
      }
    })();
    return () => {
      alive = false;
      queueRef.current?.dispose();
    };
  }, [id, userId, resume]);

  const flRef = useRef(flagged);
  flRef.current = flagged;

  // Reconnect: resume untuk kalibrasi ulang dan revisi terbaru, lalu kirim ulang batch tertunda (opId sama).
  useEffect(() => {
    const onOnline = async () => {
      try {
        const d = await resume();
        queueRef.current?.load(d.answers);
        if (d.status !== "in_progress") {
          setReceipt(d.receipt);
          return;
        }
      } catch {
        /* masih offline */
      }
      queueRef.current?.online();
    };
    window.addEventListener("online", onOnline);
    return () => window.removeEventListener("online", onOnline);
  }, [resume]);

  // Heartbeat best-effort sekitar 30 detik dengan jitter; tidak memperpanjang sesi.
  useEffect(() => {
    if (!dto || receipt) return;
    let t: ReturnType<typeof setTimeout>;
    const beat = () => {
      t = setTimeout(async () => {
        const sent = Date.now();
        try {
          const r = await api<{ serverTime: string }>("POST", `/exam-attempts/${id}/heartbeat`, {});
          offset.current = clockOffset(r.serverTime, sent, Date.now());
        } catch {
          /* best-effort */
        }
        beat();
      }, 25_000 + Math.random() * 10_000);
    };
    beat();
    return () => clearTimeout(t);
  }, [dto, receipt, id]);

  const left = dto ? remainingMs(dto.deadlineAt, offset.current, now) : 0;
  useEffect(() => {
    if (!dto || receipt) return;
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, [dto, receipt]);

  const doSubmit = useCallback(
    async (timeUp: boolean) => {
      const q = queueRef.current;
      if (!q) return;
      setSubmitting(true);
      setSubmitErr("");
      try {
        await q.flush();
        if (q.hasUnsaved() && !timeUp) {
          setSubmitErr("Masih ada jawaban di perangkat yang belum diterima server. Pastikan koneksi tersambung, lalu kirim lagi.");
          return;
        }
        let r: Receipt;
        try {
          r = await api<Receipt>("POST", `/exam-attempts/${id}/submit`, { expectedRevision: revision.current }, { "Idempotency-Key": submitKey.current });
        } catch (e) {
          const ae = e as ApiError;
          if (ae.code === "stale_revision") {
            const d = await resume();
            q.load(d.answers);
            r = await api<Receipt>("POST", `/exam-attempts/${id}/submit`, { expectedRevision: d.revision }, { "Idempotency-Key": submitKey.current });
          } else if ((ae.code === "deadline_passed" || ae.code === "attempt_finalized") && (ae.details as { receipt?: Receipt })?.receipt) {
            r = (ae.details as { receipt: Receipt }).receipt;
          } else throw e;
        }
        await deleteDraft(userId, id);
        setReceipt(r);
        setConfirmOpen(false);
      } catch (e) {
        setSubmitErr((e as ApiError).message);
      } finally {
        setSubmitting(false);
      }
    },
    [id, resume, userId],
  );

  // Pengumuman sisa waktu hanya pada ambang tertentu, bukan setiap detik. Saat habis: finalisasi.
  useEffect(() => {
    if (!dto || receipt) return;
    for (const m of [10, 5, 1]) {
      if (left <= m * 60_000 && left > 0 && !announced.current.has(m)) {
        announced.current.add(m);
        setAnnounce(`Sisa waktu ${m} menit.`);
      }
    }
    if (left === 0 && !announced.current.has(0)) {
      announced.current.add(0);
      setAnnounce("Waktu habis. Jawaban yang sudah diterima server sedang difinalisasi.");
      void doSubmit(true);
    }
  }, [left, dto, receipt, doSubmit]);

  const items = dto?.items ?? [];
  const item: ExamItem | undefined = items[idx];
  const q = queueRef.current;
  const answered = useMemo(() => {
    const s = new Set<string>();
    for (const it of items) {
      const v = q?.valueOf(it.itemId);
      if (v && ((v.selected?.length ?? 0) > 0 || (v.text ?? "") !== "")) s.add(it.itemId);
    }
    return s;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [items, q, status, idx]);

  const go = (i: number) => {
    void q?.flush(); // flush saat pindah soal; tidak ada write bila tidak ada perubahan
    setIdx(i);
    setNavOpen(false);
    document.getElementById("soal")?.focus();
  };
  const setValue = (v: Value) => {
    q?.set(item!.itemId, v);
    rerender();
  };
  const toggleFlag = (itemId: string) => {
    const n = new Set(flagged);
    if (n.has(itemId)) n.delete(itemId);
    else n.add(itemId);
    setFlagged(n);
    flRef.current = n;
    q?.persistNow(); // tanda ragu hanya disimpan di draft lokal
  };

  if (loadErr) {
    return (
      <div className="mx-auto max-w-2xl p-4">
        <Alert tone={loadErr.status === 0 ? "warning" : "danger"} title={loadErr.status === 0 ? "Tidak ada koneksi" : "Ujian tidak dapat dibuka"}>
          {loadErr.message}
        </Alert>
        <Link to="/akun/ujian" className="mt-4 inline-block font-semibold text-primary underline">
          Kembali ke daftar ujian
        </Link>
      </div>
    );
  }
  if (!dto)
    return (
      <p role="status" className="p-6 text-ink-2">
        Memuat ujian...
      </p>
    );

  if (receipt) return <ReceiptView receipt={receipt} title={dto.title} note={finalMsg} />;

  const st = statusText(status);
  const v = item ? q?.valueOf(item.itemId) ?? null : null;
  const conflictMine = item ? q?.localConflictValue(item.itemId) : undefined;
  const committedHere = item ? q?.isCommitted(item.itemId) : true;
  const lowTime = left < 5 * 60_000;

  const navigator = (
    <nav aria-label="Navigasi soal">
      <ol className="grid grid-cols-5 gap-2">
        {items.map((it, i) => {
          const isAns = answered.has(it.itemId);
          const isFlag = flagged.has(it.itemId);
          const label = `Soal ${it.number}${isAns ? ", dijawab" : ", belum dijawab"}${isFlag ? ", ditandai ragu" : ""}`;
          return (
            <li key={it.itemId}>
              <button
                type="button"
                aria-label={label}
                aria-current={i === idx ? "step" : undefined}
                onClick={() => go(i)}
                className={cx(
                  "relative flex h-11 w-full items-center justify-center rounded-md border text-[15px] font-semibold",
                  i === idx ? "border-primary ring-2 ring-primary" : "border-line-strong",
                  isAns ? "bg-primary-soft text-primary" : "bg-white text-ink",
                )}
              >
                {it.number}
                {isFlag ? <span className="absolute right-1 top-0.5 text-[11px] text-warning" aria-hidden>R</span> : null}
              </button>
            </li>
          );
        })}
      </ol>
      <p className="mt-3 text-[13px] text-ink-2">Biru: dijawab. R: ditandai ragu.</p>
    </nav>
  );

  return (
    <div className="flex min-h-dvh flex-col bg-white">
      <a href="#soal" className="skip-link">
        Lewati ke soal
      </a>
      <header className="sticky top-0 z-20 border-b border-line bg-white">
        <div className="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-2 px-4 py-2">
          <div className="min-w-0">
            <p className="truncate font-semibold">{dto.title}</p>
            <p className={cx("text-[13px]", st.tone)} role="status" aria-live="polite">
              {st.text}
            </p>
          </div>
          <div className="flex items-center gap-3">
            <p className={cx("rounded-md px-3 py-1 text-[15px] font-semibold tabular-nums", lowTime ? "bg-danger-soft text-danger" : "bg-surface text-ink")}>
              <span className="sr-only">Sisa waktu </span>
              {fmtRemaining(left)}
            </p>
            <Button variant="secondary" className="lg:hidden" aria-expanded={navOpen} onClick={() => setNavOpen(true)}>
              Daftar soal
            </Button>
          </div>
        </div>
      </header>
      <p className="sr-only" aria-live="assertive">
        {announce}
      </p>

      <div className="mx-auto flex w-full max-w-5xl flex-1 gap-8 px-4 py-6">
        <main className="min-w-0 flex-1 pb-28">
          {status.kind === "offline" ? (
            <div className="mb-4">
              <Alert tone="warning" title="Koneksi terputus">
                Jawaban yang ditandai "Hanya di perangkat" belum diterima server. Tetap kerjakan; jawaban dikirim otomatis saat tersambung kembali. Waktu ujian tetap berjalan.
              </Alert>
            </div>
          ) : null}
          {status.kind === "conflict" ? (
            <div className="mb-4">
              <Alert tone="danger" title="Jawaban berbeda dengan server">
                Ada soal yang diubah dari tab atau perangkat lain: {status.items.map((c) => `soal ${items.find((x) => x.itemId === c.itemId)?.number ?? "?"}`).join(", ")}. Buka soal tersebut dan pilih versi yang dipakai.
              </Alert>
            </div>
          ) : null}
          {item ? (
            <section aria-labelledby="judul-soal">
              <div id="soal" tabIndex={-1}>
                <h1 id="judul-soal" className="text-[20px]">
                  Soal {item.number} dari {items.length}
                </h1>
                <p className="mt-1 text-[13px]" aria-live="polite">
                  {answered.has(item.itemId) ? (
                    committedHere ? (
                      <span className="text-success">Jawaban tersimpan di server</span>
                    ) : (
                      <span className="text-warning">Jawaban hanya di perangkat, belum diterima server</span>
                    )
                  ) : (
                    <span className="text-ink-2">Belum dijawab</span>
                  )}
                </p>
                <p className="mt-4 whitespace-pre-line text-[16px] leading-relaxed">{item.stem}</p>
              </div>

              {conflictMine !== undefined && status.kind === "conflict" ? (
                <div className="mt-4 rounded-md border border-danger p-4">
                  <p className="font-semibold">Pilih jawaban yang dipakai untuk soal ini</p>
                  <p className="mt-1 text-sm">Di perangkat ini: {describe(item, conflictMine)}</p>
                  <p className="text-sm">Di server: {describe(item, q?.valueOf(item.itemId) ?? null)}</p>
                  <div className="mt-3 flex flex-wrap gap-2">
                    <Button variant="primary" onClick={() => { q?.resolve(item.itemId, "mine"); rerender(); }}>
                      Pakai jawaban di perangkat ini
                    </Button>
                    <Button onClick={() => { q?.resolve(item.itemId, "server"); rerender(); }}>Pakai jawaban di server</Button>
                  </div>
                </div>
              ) : null}

              <div className="mt-5">
                {item.kind === "essay" ? (
                  <div>
                    <label htmlFor="esai" className="text-sm font-semibold">
                      Jawaban esai
                    </label>
                    <textarea
                      id="esai"
                      className="field-input mt-1 min-h-48"
                      value={v?.text ?? ""}
                      maxLength={item.maxChars}
                      onChange={(e) => setValue(e.target.value ? { text: e.target.value } : null)}
                    />
                    <p className="mt-1 text-[13px] text-ink-2">
                      {(v?.text ?? "").length} / {item.maxChars} karakter
                    </p>
                  </div>
                ) : (
                  <fieldset>
                    <legend className="sr-only">{item.kind === "single" ? "Pilih satu jawaban" : "Pilih semua jawaban yang benar"}</legend>
                    <p className="mb-2 text-sm text-ink-2">{item.kind === "single" ? "Pilih satu jawaban." : "Pilih semua jawaban yang benar."}</p>
                    <ul className="flex flex-col gap-2">
                      {item.options.map((o, i) => {
                        const sel = v?.selected ?? [];
                        const checked = sel.includes(o.id);
                        return (
                          <li key={o.id}>
                            <label
                              className={cx(
                                "flex min-h-11 cursor-pointer items-start gap-3 rounded-md border px-3 py-2.5",
                                checked ? "border-primary bg-primary-soft" : "border-line-strong",
                              )}
                            >
                              <input
                                type={item.kind === "single" ? "radio" : "checkbox"}
                                name={`opsi-${item.itemId}`}
                                className="mt-1 h-5 w-5 shrink-0"
                                checked={checked}
                                onChange={() => {
                                  if (item.kind === "single") setValue({ selected: [o.id] });
                                  else {
                                    const next = checked ? sel.filter((s) => s !== o.id) : [...sel, o.id];
                                    setValue(next.length ? { selected: next } : null);
                                  }
                                }}
                              />
                              <span>
                                <span className="font-semibold">{letters[i]}.</span> {o.text}
                              </span>
                            </label>
                          </li>
                        );
                      })}
                    </ul>
                    {item.kind === "single" && (v?.selected?.length ?? 0) > 0 ? (
                      <Button variant="ghost" size="sm" className="mt-2" onClick={() => setValue(null)}>
                        Kosongkan jawaban
                      </Button>
                    ) : null}
                  </fieldset>
                )}
              </div>
              <label className="mt-5 flex min-h-11 items-center gap-3">
                <input type="checkbox" className="h-5 w-5" checked={flagged.has(item.itemId)} onChange={() => toggleFlag(item.itemId)} />
                Tandai ragu-ragu
              </label>
            </section>
          ) : null}
        </main>
        <aside className="hidden w-64 shrink-0 lg:block">
          <div className="sticky top-24">
            {navigator}
            <Button variant="primary" className="mt-6 w-full" onClick={() => setConfirmOpen(true)}>
              Selesai dan kirim
            </Button>
          </div>
        </aside>
      </div>

      {/* Action bar bawah: tidak menutup isi (main diberi padding bawah). */}
      <div className="fixed inset-x-0 bottom-0 z-20 border-t border-line bg-white px-4 py-2 pb-[max(8px,env(safe-area-inset-bottom))]">
        <div className="mx-auto flex max-w-5xl items-center justify-between gap-2">
          <Button disabled={idx === 0} onClick={() => go(idx - 1)}>
            Sebelumnya
          </Button>
          <Button variant="primary" className="lg:hidden" onClick={() => setConfirmOpen(true)}>
            Kirim
          </Button>
          <Button disabled={idx >= items.length - 1} onClick={() => go(idx + 1)}>
            Berikutnya
          </Button>
        </div>
      </div>

      <Dialog open={navOpen} onOpenChange={setNavOpen} title="Daftar soal">
        {navigator}
      </Dialog>
      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen} title="Kirim jawaban sekarang?" description="Setelah dikirim, jawaban tidak dapat diubah.">
        <ul className="list-disc pl-5 text-[15px]">
          <li>
            Dijawab: {answered.size} dari {items.length}
          </li>
          {items.length - answered.size > 0 ? <li>Belum dijawab: {items.length - answered.size} soal</li> : null}
          {flagged.size > 0 ? <li>Ditandai ragu: {flagged.size} soal</li> : null}
        </ul>
        {submitErr ? (
          <div className="mt-3">
            <Alert tone="danger">{submitErr}</Alert>
          </div>
        ) : null}
        <div className="mt-4 flex justify-end gap-2">
          <Button onClick={() => setConfirmOpen(false)}>Kembali mengerjakan</Button>
          <Button variant="primary" busy={submitting} onClick={() => doSubmit(false)}>
            Kirim jawaban
          </Button>
        </div>
      </Dialog>
    </div>
  );
}

function describe(item: ExamItem, v: Value | undefined): string {
  if (!v) return "kosong";
  if (v.text !== undefined) return `"${v.text.slice(0, 80)}${v.text.length > 80 ? "..." : ""}"`;
  const labels = (v.selected ?? []).map((s) => letters[item.options.findIndex((o) => o.id === s)] ?? "?");
  return labels.length ? `pilihan ${labels.join(", ")}` : "kosong";
}

function ReceiptView({ receipt, title, note }: { receipt: Receipt; title: string; note: string }) {
  return (
    <div className="mx-auto max-w-2xl px-4 py-10">
      <h1 className="text-[26px]">Bukti penerimaan jawaban</h1>
      <p className="mt-1 text-ink-2">{title}</p>
      <div className="mt-6">
        <Alert tone={receipt.reason === "submitted" ? "success" : "warning"} title={receipt.reason === "submitted" ? "Jawaban Anda sudah diterima" : "Waktu ujian habis"}>
          {receipt.reason === "expired"
            ? "Ujian difinalisasi otomatis dengan jawaban terakhir yang sudah diterima server. Jawaban yang hanya tersimpan di perangkat setelah koneksi terputus tidak ikut dinilai. Bila Anda keberatan, ajukan melalui tim rekrutmen dengan menyebut kode bukti ini; keberatan dicatat dan ditinjau."
            : note || "Simpan kode bukti ini."}
        </Alert>
      </div>
      <dl className="mt-6 grid grid-cols-[max-content_1fr] gap-x-6 gap-y-2 text-[15px]">
        <dt className="text-ink-2">Kode bukti</dt>
        <dd className="font-semibold tracking-wide">{receipt.code}</dd>
        <dt className="text-ink-2">Waktu diterima</dt>
        <dd>{fmtDateTime(receipt.submittedAt)}</dd>
        <dt className="text-ink-2">Soal dijawab</dt>
        <dd>
          {receipt.answered} dari {receipt.total}
        </dd>
        <dt className="text-ink-2">Sidik jawaban</dt>
        <dd>{receipt.answerDigest}</dd>
      </dl>
      <Link to="/akun/ujian" className="mt-8 inline-block font-semibold text-primary underline">
        Kembali ke daftar ujian
      </Link>
    </div>
  );
}
