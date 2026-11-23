import { useEffect } from "react";
import { post } from "../../lib/api";

/** Jenis aktivitas yang dicatat. Peserta diberi tahu di ruang ujian; tidak ada kamera atau isi layar yang direkam. */
export type ActivityKind = "tab_hidden" | "tab_visible" | "window_blur" | "copy" | "paste" | "offline" | "online";
type Pending = { seq: number; kind: ActivityKind; at: string; durationMs?: number };
type Stored = { seq: number; pending: Pending[]; hiddenAt?: number };

const FLUSH_MS = 5_000;
const BATCH = 50;

// Antrean disimpan di localStorage per percobaan, sehingga catatan saat koneksi putus atau tab ditutup tetap terkirim
// saat halaman dibuka lagi. Gagal menyimpan (mode privat, kuota penuh) tidak mengganggu ujian.
function load(key: string): Stored {
  try {
    const raw = localStorage.getItem(key);
    if (raw) return JSON.parse(raw) as Stored;
  } catch {
    /* abaikan */
  }
  return { seq: 0, pending: [] };
}
function store(key: string, s: Stored) {
  try {
    localStorage.setItem(key, JSON.stringify(s));
  } catch {
    /* abaikan */
  }
}

/** Mencatat pindah tab, keluar jendela, salin, tempel, dan status koneksi selama percobaan berjalan. */
export function useActivityLog(attemptId: string, active: boolean) {
  useEffect(() => {
    if (!active || !attemptId) return;
    const key = `exam-activity:${attemptId}`;
    let state = load(key);
    let sending = false;

    const add = (kind: ActivityKind, durationMs?: number) => {
      state.seq += 1;
      state.pending.push({ seq: state.seq, kind, at: new Date().toISOString(), ...(durationMs !== undefined ? { durationMs } : {}) });
      store(key, state);
    };
    const flush = async () => {
      if (sending || state.pending.length === 0 || !navigator.onLine) return;
      sending = true;
      const batch = state.pending.slice(0, BATCH);
      try {
        await post(`/exam-attempts/${attemptId}/activity`, { events: batch });
        const sent = new Set(batch.map((b) => b.seq));
        state = { ...state, pending: state.pending.filter((p) => !sent.has(p.seq)) };
        store(key, state);
      } catch {
        /* dicoba lagi pada putaran berikutnya */
      } finally {
        sending = false;
      }
    };

    const onVisibility = () => {
      if (document.visibilityState === "hidden") {
        state.hiddenAt = Date.now();
        add("tab_hidden");
      } else {
        const d = state.hiddenAt ? Date.now() - state.hiddenAt : undefined;
        state.hiddenAt = undefined;
        add("tab_visible", d);
        void flush();
      }
    };
    // Blur saat tab tetap terlihat = berpindah ke jendela atau aplikasi lain.
    const onBlur = () => {
      if (document.visibilityState === "visible") add("window_blur");
    };
    const onCopy = () => add("copy");
    const onPaste = () => add("paste");
    const onOffline = () => add("offline");
    const onOnline = () => {
      add("online");
      void flush();
    };

    document.addEventListener("visibilitychange", onVisibility);
    window.addEventListener("blur", onBlur);
    document.addEventListener("copy", onCopy);
    document.addEventListener("paste", onPaste);
    window.addEventListener("offline", onOffline);
    window.addEventListener("online", onOnline);
    const timer = setInterval(() => void flush(), FLUSH_MS);
    void flush();
    return () => {
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener("blur", onBlur);
      document.removeEventListener("copy", onCopy);
      document.removeEventListener("paste", onPaste);
      window.removeEventListener("offline", onOffline);
      window.removeEventListener("online", onOnline);
      clearInterval(timer);
      void flush();
    };
  }, [attemptId, active]);
}
