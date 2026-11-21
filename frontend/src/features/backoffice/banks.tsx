import { useState } from "react";
import { ApiError, post } from "../../lib/api";
import { fmtDateTime } from "../../lib/format";
import { useQuery } from "../../lib/useQuery";
import { Alert, Button, Field, Input, PageHeader, PageState, Select, Textarea, useToast } from "../../ui";

type Bank = { id: string; name: string; category: string; questionCount: number };
type Pkg = { id: string; key: string; name: string; latestVersionNo: number | null; durationMinutes: number | null; passScore: number | null; itemCount: number | null; publishedAt: string | null };
type Question = { questionId: string; versionId: string; versionNo: number; kind: string; stem: string; options: { id: string; text: string }[]; answerKey: string[]; weight: number };
type Row = { line: number; kind: string; stem: string; options: string[] | null; key: string[] | null; weight: number; errors: string[] };

const kindLabel: Record<string, string> = { single: "Pilihan tunggal", multiple: "Pilihan jamak", essay: "Esai" };
const template = "jenis;pertanyaan;opsi;kunci;bobot;rubrik\ntunggal;Contoh pertanyaan?;Pilihan A|Pilihan B|Pilihan C;B;1;\nesai;Contoh esai;;;2;Rubrik penilaian";

export function BanksPage() {
  const q = useQuery<{ banks: Bank[]; packages: Pkg[] }>("/exam-banks");
  const [bankId, setBankId] = useState<string | null>(null);
  const toast = useToast();
  const [newBank, setNewBank] = useState({ name: "", category: "" });
  const [err, setErr] = useState<ApiError | null>(null);

  return (
    <div className="max-w-6xl">
      <PageHeader title="Bank soal dan paket ujian" subtitle="Perubahan soal selalu membuat versi baru. Ujian yang sedang berjalan tidak terpengaruh." />
      {err ? (
        <div className="mb-4">
          <Alert tone="danger">{Object.values(err.fieldErrors)[0] ?? err.message}</Alert>
        </div>
      ) : null}
      <PageState loading={q.loading} error={q.error} onRetry={q.reload}>
        <div className="grid gap-8 lg:grid-cols-2">
          <section>
            <h2 className="mb-2 text-[20px]">Bank soal</h2>
            <ul className="divide-y divide-line border-y border-line">
              {q.data?.banks.map((b) => (
                <li key={b.id} className="flex items-center justify-between gap-2 py-3">
                  <span>
                    {b.name} <span className="text-sm text-ink-2">· {b.category || "Tanpa kategori"} · {b.questionCount} soal</span>
                  </span>
                  <Button size="sm" variant={bankId === b.id ? "primary" : "secondary"} onClick={() => setBankId(b.id)}>
                    Buka
                  </Button>
                </li>
              ))}
            </ul>
            <form
              className="mt-3 flex flex-wrap items-end gap-2"
              onSubmit={async (e) => {
                e.preventDefault();
                setErr(null);
                try {
                  await post("/exam-banks", newBank);
                  toast("success", "Bank soal dibuat.");
                  setNewBank({ name: "", category: "" });
                  q.reload();
                } catch (x) {
                  setErr(x as ApiError);
                }
              }}
            >
              <Field label="Nama bank baru" required>{({ id }) => <Input id={id} value={newBank.name} onChange={(e) => setNewBank({ ...newBank, name: e.target.value })} />}</Field>
              <Field label="Kategori">{({ id }) => <Input id={id} value={newBank.category} onChange={(e) => setNewBank({ ...newBank, category: e.target.value })} />}</Field>
              <Button type="submit">Buat bank</Button>
            </form>
          </section>
          <section>
            <h2 className="mb-2 text-[20px]">Paket ujian</h2>
            <ul className="divide-y divide-line border-y border-line">
              {q.data?.packages.map((p) => (
                <li key={p.id} className="py-3">
                  {p.name} <span className="text-sm text-ink-2">({p.key})</span>
                  <span className="block text-sm text-ink-2">
                    {p.latestVersionNo ? `Versi ${p.latestVersionNo} · ${p.itemCount} soal · ${p.durationMinutes} menit · ambang ${p.passScore} · terbit ${fmtDateTime(p.publishedAt)}` : "Belum ada versi"}
                  </span>
                </li>
              ))}
            </ul>
          </section>
        </div>
      </PageState>
      {bankId ? <BankDetail key={bankId} bankId={bankId} packages={q.data?.packages ?? []} onChange={q.reload} /> : null}
    </div>
  );
}

function BankDetail({ bankId, packages, onChange }: { bankId: string; packages: Pkg[]; onChange: () => void }) {
  const q = useQuery<{ bank: Bank; questions: Question[] }>(`/exam-banks/${bankId}/questions`);
  const toast = useToast();
  const [content, setContent] = useState("");
  const [preview, setPreview] = useState<{ rows: Row[]; digest: string; valid: boolean; invalidCount: number } | null>(null);
  const [err, setErr] = useState<ApiError | null>(null);
  const [sel, setSel] = useState<Set<string>>(new Set());
  const [pkg, setPkg] = useState({ key: packages[0]?.key ?? "", name: "", duration: "30", pass: "75" });

  const doPreview = async () => {
    setErr(null);
    try {
      setPreview(await post(`/exam-banks/${bankId}/import/preview`, { content }));
    } catch (e) {
      setErr(e as ApiError);
      setPreview(null);
    }
  };
  const doImport = async () => {
    if (!preview) return;
    setErr(null);
    try {
      const r = await post<{ imported: number }>(`/exam-banks/${bankId}/import`, { content, digest: preview.digest });
      toast("success", `${r.imported} soal diimpor.`);
      setContent("");
      setPreview(null);
      q.reload();
      onChange();
    } catch (e) {
      setErr(e as ApiError);
    }
  };
  const publish = async () => {
    setErr(null);
    try {
      const r = await post<{ versionNo: number }>(`/exam-packages/${encodeURIComponent(pkg.key)}/versions`, {
        name: pkg.name, questionVersionIds: [...sel], durationMinutes: Number(pkg.duration), passScore: Number(pkg.pass), shuffleItems: true, shuffleOptions: true,
      });
      toast("success", `Paket ${pkg.key} versi ${r.versionNo} diterbitkan. Jadwal baru memakai versi ini.`);
      setSel(new Set());
      onChange();
    } catch (e) {
      setErr(e as ApiError);
    }
  };

  return (
    <section className="mt-10 border-t border-line pt-6">
      <PageState loading={q.loading} error={q.error} onRetry={q.reload}>
        {q.data ? (
          <>
            <h2 className="text-[22px]">{q.data.bank.name}</h2>
            {err ? (
              <div className="my-3">
                <Alert tone="danger">{Object.values(err.fieldErrors)[0] ?? err.message}</Alert>
              </div>
            ) : null}
            <div className="mt-4 overflow-x-auto">
              <table className="w-full border-collapse text-left text-sm">
                <caption className="sr-only">Soal dalam bank</caption>
                <thead>
                  <tr className="border-b border-line text-ink-2">
                    <th scope="col" className="py-2 pr-3">Pilih</th>
                    <th scope="col" className="py-2 pr-3">Soal</th>
                    <th scope="col" className="py-2 pr-3">Jenis</th>
                    <th scope="col" className="py-2 pr-3">Kunci</th>
                    <th scope="col" className="py-2">Versi</th>
                  </tr>
                </thead>
                <tbody>
                  {q.data.questions.map((x) => (
                    <tr key={x.versionId} className="border-b border-line align-top">
                      <td className="py-2 pr-3">
                        <input
                          type="checkbox"
                          className="h-5 w-5"
                          aria-label={`Pilih soal ${x.stem.slice(0, 40)}`}
                          checked={sel.has(x.versionId)}
                          onChange={(e) => {
                            const n = new Set(sel);
                            if (e.target.checked) n.add(x.versionId);
                            else n.delete(x.versionId);
                            setSel(n);
                          }}
                        />
                      </td>
                      <td className="py-2 pr-3">{x.stem}</td>
                      <td className="py-2 pr-3">{kindLabel[x.kind]}</td>
                      <td className="py-2 pr-3">{x.answerKey.map((k) => k.toUpperCase()).join(", ") || "-"}</td>
                      <td className="py-2">{x.versionNo}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <div className="mt-6 grid gap-8 lg:grid-cols-2">
              <div>
                <h3 className="mb-2 text-[18px]">Impor soal (CSV, pemisah titik koma)</h3>
                <p className="mb-2 text-sm text-ink-2">Kolom: jenis (tunggal/jamak/esai); pertanyaan; opsi dipisah |; kunci huruf dipisah koma; bobot; rubrik.</p>
                <Field label="Isi CSV" required>{({ id }) => <Textarea id={id} className="min-h-40" value={content} placeholder={template} onChange={(e) => { setContent(e.target.value); setPreview(null); }} />}</Field>
                <div className="mt-2 flex gap-2">
                  <Button onClick={doPreview} disabled={!content.trim()}>
                    Pratinjau
                  </Button>
                  {preview?.valid ? (
                    <Button variant="primary" onClick={doImport}>
                      Impor {preview.rows.length} soal
                    </Button>
                  ) : null}
                </div>
                {preview ? (
                  <div className="mt-3">
                    <Alert tone={preview.valid ? "success" : "warning"} title={preview.valid ? `${preview.rows.length} baris valid` : `${preview.invalidCount} baris perlu diperbaiki`}>
                      <ul className="mt-1">
                        {preview.rows.map((r) => (
                          <li key={r.line}>
                            Baris {r.line}: {kindLabel[r.kind] ?? "?"} · {r.stem.slice(0, 60) || "(kosong)"}
                            {r.errors.length ? <span className="block text-danger">{r.errors.join(" ")}</span> : null}
                          </li>
                        ))}
                      </ul>
                    </Alert>
                  </div>
                ) : null}
              </div>
              <div>
                <h3 className="mb-2 text-[18px]">Terbitkan versi paket</h3>
                <p className="mb-2 text-sm text-ink-2">{sel.size} soal dipilih. Versi paket tidak dapat diubah setelah terbit.</p>
                <div className="flex flex-col gap-3">
                  <Field label="Kunci paket" required hint="Paket baru dibuat bila kunci belum ada.">
                    {({ id, describedBy }) => (
                      <Select id={id} aria-describedby={describedBy} value={pkg.key} onChange={(e) => setPkg({ ...pkg, key: e.target.value })}>
                        {packages.map((p) => (
                          <option key={p.key} value={p.key}>
                            {p.key}
                          </option>
                        ))}
                      </Select>
                    )}
                  </Field>
                  <div className="grid grid-cols-2 gap-3">
                    <Field label="Durasi (menit)" required>{({ id }) => <Input id={id} inputMode="numeric" value={pkg.duration} onChange={(e) => setPkg({ ...pkg, duration: e.target.value })} />}</Field>
                    <Field label="Ambang lulus" required>{({ id }) => <Input id={id} inputMode="decimal" value={pkg.pass} onChange={(e) => setPkg({ ...pkg, pass: e.target.value })} />}</Field>
                  </div>
                  <Button variant="primary" disabled={sel.size === 0} onClick={publish}>
                    Terbitkan versi baru
                  </Button>
                </div>
              </div>
            </div>
          </>
        ) : null}
      </PageState>
    </section>
  );
}
