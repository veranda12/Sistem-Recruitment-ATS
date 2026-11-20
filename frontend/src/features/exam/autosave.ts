// Antrean autosave jawaban ujian. Modul murni (tanpa React) agar bisa diuji dengan timer palsu.
//
// Jaminan:
// - Hanya item yang berubah dikirim; navigasi tanpa perubahan tidak memicu write.
// - Debounce 800 ms, tetapi tidak lebih dari 3 detik sejak perubahan pertama yang belum terkirim.
// - Satu request per attempt pada satu waktu; edit selama request berjalan digabung ke batch berikutnya.
// - Batch yang gagal karena jaringan dikirim ulang dengan opId dan payload yang SAMA (server mengembalikan receipt lama).
// - "Tersimpan" hanya setelah ACK commit server. Konflik revisi ditampilkan, tidak ditimpa diam-diam.

export type Value = { selected?: string[]; text?: string } | null;

export type AnswerInput = { itemId: string; value: Value; baseRevision: number };
export type Batch = { opId: string; answers: AnswerInput[] };
export type Ack = { revision: number; savedAt: string; items: { itemId: string; revision: number }[] };
export type ConflictItem = { itemId: string; serverValue: Value; serverRevision: number };

export type SendResult =
  | { kind: "ack"; ack: Ack }
  | { kind: "conflict"; items: ConflictItem[] }
  | { kind: "final"; code: string; message: string }
  | { kind: "retry"; retryAfterMs?: number } // jaringan putus, 5xx, 429
  | { kind: "invalid"; message: string };

export type SaveStatus =
  | { kind: "idle" }
  | { kind: "dirty" }
  | { kind: "saving" }
  | { kind: "saved"; at: Date }
  | { kind: "offline"; pending: number }
  | { kind: "conflict"; items: ConflictItem[] }
  | { kind: "final"; code: string; message: string }
  | { kind: "error"; message: string };

export type Committed = { value: Value; revision: number };

export type QueueOptions = {
  send: (b: Batch) => Promise<SendResult>;
  onStatus?: (s: SaveStatus) => void;
  onPersist?: (snap: QueueSnapshot) => void;
  newOpId?: () => string;
  debounceMs?: number;
  maxDelayMs?: number;
  retryBaseMs?: number;
  maxBatch?: number;
};

/** Bagian state yang disimpan ke IndexedDB untuk pemulihan setelah refresh. */
export type QueueSnapshot = { dirty: Record<string, { value: Value; baseRevision: number }>; pending: Batch | null };

export function sameValue(a: Value, b: Value): boolean {
  const norm = (v: Value) => {
    if (!v) return "";
    if (v.text !== undefined) return "t:" + v.text;
    return "s:" + [...(v.selected ?? [])].sort().join(",");
  };
  return norm(a) === norm(b);
}

export class AutosaveQueue {
  private committed = new Map<string, Committed>();
  private dirty = new Map<string, Value>();
  private pending: Batch | null = null; // batch yang sedang/akan dikirim ulang dengan opId sama
  private inFlight: Promise<void> | null = null;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private firstDirtyAt: number | null = null;
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private retries = 0;
  private finalState: SaveStatus | null = null;
  private conflicts: ConflictItem[] = [];
  private status: SaveStatus = { kind: "idle" };
  private o: Required<Omit<QueueOptions, "onStatus" | "onPersist">> & Pick<QueueOptions, "onStatus" | "onPersist">;

  constructor(opts: QueueOptions) {
    this.o = {
      debounceMs: 800,
      maxDelayMs: 3000,
      retryBaseMs: 1000,
      maxBatch: 50,
      newOpId: () => crypto.randomUUID(),
      ...opts,
    };
  }

  /** Muat state committed dari server (resume). */
  load(answers: { itemId: string; value: Value; revision: number }[]) {
    for (const a of answers) this.committed.set(a.itemId, { value: a.value, revision: a.revision });
  }

  /** Pulihkan draft lokal: item yang base revision-nya masih sama dengan server dikirim ulang; yang berbeda menjadi konflik. */
  restore(snap: QueueSnapshot) {
    if (snap.pending) {
      this.pending = snap.pending;
    }
    const conflicts: ConflictItem[] = [];
    for (const [itemId, d] of Object.entries(snap.dirty)) {
      const c = this.committed.get(itemId) ?? { value: null, revision: 0 };
      if (sameValue(c.value, d.value)) continue;
      if (c.revision === d.baseRevision) this.dirty.set(itemId, d.value);
      else conflicts.push({ itemId, serverValue: c.value, serverRevision: c.revision });
    }
    if (conflicts.length) {
      this.conflicts = conflicts;
      for (const cf of conflicts) this.localConflict.set(cf.itemId, snap.dirty[cf.itemId]!.value);
      this.setStatus({ kind: "conflict", items: conflicts });
    }
    if (this.pending || this.dirty.size) this.schedule(true);
  }

  private localConflict = new Map<string, Value>();

  /** Nilai yang terlihat oleh peserta: dirty > in-flight > committed. */
  valueOf(itemId: string): Value {
    if (this.dirty.has(itemId)) return this.dirty.get(itemId)!;
    const p = this.pending?.answers.find((a) => a.itemId === itemId);
    if (p) return p.value;
    return this.committed.get(itemId)?.value ?? null;
  }

  /** Apakah jawaban item sudah diterima server (bukan hanya di perangkat). */
  isCommitted(itemId: string): boolean {
    return !this.dirty.has(itemId) && !this.pending?.answers.some((a) => a.itemId === itemId) && !this.localConflict.has(itemId);
  }

  localConflictValue(itemId: string): Value | undefined {
    return this.localConflict.get(itemId);
  }

  getStatus() {
    return this.status;
  }

  hasUnsaved() {
    return this.dirty.size > 0 || this.pending !== null;
  }

  set(itemId: string, value: Value) {
    if (this.finalState) return;
    const base = this.pending?.answers.find((a) => a.itemId === itemId)?.value ?? this.committed.get(itemId)?.value ?? null;
    if (sameValue(base, value) && !this.pending?.answers.some((a) => a.itemId === itemId)) {
      // Kembali ke nilai committed: tidak ada yang perlu ditulis.
      this.dirty.delete(itemId);
    } else {
      this.dirty.set(itemId, value);
    }
    this.persist();
    if (this.dirty.size === 0 && !this.pending) {
      if (this.status.kind === "dirty") this.setStatus({ kind: "idle" });
      return;
    }
    if (this.firstDirtyAt === null) this.firstDirtyAt = Date.now();
    this.setStatus({ kind: "dirty" });
    this.schedule(false);
  }

  /** Flush segera (pindah soal, sebelum submit). Resolve setelah semua yang dapat dikirim sudah ACK atau gagal. */
  async flush(): Promise<void> {
    this.clearTimer();
    while (true) {
      if (this.inFlight) {
        await this.inFlight;
        continue;
      }
      if (this.finalState || this.status.kind === "conflict" || this.status.kind === "offline") return;
      if (!this.pending && this.dirty.size === 0) return;
      const before = this.pending?.opId;
      await this.sendOnce();
      if (this.pending && this.pending.opId === before) return; // gagal kirim; jangan berputar
    }
  }

  /** Dipanggil saat koneksi kembali. */
  online() {
    if (this.status.kind === "offline") {
      this.retries = 0;
      this.setStatus({ kind: "saving" });
      void this.flush();
    }
  }

  /** Peserta memilih versi untuk item yang konflik. */
  resolve(itemId: string, choice: "mine" | "server") {
    const cf = this.conflicts.find((c) => c.itemId === itemId);
    if (!cf) return;
    this.committed.set(itemId, { value: cf.serverValue, revision: cf.serverRevision });
    const mine = this.localConflict.get(itemId);
    this.localConflict.delete(itemId);
    if (choice === "mine" && mine !== undefined && !sameValue(mine, cf.serverValue)) this.dirty.set(itemId, mine);
    else this.dirty.delete(itemId);
    this.conflicts = this.conflicts.filter((c) => c.itemId !== itemId);
    this.persist();
    if (this.conflicts.length) this.setStatus({ kind: "conflict", items: this.conflicts });
    else {
      this.setStatus(this.dirty.size ? { kind: "dirty" } : { kind: "idle" });
      if (this.dirty.size) this.schedule(true);
    }
  }

  committedRevision(itemId: string) {
    return this.committed.get(itemId)?.revision ?? 0;
  }

  dispose() {
    this.clearTimer();
    if (this.retryTimer) clearTimeout(this.retryTimer);
  }

  private schedule(immediate: boolean) {
    this.clearTimer();
    if (this.inFlight) return; // akan dijadwalkan ulang setelah request selesai
    const now = Date.now();
    const first = this.firstDirtyAt ?? now;
    const wait = immediate ? 0 : Math.max(0, Math.min(this.o.debounceMs, first + this.o.maxDelayMs - now));
    this.timer = setTimeout(() => {
      this.timer = null;
      void this.sendOnce();
    }, wait);
  }

  private clearTimer() {
    if (this.timer) clearTimeout(this.timer);
    this.timer = null;
  }

  private sendOnce(): Promise<void> {
    if (this.inFlight) return this.inFlight;
    if (this.finalState || this.status.kind === "conflict") return Promise.resolve();
    if (!this.pending) {
      if (this.dirty.size === 0) return Promise.resolve();
      const answers: AnswerInput[] = [];
      for (const [itemId, value] of this.dirty) {
        if (answers.length >= this.o.maxBatch) break;
        answers.push({ itemId, value, baseRevision: this.committed.get(itemId)?.revision ?? 0 });
      }
      for (const a of answers) this.dirty.delete(a.itemId);
      this.pending = { opId: this.o.newOpId(), answers };
      this.firstDirtyAt = this.dirty.size ? Date.now() : null;
      this.persist();
    }
    const batch = this.pending;
    this.setStatus({ kind: "saving" });
    this.inFlight = this.o
      .send(batch)
      .catch((): SendResult => ({ kind: "retry" }))
      .then((r) => this.handle(batch, r))
      .finally(() => {
        this.inFlight = null;
        if (!this.finalState && this.status.kind !== "conflict" && this.status.kind !== "offline" && (this.dirty.size || this.pending)) this.schedule(true);
      });
    return this.inFlight;
  }

  private handle(batch: Batch, r: SendResult) {
    switch (r.kind) {
      case "ack": {
        for (const it of r.ack.items) {
          const a = batch.answers.find((x) => x.itemId === it.itemId);
          if (a) this.committed.set(it.itemId, { value: a.value, revision: it.revision });
        }
        this.pending = null;
        this.retries = 0;
        this.persist();
        this.setStatus(this.dirty.size ? { kind: "dirty" } : { kind: "saved", at: new Date(r.ack.savedAt) });
        return;
      }
      case "conflict": {
        // Batch tidak diterapkan server. Item non-konflik dikembalikan ke dirty dengan base revisi lama.
        const ids = new Set(r.items.map((c) => c.itemId));
        for (const a of batch.answers) {
          if (ids.has(a.itemId)) this.localConflict.set(a.itemId, this.dirty.get(a.itemId) ?? a.value);
          else if (!this.dirty.has(a.itemId)) this.dirty.set(a.itemId, a.value);
        }
        for (const c of r.items) this.dirty.delete(c.itemId);
        this.pending = null;
        this.conflicts = [...this.conflicts.filter((c) => !ids.has(c.itemId)), ...r.items];
        this.persist();
        this.setStatus({ kind: "conflict", items: this.conflicts });
        return;
      }
      case "final":
        this.finalState = { kind: "final", code: r.code, message: r.message };
        this.setStatus(this.finalState);
        return;
      case "invalid":
        // Payload ditolak validasi: buang batch agar antrean tidak macet, tampilkan error.
        this.pending = null;
        this.persist();
        this.setStatus({ kind: "error", message: r.message });
        return;
      case "retry": {
        this.retries++;
        const pendingCount = (this.pending?.answers.length ?? 0) + this.dirty.size;
        this.setStatus({ kind: "offline", pending: pendingCount });
        const base = r.retryAfterMs ?? Math.min(this.o.retryBaseMs * 2 ** (this.retries - 1), 30_000);
        const delay = base * (0.7 + 0.6 * Math.random());
        if (this.retryTimer) clearTimeout(this.retryTimer);
        this.retryTimer = setTimeout(() => this.online(), delay);
        return;
      }
    }
  }

  /** Simpan ulang snapshot (mis. setelah tanda ragu berubah) tanpa menandai jawaban berubah. */
  persistNow() {
    this.persist();
  }

  private persist() {
    if (!this.o.onPersist) return;
    const dirty: QueueSnapshot["dirty"] = {};
    for (const [id, v] of this.dirty) dirty[id] = { value: v, baseRevision: this.committed.get(id)?.revision ?? 0 };
    for (const [id, v] of this.localConflict) dirty[id] = { value: v, baseRevision: this.committed.get(id)?.revision ?? 0 };
    this.o.onPersist({ dirty, pending: this.pending });
  }

  private setStatus(s: SaveStatus) {
    this.status = s;
    this.o.onStatus?.(s);
  }
}
