import { useState } from "react";
import { Link } from "react-router";
import { ApiError, patch, post, type Schemas } from "../../lib/api";
import { employmentLabel, fmtDate, statusLabel, trackLabel } from "../../lib/format";
import { useSession } from "../../lib/session";
import { useQuery } from "../../lib/useQuery";
import { Alert, Badge, Button, Dialog, Field, Input, PageHeader, PageState, Select, Textarea, useToast } from "../../ui";

type Job = Schemas["StaffJob"];
// Permintaan tenaga kerja dari GET /requisitions. Hanya yang berstatus approved yang bisa dipakai untuk lowongan.
type Requisition = { id: string; title: string; unitName: string; headcount: number; status: string; track: string };

// Kunci workflow template per track (seed). Memilih track mengisi kunci ini; bisa diubah manual.
const workflowOf: Record<string, string> = { OPERATOR: "track-operator", STAFF: "track-staff", MT_S1_D4: "track-mt-s1-d4" };

const emptyForm = {
  title: "",
  slug: "",
  unitCode: "",
  locationCode: "",
  employmentType: "full_time",
  description: "",
  requirements: "",
  workflowKey: "standar",
  track: "",
  quota: "1",
  requisitionId: "",
};

export function JobsPage() {
  const q = useQuery<{ items: Job[] }>("/staff/jobs");
  const org = useQuery<Schemas["PublicOrg"]>("/org");
  const { can } = useSession();
  const toast = useToast();
  const [confirm, setConfirm] = useState<{ job: Job; to: "publish" | "close" } | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<ApiError | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [form, setForm] = useState(emptyForm);
  const [kuotaJob, setKuotaJob] = useState<Job | null>(null);
  const [kuota, setKuota] = useState("");
  const [kuotaErr, setKuotaErr] = useState<ApiError | null>(null);
  // Permintaan tenaga kerja yang disetujui untuk unit terpilih. Pencocokan unit dilakukan server lewat unitCode.
  const reqs = useQuery<{ items: Requisition[] }>(createOpen && can("jobs.manage") && form.unitCode ? `/requisitions?unitCode=${encodeURIComponent(form.unitCode)}` : null);
  const disetujui = (reqs.data?.items ?? []).filter((r) => r.status === "approved");

  const change = async () => {
    if (!confirm) return;
    setBusy(true);
    setErr(null);
    try {
      await post(`/staff/jobs/${confirm.job.id}/${confirm.to}`, { version: confirm.job.version });
      toast("success", confirm.to === "publish" ? "Lowongan diterbitkan." : "Lowongan ditutup.");
      setConfirm(null);
      q.reload();
    } catch (e) {
      setErr(e as ApiError);
    } finally {
      setBusy(false);
    }
  };

  const pilihTrack = (track: string) => setForm((f) => ({ ...f, track, workflowKey: workflowOf[track] ?? f.workflowKey }));

  // Memilih permintaan mewarisi track-nya; mengosongkan pilihan melepas track warisan.
  const pilihRequisition = (id: string) => {
    const r = reqs.data?.items.find((x) => x.id === id);
    setForm((f) => ({ ...f, requisitionId: id, track: r?.track ?? "", workflowKey: r ? (workflowOf[r.track] ?? f.workflowKey) : f.workflowKey }));
  };

  // Permintaan hanya bisa dipilih setelah unit, jadi mengganti unit membuang permintaan yang dipilih beserta track warisannya.
  // Track yang dipilih manual tidak ikut berubah.
  const gantiUnit = (unitCode: string) =>
    setForm((f) => {
      if (unitCode === f.unitCode) return f;
      return f.requisitionId ? { ...f, unitCode, requisitionId: "", track: "" } : { ...f, unitCode };
    });

  const create = async () => {
    setBusy(true);
    setErr(null);
    try {
      // Track tidak dikirim saat lowongan memakai permintaan: server mewarisi track permintaan tersebut.
      await post("/staff/jobs", {
        ...form,
        quota: Number(form.quota),
        requisitionId: form.requisitionId || null,
        track: form.requisitionId ? undefined : form.track,
      });
      toast("success", "Draft lowongan dibuat.");
      setForm(emptyForm);
      setCreateOpen(false);
      q.reload();
    } catch (e) {
      setErr(e as ApiError);
    } finally {
      setBusy(false);
    }
  };

  const simpanKuota = async () => {
    if (!kuotaJob) return;
    setBusy(true);
    setKuotaErr(null);
    try {
      await patch(`/staff/jobs/${kuotaJob.id}`, { expectedVersion: kuotaJob.version, quota: Number(kuota) });
      toast("success", "Kuota lowongan diperbarui.");
      setKuotaJob(null);
      q.reload();
    } catch (e) {
      setKuotaErr(e as ApiError);
    } finally {
      setBusy(false);
    }
  };

  const fe = err?.fieldErrors ?? {};

  return (
    <div>
      <PageHeader
        title="Lowongan"
        actions={
          can("jobs.manage") ? (
            <Button
              variant="primary"
              onClick={() => {
                setErr(null);
                setForm(emptyForm);
                setCreateOpen(true);
              }}
            >
              Buat lowongan
            </Button>
          ) : null
        }
      />
      <PageState loading={q.loading} error={q.error} onRetry={q.reload} empty={q.data?.items.length === 0} emptyText="Belum ada lowongan dalam akses Anda.">
        <ul className="divide-y divide-line border-y border-line">
          {q.data?.items.map((j) => (
            <li key={j.id} className="flex flex-wrap items-center justify-between gap-3 py-4">
              <div>
                <p className="text-[17px] font-semibold">{j.title}</p>
                <p className="text-sm text-ink-2">
                  {j.unitName} · {j.locationName} · {employmentLabel[j.employmentType]} · Track {trackLabel[j.track]} · kuota {j.quota} · {j.applicationCount} lamaran
                  {j.closesAt ? ` · batas ${fmtDate(j.closesAt)}` : ""}
                </p>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <Badge status={j.status}>{statusLabel[j.status]}</Badge>
                {j.status === "published" ? (
                  <Link className="text-sm text-primary underline" to={`/lowongan/${j.slug}`}>
                    Lihat di portal
                  </Link>
                ) : null}
                {can("jobs.manage") && j.status !== "closed" ? (
                  <Button
                    size="sm"
                    onClick={() => {
                      setKuotaErr(null);
                      setKuota(String(j.quota));
                      setKuotaJob(j);
                    }}
                  >
                    Ubah kuota
                  </Button>
                ) : null}
                {can("jobs.publish") && j.status === "draft" ? (
                  <Button size="sm" onClick={() => setConfirm({ job: j, to: "publish" })}>
                    Terbitkan
                  </Button>
                ) : null}
                {can("jobs.publish") && j.status === "published" ? (
                  <Button size="sm" onClick={() => setConfirm({ job: j, to: "close" })}>
                    Tutup
                  </Button>
                ) : null}
              </div>
            </li>
          ))}
        </ul>
      </PageState>

      <Dialog
        open={!!confirm}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={confirm?.to === "publish" ? "Terbitkan lowongan?" : "Tutup lowongan?"}
        description={confirm?.to === "publish" ? "Lowongan akan tampil di portal karier." : "Kandidat tidak dapat melamar lagi. Lamaran yang ada tetap diproses."}
      >
        {err ? <Alert tone="danger">{err.message}</Alert> : null}
        <div className="mt-4 flex justify-end gap-2">
          <Button onClick={() => setConfirm(null)}>Batal</Button>
          <Button variant="primary" busy={busy} onClick={change}>
            {confirm?.to === "publish" ? "Terbitkan" : "Tutup lowongan"}
          </Button>
        </div>
      </Dialog>

      <Dialog open={!!kuotaJob} onOpenChange={(o) => !o && setKuotaJob(null)} title="Ubah kuota" description={kuotaJob ? `${kuotaJob.title} · Track ${trackLabel[kuotaJob.track]}` : undefined}>
        {kuotaErr ? <Alert tone="danger">{kuotaErr.fieldErrors?.quota ?? kuotaErr.message}</Alert> : null}
        <div className="mt-3 flex flex-col gap-3">
          <Field label="Kuota" required error={kuotaErr?.fieldErrors?.quota} hint="Total kuota lowongan aktif dalam satu permintaan tidak boleh melebihi kebutuhan.">
            {({ id, invalid }) => <Input id={id} invalid={invalid} inputMode="numeric" value={kuota} onChange={(e) => setKuota(e.target.value)} />}
          </Field>
        </div>
        <div className="mt-4 flex justify-end gap-2">
          <Button onClick={() => setKuotaJob(null)}>Batal</Button>
          <Button variant="primary" busy={busy} onClick={simpanKuota}>
            Simpan kuota
          </Button>
        </div>
      </Dialog>

      <Dialog open={createOpen} onOpenChange={setCreateOpen} title="Buat lowongan" description="Lowongan dibuat sebagai draft dan baru tampil setelah diterbitkan.">
        <div className="flex flex-col gap-3">
          {err && !Object.keys(fe).length ? <Alert tone="danger">{err.message}</Alert> : null}
          <Field label="Judul" required error={fe.title}>
            {({ id, invalid }) => <Input id={id} invalid={invalid} value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} />}
          </Field>
          <Field label="Slug URL" required hint="Huruf kecil, angka, tanda hubung." error={fe.slug}>
            {({ id, invalid, describedBy }) => <Input id={id} aria-describedby={describedBy} invalid={invalid} value={form.slug} onChange={(e) => setForm({ ...form, slug: e.target.value })} />}
          </Field>
          <Field label="Unit" required error={fe.unitCode}>
            {({ id, invalid }) => (
              <Select id={id} invalid={invalid} value={form.unitCode} onChange={(e) => gantiUnit(e.target.value)}>
                <option value="">Pilih unit</option>
                {org.data?.units.map((u) => (
                  <option key={u.code} value={u.code}>
                    {u.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="Lokasi" required error={fe.locationCode}>
            {({ id, invalid }) => (
              <Select id={id} invalid={invalid} value={form.locationCode} onChange={(e) => setForm({ ...form, locationCode: e.target.value })}>
                <option value="">Pilih lokasi</option>
                {org.data?.locations.map((l) => (
                  <option key={l.code} value={l.code}>
                    {l.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="Permintaan tenaga kerja" hint={form.unitCode ? "Opsional. Hanya permintaan yang sudah disetujui di unit yang dipilih." : "Pilih unit dulu."} error={fe.requisitionId}>
            {({ id, invalid }) => (
              <Select id={id} invalid={invalid} disabled={!form.unitCode} value={form.requisitionId} onChange={(e) => pilihRequisition(e.target.value)}>
                <option value="">Tanpa permintaan</option>
                {disetujui.map((r) => (
                  <option key={r.id} value={r.id}>
                    {`${r.title} · ${r.headcount} orang · ${trackLabel[r.track]}`}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="Track" required={!form.requisitionId} error={fe.track} hint={form.requisitionId ? "Diwarisi dari permintaan tenaga kerja." : undefined}>
            {({ id, invalid }) => (
              <Select id={id} invalid={invalid} disabled={!!form.requisitionId} value={form.track} onChange={(e) => pilihTrack(e.target.value)}>
                <option value="">Pilih track</option>
                {Object.entries(trackLabel).map(([k, v]) => (
                  <option key={k} value={k}>
                    {v}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="Kuota" required error={fe.quota} hint="Jumlah posisi untuk lowongan ini.">
            {({ id, invalid }) => <Input id={id} invalid={invalid} inputMode="numeric" value={form.quota} onChange={(e) => setForm({ ...form, quota: e.target.value })} />}
          </Field>
          <Field label="Jenis kerja" required error={fe.employmentType}>
            {({ id }) => (
              <Select id={id} value={form.employmentType} onChange={(e) => setForm({ ...form, employmentType: e.target.value })}>
                {Object.entries(employmentLabel).map(([k, v]) => (
                  <option key={k} value={k}>
                    {v}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="Deskripsi" required error={fe.description}>
            {({ id }) => <Textarea id={id} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />}
          </Field>
          <Field label="Kualifikasi" required>
            {({ id }) => <Textarea id={id} value={form.requirements} onChange={(e) => setForm({ ...form, requirements: e.target.value })} />}
          </Field>
          <Field label="Workflow" required error={fe.workflowKey} hint="Terisi otomatis dari track: track-operator, track-staff, atau track-mt-s1-d4.">
            {({ id, invalid }) => <Input id={id} invalid={invalid} value={form.workflowKey} onChange={(e) => setForm({ ...form, workflowKey: e.target.value })} />}
          </Field>
        </div>
        <div className="mt-4 flex justify-end gap-2">
          <Button onClick={() => setCreateOpen(false)}>Batal</Button>
          <Button variant="primary" busy={busy} onClick={create}>
            Simpan draft
          </Button>
        </div>
      </Dialog>
    </div>
  );
}
