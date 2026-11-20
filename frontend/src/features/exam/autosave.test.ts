import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AutosaveQueue, type Batch, type SaveStatus, type SendResult } from "./autosave";

type Call = { batch: Batch; resolve: (r: SendResult) => void };

function harness(opts: Partial<ConstructorParameters<typeof AutosaveQueue>[0]> = {}) {
  const calls: Call[] = [];
  const statuses: SaveStatus[] = [];
  let n = 0;
  const q = new AutosaveQueue({
    send: (batch) => new Promise<SendResult>((resolve) => calls.push({ batch: structuredClone(batch), resolve })),
    onStatus: (s) => statuses.push(s),
    newOpId: () => `op-${++n}`,
    retryBaseMs: 1000,
    ...opts,
  });
  const ack = (i: number, rev = 1) => {
    const c = calls[i]!;
    c.resolve({ kind: "ack", ack: { revision: rev, savedAt: "2026-11-20T03:00:00Z", items: c.batch.answers.map((a) => ({ itemId: a.itemId, revision: a.baseRevision + 1 })) } });
  };
  return { q, calls, statuses, ack };
}

const tick = async (ms: number) => {
  await vi.advanceTimersByTimeAsync(ms);
};

beforeEach(() => {
  vi.useFakeTimers();
  vi.spyOn(Math, "random").mockReturnValue(0.5);
});
afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("AutosaveQueue", () => {
  it("debounce 800 ms setelah perubahan terakhir", async () => {
    const { q, calls } = harness();
    q.set("i1", { selected: ["x"] });
    await tick(799);
    expect(calls).toHaveLength(0);
    await tick(1);
    expect(calls).toHaveLength(1);
    expect(calls[0]!.batch.answers).toEqual([{ itemId: "i1", value: { selected: ["x"] }, baseRevision: 0 }]);
  });

  it("mengetik terus-menerus tetap dikirim paling lambat 3 detik", async () => {
    const { q, calls } = harness();
    for (let t = 0; t < 3000; t += 500) {
      q.set("e1", { text: "a".repeat(t / 100 + 1) });
      await tick(500);
    }
    expect(calls).toHaveLength(1);
    expect(calls[0]!.batch.answers[0]!.value).toEqual({ text: "a".repeat(26) });
  });

  it("navigasi tanpa perubahan tidak memicu write; hanya item berubah yang dikirim", async () => {
    const { q, calls } = harness();
    q.load([
      { itemId: "i1", value: { selected: ["x"] }, revision: 3 },
      { itemId: "i2", value: { selected: ["y"] }, revision: 1 },
    ]);
    q.set("i1", { selected: ["x"] });
    await q.flush();
    expect(calls).toHaveLength(0);
    q.set("i2", { selected: ["z"] });
    await tick(800);
    expect(calls[0]!.batch.answers).toEqual([{ itemId: "i2", value: { selected: ["z"] }, baseRevision: 1 }]);
  });

  it("serialisasi per attempt: edit saat request berjalan digabung ke batch berikutnya", async () => {
    const { q, calls, ack } = harness();
    q.set("i1", { selected: ["a"] });
    await tick(800);
    expect(calls).toHaveLength(1);
    q.set("i1", { selected: ["b"] });
    q.set("i2", { selected: ["c"] });
    await tick(5000);
    expect(calls).toHaveLength(1); // belum ada request paralel
    ack(0);
    await tick(0);
    await tick(0);
    expect(calls).toHaveLength(2);
    expect(calls[1]!.batch.answers).toEqual([
      { itemId: "i1", value: { selected: ["b"] }, baseRevision: 1 },
      { itemId: "i2", value: { selected: ["c"] }, baseRevision: 0 },
    ]);
  });

  it("label tersimpan hanya setelah ACK; gagal jaringan mengirim ulang opId dan payload yang sama", async () => {
    const { q, calls, statuses, ack } = harness();
    q.set("i1", { selected: ["a"] });
    await tick(800);
    expect(statuses.some((s) => s.kind === "saved")).toBe(false);
    calls[0]!.resolve({ kind: "retry" });
    await tick(0);
    expect(q.getStatus().kind).toBe("offline");
    expect(q.isCommitted("i1")).toBe(false);
    q.set("i2", { selected: ["b"] }); // edit baru saat offline tidak mengubah batch yang sedang diulang
    await tick(1000);
    expect(calls).toHaveLength(2);
    expect(calls[1]!.batch).toEqual(calls[0]!.batch);
    ack(1);
    await tick(0);
    await tick(0);
    expect(q.isCommitted("i1")).toBe(true);
    expect(calls[2]!.batch.opId).not.toBe(calls[0]!.batch.opId);
    expect(calls[2]!.batch.answers.map((a) => a.itemId)).toEqual(["i2"]);
    ack(2);
    await tick(0);
    expect(q.getStatus().kind).toBe("saved");
  });

  it("konflik ditampilkan dan diselesaikan eksplisit tanpa overwrite diam-diam", async () => {
    const { q, calls, ack } = harness();
    q.load([{ itemId: "i1", value: { selected: ["a"] }, revision: 1 }]);
    q.set("i1", { selected: ["b"] });
    await tick(800);
    calls[0]!.resolve({ kind: "conflict", items: [{ itemId: "i1", serverValue: { selected: ["c"] }, serverRevision: 2 }] });
    await tick(0);
    expect(q.getStatus().kind).toBe("conflict");
    expect(q.localConflictValue("i1")).toEqual({ selected: ["b"] });
    await tick(10_000);
    expect(calls).toHaveLength(1); // tidak mengirim ulang otomatis
    q.resolve("i1", "mine");
    await tick(0);
    expect(calls[1]!.batch.answers).toEqual([{ itemId: "i1", value: { selected: ["b"] }, baseRevision: 2 }]);
    ack(1);
    await tick(0);
    expect(q.valueOf("i1")).toEqual({ selected: ["b"] });
  });

  it("memilih versi server membuang perubahan lokal", async () => {
    const { q, calls } = harness();
    q.load([{ itemId: "i1", value: { selected: ["a"] }, revision: 1 }]);
    q.set("i1", { selected: ["b"] });
    await tick(800);
    calls[0]!.resolve({ kind: "conflict", items: [{ itemId: "i1", serverValue: { selected: ["c"] }, serverRevision: 2 }] });
    await tick(0);
    q.resolve("i1", "server");
    await tick(5000);
    expect(calls).toHaveLength(1);
    expect(q.valueOf("i1")).toEqual({ selected: ["c"] });
    expect(q.hasUnsaved()).toBe(false);
  });

  it("flush sebelum submit menunggu ACK", async () => {
    const { q, calls, ack } = harness();
    q.set("i1", { selected: ["a"] });
    const p = q.flush();
    await tick(0);
    expect(calls).toHaveLength(1);
    let done = false;
    void p.then(() => (done = true));
    await tick(0);
    expect(done).toBe(false);
    ack(0);
    await tick(0);
    expect(done).toBe(true);
    expect(q.hasUnsaved()).toBe(false);
  });

  it("setelah final, write baru tidak dikirim", async () => {
    const { q, calls } = harness();
    q.set("i1", { selected: ["a"] });
    await tick(800);
    calls[0]!.resolve({ kind: "final", code: "deadline_passed", message: "Waktu habis" });
    await tick(0);
    q.set("i1", { selected: ["b"] });
    await tick(5000);
    expect(calls).toHaveLength(1);
    expect(q.getStatus().kind).toBe("final");
  });

  it("pemulihan draft: batch tertunda dikirim ulang dengan opId sama; revisi server berubah menjadi konflik", async () => {
    const { q, calls } = harness();
    q.load([
      { itemId: "i1", value: { selected: ["a"] }, revision: 1 },
      { itemId: "i2", value: { selected: ["s"] }, revision: 4 },
    ]);
    q.restore({
      pending: { opId: "op-lama", answers: [{ itemId: "i3", value: { selected: ["z"] }, baseRevision: 0 }] },
      dirty: { i1: { value: { selected: ["b"] }, baseRevision: 1 }, i2: { value: { selected: ["t"] }, baseRevision: 3 } },
    });
    expect(q.getStatus().kind).toBe("conflict");
    expect(q.localConflictValue("i2")).toEqual({ selected: ["t"] });
    q.resolve("i2", "server");
    await tick(0);
    expect(calls[0]!.batch.opId).toBe("op-lama");
  });
});
