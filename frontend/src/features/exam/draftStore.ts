// Draft lokal jawaban ujian di IndexedDB, untuk pemulihan setelah refresh di perangkat yang sama.
// Draft BELUM dijamin diterima server. Kunci = userId + attemptId; dihapus saat logout, finalisasi, ganti akun,
// atau setelah TTL. Pada perangkat bersama, draft hanya bisa dibaca oleh sesi browser yang sama sampai logout.
import type { QueueSnapshot } from "./autosave";

const DB = "ats-exam";
const STORE = "drafts";

export type Draft = QueueSnapshot & {
  key: string;
  userId: string;
  attemptId: string;
  submitKey?: string;
  flagged?: string[];
  expiresAt: number;
  updatedAt: number;
};

function open(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB, 1);
    req.onupgradeneeded = () => {
      const s = req.result.createObjectStore(STORE, { keyPath: "key" });
      s.createIndex("userId", "userId");
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

async function tx<T>(mode: IDBTransactionMode, fn: (s: IDBObjectStore) => IDBRequest<T> | void): Promise<T | undefined> {
  const db = await open();
  try {
    return await new Promise<T | undefined>((resolve, reject) => {
      const t = db.transaction(STORE, mode);
      const r = fn(t.objectStore(STORE));
      t.oncomplete = () => resolve(r ? r.result : undefined);
      t.onerror = () => reject(t.error);
      t.onabort = () => reject(t.error);
    });
  } finally {
    db.close();
  }
}

export const draftKey = (userId: string, attemptId: string) => `${userId}:${attemptId}`;

export async function loadDraft(userId: string, attemptId: string, now = Date.now()): Promise<Draft | null> {
  try {
    const d = await tx<Draft>("readonly", (s) => s.get(draftKey(userId, attemptId)));
    if (!d) return null;
    if (d.userId !== userId || d.expiresAt < now) {
      await deleteDraft(userId, attemptId);
      return null;
    }
    return d;
  } catch {
    return null; // IndexedDB tidak tersedia (mode privat): ujian tetap jalan tanpa pemulihan lokal
  }
}

export async function saveDraft(d: Omit<Draft, "key" | "updatedAt">): Promise<void> {
  try {
    await tx("readwrite", (s) => s.put({ ...d, key: draftKey(d.userId, d.attemptId), updatedAt: Date.now() }));
  } catch {
    /* best-effort */
  }
}

export async function deleteDraft(userId: string, attemptId: string): Promise<void> {
  try {
    await tx("readwrite", (s) => s.delete(draftKey(userId, attemptId)));
  } catch {
    /* best-effort */
  }
}

/** Hapus semua draft (logout / ganti akun). */
export async function clearAllDrafts(): Promise<void> {
  try {
    await tx("readwrite", (s) => s.clear());
  } catch {
    /* best-effort */
  }
}

/** Selisih jam server - jam klien, dikoreksi setengah round-trip. Jam klien tidak pernah memperpanjang deadline. */
export function clockOffset(serverTimeIso: string, sentAt: number, receivedAt: number): number {
  const server = Date.parse(serverTimeIso);
  return server - (sentAt + (receivedAt - sentAt) / 2);
}

export function remainingMs(deadlineIso: string, offset: number, now = Date.now()): number {
  return Math.max(0, Date.parse(deadlineIso) - (now + offset));
}

export function fmtRemaining(ms: number): string {
  const s = Math.ceil(ms / 1000);
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  const p = (n: number) => String(n).padStart(2, "0");
  return h > 0 ? `${h}:${p(m)}:${p(sec)}` : `${p(m)}:${p(sec)}`;
}
