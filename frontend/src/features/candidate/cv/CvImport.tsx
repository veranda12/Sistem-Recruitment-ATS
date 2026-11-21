import { useRef, useState } from "react";
import { Link, useNavigate } from "react-router";
import { ApiError, apiForm, post, type Schemas } from "../../../lib/api";
import { fmtDateTime } from "../../../lib/format";
import { useQuery } from "../../../lib/useQuery";
import { Alert, Button, Field } from "../../../ui";
import { inProgress, statusText, type Run } from "./model";

const MAX_MB = 15;
const ACCEPT = ".pdf,.docx,.doc,.jpg,.jpeg,.png,.webp";

/** Aksi "Isi profil dari CV": unggah CV, minta pembacaan, lalu buka halaman review. Form manual tetap tersedia. */
export function CvImport() {
  const nav = useNavigate();
  const fileRef = useRef<HTMLInputElement>(null);
  const runs = useQuery<{ items: Schemas["CvParseRunItem"][] }>("/me/cv-parses");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const start = async () => {
    const f = fileRef.current?.files?.[0];
    if (!f) {
      setErr("Pilih berkas CV terlebih dahulu.");
      return;
    }
    if (f.size > MAX_MB * 1024 * 1024) {
      setErr(`Ukuran berkas melebihi ${MAX_MB} MB. Kecilkan ukuran berkas lalu unggah kembali.`);
      return;
    }
    setBusy(true);
    setErr(null);
    try {
      const fd = new FormData();
      fd.append("kind", "cv");
      fd.append("file", f);
      const doc = await apiForm<{ id: string }>("/me/documents", fd);
      const run = await post<Run>(`/me/documents/${doc.id}/parse`);
      nav(`/akun/profil/cv/${run.id}`);
    } catch (e) {
      const ae = e as ApiError;
      setErr(ae.fieldErrors.file ?? ae.message);
    } finally {
      setBusy(false);
    }
  };

  const open = runs.data?.items.find((r) => inProgress(r) || r.status === "ready_for_review" || r.status === "partial");

  return (
    <section aria-labelledby="cv-import-title" className="flex flex-col gap-3 border-y border-line py-5">
      <h2 id="cv-import-title" className="text-[20px]">
        Isi profil dari CV
      </h2>
      <p className="text-sm text-ink-2">
        Sistem membaca CV Anda dan menampilkan usulan isian beserta bagian CV yang menjadi sumbernya. Profil hanya berubah setelah Anda memeriksa dan
        menyetujui usulan. Anda tetap bisa mengisi form di bawah secara manual.
      </p>
      {open ? (
        <Alert tone="info" title={`${statusText(open)}: ${open.documentName}`}>
          <Link to={`/akun/profil/cv/${open.id}`} className="font-semibold text-primary underline">
            {inProgress(open) ? "Lihat progres" : "Lanjutkan pemeriksaan"}
          </Link>{" "}
          <span className="text-ink-2">· dimulai {fmtDateTime(open.createdAt)}</span>
        </Alert>
      ) : null}
      {err ? (
        <Alert tone="danger" title="CV belum dapat diproses">
          {err}
        </Alert>
      ) : null}
      <div className="grid gap-3 sm:grid-cols-[1fr_auto] sm:items-end">
        <Field label="Berkas CV" required hint={`PDF, DOCX, DOC, JPG, PNG, atau WebP. Maksimal ${MAX_MB} MB dan 20 halaman. Foto CV boleh, pastikan tegak dan terang.`}>
          {({ id, describedBy }) => <input id={id} ref={fileRef} type="file" accept={ACCEPT} aria-describedby={describedBy} className="field-input py-2" />}
        </Field>
        <Button type="button" variant="primary" busy={busy} onClick={start}>
          Isi profil dari CV
        </Button>
      </div>
    </section>
  );
}
