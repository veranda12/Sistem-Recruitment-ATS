import { useState } from "react";
import { Link } from "react-router";
import { ApiError, get, post } from "../../lib/api";
import { fmtDate, fmtDateTime } from "../../lib/format";
import { useQuery } from "../../lib/useQuery";
import { Alert, Button, Dialog, Field, Input, PageHeader, PageState, Textarea, useToast } from "../../ui";

type Candidate = {
  id: string;
  name: string;
  email: string;
  createdAt: string;
  anonymizedAt: string | null;
  legalHoldAt: string | null;
  legalHoldReason: string;
  applications: number;
  activeApplications: number;
};
type Retention = { months: number; enabled: boolean; eligible: number; sample: { id: string; name: string; email: string; lastActivity: string }[] };

/** Permintaan subjek data (ekspor, anonimisasi, legal hold) dan pratinjau retensi. Hanya untuk scope seluruh organisasi. */
export function PrivacyPage() {
  const toast = useToast();
  const [email, setEmail] = useState("");
  const [result, setResult] = useState<Candidate[] | null>(null);
  const [err, setErr] = useState<ApiError | null>(null);
  const [busy, setBusy] = useState(false);
  const [dialog, setDialog] = useState<null | { kind: "anonymize" | "hold" | "release"; c: Candidate }>(null);
  const [reason, setReason] = useState("");
  const [confirm, setConfirm] = useState("");
  const retention = useQuery<Retention>("/admin/privacy/retention");

  const search = async (e?: React.FormEvent) => {
    e?.preventDefault();
    setErr(null);
    try {
      const r = await get<{ items: Candidate[] }>(`/admin/privacy/candidates?email=${encodeURIComponent(email.trim())}`);
      setResult(r.items);
    } catch (x) {
      setErr(x as ApiError);
      setResult(null);
    }
  };
  const submit = async () => {
    if (!dialog) return;
    setBusy(true);
    setErr(null);
    try {
      if (dialog.kind === "anonymize") {
        await post(`/admin/privacy/candidates/${dialog.c.id}/anonymize`, { reason, confirmEmail: confirm });
        toast("success", "Data kandidat dianonimkan.");
      } else {
        await post(`/admin/privacy/candidates/${dialog.c.id}/legal-hold`, { hold: dialog.kind === "hold", reason });
        toast("success", dialog.kind === "hold" ? "Legal hold dipasang." : "Legal hold dilepas.");
      }
      setDialog(null);
      await search();
      retention.reload();
    } catch (x) {
      setErr(x as ApiError);
    } finally {
      setBusy(false);
    }
  };
  const open = (kind: "anonymize" | "hold" | "release", c: Candidate) => {
    setErr(null);
    setReason("");
    setConfirm("");
    setDialog({ kind, c });
  };

  return (
    <div className="max-w-4xl">
      <PageHeader title="Privasi data" subtitle="Permintaan subjek data dan masa simpan data pelamar. Setiap tindakan dicatat di audit." />

      <section aria-label="Permintaan subjek data" className="mb-10">
        <h2 className="mb-1 text-[20px]">Permintaan subjek data</h2>
        <p className="mb-3 text-sm text-ink-2">Cari kandidat dengan email persis yang tercantum pada permintaan. Pastikan identitas pemohon sudah diverifikasi sesuai prosedur.</p>
        <form onSubmit={search} className="flex flex-wrap items-end gap-2">
          <Field label="Email kandidat" required>{({ id }) => <Input id={id} type="email" className="w-72 max-w-full" value={email} onChange={(e) => setEmail(e.target.value)} />}</Field>
          <Button type="submit" variant="primary">
            Cari
          </Button>
        </form>
        {err && !dialog ? (
          <div className="mt-3">
            <Alert tone="danger" title={Object.values(err.fieldErrors)[0] ?? err.message} />
          </div>
        ) : null}
        {result?.length === 0 ? <p className="mt-3 text-ink-2">Tidak ada kandidat dengan email tersebut.</p> : null}
        {result?.map((c) => (
          <div key={c.id} className="mt-4 rounded-md border border-line p-4">
            <p className="font-semibold">
              {c.name} <span className="font-normal text-ink-2">{c.email}</span>
            </p>
            <p className="text-sm text-ink-2">
              Terdaftar {fmtDate(c.createdAt)} · {c.applications} lamaran, {c.activeApplications} aktif
              {c.anonymizedAt ? ` · Dianonimkan ${fmtDateTime(c.anonymizedAt)}` : ""}
            </p>
            {c.legalHoldAt ? (
              <p className="mt-1 text-sm text-warning">
                Legal hold sejak {fmtDateTime(c.legalHoldAt)}: {c.legalHoldReason}
              </p>
            ) : null}
            {!c.anonymizedAt ? (
              <div className="mt-3 flex flex-wrap gap-2">
                <a className="inline-flex min-h-11 items-center rounded-md border border-line px-4 font-medium text-primary hover:bg-surface" href={`/api/v1/admin/privacy/candidates/${c.id}/export`}>
                  Unduh data (JSON)
                </a>
                {c.legalHoldAt ? <Button onClick={() => open("release", c)}>Lepas legal hold</Button> : <Button onClick={() => open("hold", c)}>Pasang legal hold</Button>}
                <Button variant="danger" onClick={() => open("anonymize", c)}>
                  Anonimkan data
                </Button>
              </div>
            ) : null}
          </div>
        ))}
      </section>

      <section aria-label="Retensi data pelamar">
        <h2 className="mb-1 text-[20px]">Retensi data pelamar</h2>
        <PageState loading={retention.loading} error={retention.error} onRetry={retention.reload}>
          {retention.data ? (
            <>
              <p className="mb-2 text-[15px]">
                Masa simpan {retention.data.months} bulan sejak aktivitas terakhir, untuk kandidat tanpa lamaran aktif dan tanpa legal hold.{" "}
                {retention.data.enabled ? (
                  <span className="font-semibold text-danger">Anonimisasi otomatis aktif.</span>
                ) : (
                  <span className="font-semibold">Anonimisasi otomatis mati</span>
                )}
                {retention.data.enabled ? "" : ": daftar di bawah hanya pratinjau."} Ubah di{" "}
                <Link className="text-primary underline" to="/staf/pengaturan">
                  Pengaturan
                </Link>
                .
              </p>
              <p className="mb-2 font-semibold">{retention.data.eligible} kandidat melewati masa simpan</p>
              {retention.data.sample.length ? (
                <ul className="divide-y divide-line border-y border-line text-sm">
                  {retention.data.sample.map((s) => (
                    <li key={s.id} className="flex flex-wrap justify-between gap-2 py-2">
                      <span>
                        {s.name} <span className="text-ink-2">{s.email}</span>
                      </span>
                      <span className="text-ink-2">Aktivitas terakhir {fmtDate(s.lastActivity)}</span>
                    </li>
                  ))}
                </ul>
              ) : null}
            </>
          ) : null}
        </PageState>
      </section>

      <Dialog
        open={!!dialog}
        onOpenChange={(o) => !o && setDialog(null)}
        title={dialog?.kind === "anonymize" ? `Anonimkan data ${dialog.c.name}?` : dialog?.kind === "hold" ? "Pasang legal hold" : "Lepas legal hold"}
        description={
          dialog?.kind === "anonymize"
            ? "Nama, kontak, NIK, profil, dokumen, hasil CV, dan isi pesan dihapus permanen. Riwayat proses dan nilai tetap tersimpan tanpa identitas. Tindakan ini tidak dapat dibatalkan."
            : "Legal hold mencegah anonimisasi, termasuk oleh retensi otomatis."
        }
      >
        {err && dialog ? <Alert tone="danger">{Object.values(err.fieldErrors)[0] ?? err.message}</Alert> : null}
        <div className="flex flex-col gap-3">
          <Field label={dialog?.kind === "anonymize" ? "Dasar penghapusan" : "Alasan"} required hint={dialog?.kind === "anonymize" ? "Misalnya nomor permintaan subjek data." : undefined}>
            {({ id, describedBy }) => <Textarea id={id} aria-describedby={describedBy} className="min-h-11" value={reason} onChange={(e) => setReason(e.target.value)} />}
          </Field>
          {dialog?.kind === "anonymize" ? (
            <Field label={`Ketik ulang email ${dialog.c.email}`} required>
              {({ id }) => <Input id={id} value={confirm} autoComplete="off" onChange={(e) => setConfirm(e.target.value)} />}
            </Field>
          ) : null}
        </div>
        <div className="mt-4 flex justify-end gap-2">
          <Button onClick={() => setDialog(null)}>Batal</Button>
          <Button variant={dialog?.kind === "anonymize" ? "danger" : "primary"} busy={busy} onClick={submit}>
            {dialog?.kind === "anonymize" ? "Anonimkan permanen" : "Simpan"}
          </Button>
        </div>
      </Dialog>
    </div>
  );
}
