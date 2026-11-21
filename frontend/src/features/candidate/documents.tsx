import { useRef, useState } from "react";
import { ApiError, apiForm, del, post, type Schemas } from "../../lib/api";
import { fmtDateTime } from "../../lib/format";
import { useQuery } from "../../lib/useQuery";
import { Alert, Button, Field, PageState, Select, useToast } from "../../ui";

export type Doc = Schemas["Document"];

export const kindLabel: Record<string, string> = { cv: "CV", ijazah: "Ijazah", sertifikat: "Sertifikat", lainnya: "Lainnya" };
export const scanLabel: Record<string, string> = {
  pending: "Menunggu pemindaian keamanan",
  clean: "Siap",
  infected: "Ditolak pemindai keamanan",
  error: "Pemindaian gagal",
};

/** Membuka tautan unduh berumur 60 detik untuk dokumen yang sudah lolos pindai. */
export async function openDocument(id: string) {
  const r = await post<{ url: string }>(`/documents/${id}/link`);
  window.location.assign(r.url);
}

export function DocumentsSection() {
  const q = useQuery<{ items: Doc[] }>("/me/documents");
  const toast = useToast();
  const fileRef = useRef<HTMLInputElement>(null);
  const [kind, setKind] = useState("cv");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<ApiError | null>(null);

  const upload = async () => {
    const f = fileRef.current?.files?.[0];
    if (!f) {
      setErr(new ApiError(422, { error: { code: "validation_failed", message: "Pilih berkas terlebih dahulu.", fieldErrors: { file: "Pilih berkas." } }, requestId: "" }, null));
      return;
    }
    const limitMB = kind === "cv" ? 15 : 5;
    if (f.size > limitMB * 1024 * 1024) {
      setErr(new ApiError(413, { error: { code: "payload_too_large", message: `Ukuran berkas maksimal ${limitMB} MB.` }, requestId: "" }, null));
      return;
    }
    const fd = new FormData();
    fd.append("kind", kind);
    fd.append("file", f);
    setBusy(true);
    setErr(null);
    try {
      await apiForm("/me/documents", fd);
      toast("success", "Dokumen diunggah dan menunggu pemindaian keamanan.");
      if (fileRef.current) fileRef.current.value = "";
      q.reload();
    } catch (e) {
      setErr(e as ApiError);
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-[20px]">Dokumen</h2>
      <p className="text-sm text-ink-2">PDF, DOCX, DOC, JPG, PNG, atau WebP. CV maksimal 15 MB, dokumen lain 5 MB. Dokumen dapat dibuka setelah lolos pemindaian keamanan.</p>
      {err ? <Alert tone="danger">{err.fieldErrors.file ?? err.message}</Alert> : null}
      <div className="grid gap-3 sm:grid-cols-[160px_1fr_auto] sm:items-end">
        <Field label="Jenis" required>
          {({ id }) => (
            <Select id={id} value={kind} onChange={(e) => setKind(e.target.value)}>
              {Object.entries(kindLabel).map(([k, v]) => (
                <option key={k} value={k}>
                  {v}
                </option>
              ))}
            </Select>
          )}
        </Field>
        <Field label="Berkas" required>
          {({ id }) => <input id={id} ref={fileRef} type="file" accept=".pdf,.docx,.doc,.jpg,.jpeg,.png,.webp" className="field-input py-2" />}
        </Field>
        <Button type="button" busy={busy} onClick={upload}>
          Unggah
        </Button>
      </div>
      <PageState loading={q.loading} error={q.error} onRetry={q.reload} empty={q.data?.items.length === 0} emptyText="Belum ada dokumen.">
        <ul className="divide-y divide-line border-y border-line">
          {q.data?.items.map((d) => (
            <li key={d.id} className="flex flex-wrap items-center justify-between gap-2 py-3">
              <span>
                <span className="font-semibold">{kindLabel[d.kind]}</span> · {d.fileName} · {Math.ceil(d.sizeBytes / 1024)} KB · {scanLabel[d.scanStatus] ?? d.scanStatus}
                <span className="block text-[13px] text-ink-2">Diunggah {fmtDateTime(d.createdAt)}</span>
              </span>
              <span className="flex gap-2">
                {d.scanStatus === "clean" ? (
                  <Button type="button" size="sm" onClick={() => openDocument(d.id).catch((e: ApiError) => setErr(e))}>
                    Unduh
                  </Button>
                ) : null}
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  aria-label={`Hapus ${d.fileName}`}
                  onClick={async () => {
                    try {
                      await del(`/me/documents/${d.id}`);
                      toast("success", "Dokumen dihapus.");
                    } catch (e) {
                      setErr(e as ApiError);
                    } finally {
                      q.reload();
                    }
                  }}
                >
                  Hapus
                </Button>
              </span>
            </li>
          ))}
        </ul>
      </PageState>
    </section>
  );
}
