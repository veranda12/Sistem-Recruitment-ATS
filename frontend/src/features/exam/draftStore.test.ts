import "fake-indexeddb/auto";
import { describe, expect, it } from "vitest";
import { clearAllDrafts, clockOffset, deleteDraft, fmtRemaining, loadDraft, remainingMs, saveDraft } from "./draftStore";

const base = { pending: null, dirty: { i1: { value: { selected: ["x"] }, baseRevision: 2 } } };

describe("draft IndexedDB", () => {
  it("terikat user dan attempt", async () => {
    await saveDraft({ ...base, userId: "u1", attemptId: "a1", expiresAt: Date.now() + 60_000 });
    expect((await loadDraft("u1", "a1"))?.dirty.i1?.baseRevision).toBe(2);
    expect(await loadDraft("u2", "a1")).toBeNull();
    expect(await loadDraft("u1", "a2")).toBeNull();
  });

  it("TTL: draft kedaluwarsa dihapus", async () => {
    await saveDraft({ ...base, userId: "u1", attemptId: "a3", expiresAt: Date.now() - 1 });
    expect(await loadDraft("u1", "a3")).toBeNull();
  });

  it("dihapus saat finalisasi dan logout", async () => {
    await saveDraft({ ...base, userId: "u1", attemptId: "a4", expiresAt: Date.now() + 60_000 });
    await deleteDraft("u1", "a4");
    expect(await loadDraft("u1", "a4")).toBeNull();
    await saveDraft({ ...base, userId: "u1", attemptId: "a5", expiresAt: Date.now() + 60_000 });
    await clearAllDrafts();
    expect(await loadDraft("u1", "a5")).toBeNull();
  });
});

describe("kalibrasi waktu", () => {
  it("offset memakai jam server dan setengah RTT; jam klien maju tidak menambah sisa waktu", () => {
    const sent = 1_000_000;
    const recv = sent + 200;
    const off = clockOffset(new Date(sent + 100 + 5_000).toISOString(), sent, recv); // klien terlambat 5 detik
    expect(off).toBe(5_000);
    const deadline = new Date(sent + 100 + 5_000 + 60_000).toISOString();
    expect(remainingMs(deadline, off, sent + 100)).toBe(60_000);
    // Klien mengubah jam mundur 10 menit: offset hasil kalibrasi ulang mengoreksi, sisa waktu tidak bertambah.
    const off2 = clockOffset(new Date(sent + 100 + 5_000).toISOString(), sent - 600_000, recv - 600_000);
    expect(remainingMs(deadline, off2, sent + 100 - 600_000)).toBe(60_000);
    expect(remainingMs(deadline, off, sent + 10_000_000)).toBe(0);
  });
  it("format sisa waktu", () => {
    expect(fmtRemaining(65_000)).toBe("01:05");
    expect(fmtRemaining(3_725_000)).toBe("1:02:05");
  });
});
