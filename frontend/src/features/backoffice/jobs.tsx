import { useState } from "react";
import { Link } from "react-router";
import { ApiError, post, type Schemas } from "../../lib/api";
import { employmentLabel, fmtDate, statusLabel } from "../../lib/format";
import { useSession } from "../../lib/session";
import { useQuery } from "../../lib/useQuery";
import { Alert, Badge, Button, Dialog, Field, Input, PageHeader, PageState, Select, Textarea, useToast } from "../../ui";

type Job = Schemas["StaffJob"];

export function JobsPage() {
  const q = useQuery<{ items: Job[] }>("/staff/jobs");
  const org = useQuery<Schemas["PublicOrg"]>("/org");
  const { can } = useSession();
  const toast = useToast();
  const [confirm, setConfirm] = useState<{ job: Job; to: "publish" | "close" } | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<ApiError | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [form, setForm] = useState({ title: "", slug: "", unitCode: "", locationCode: "", employmentType: "full_time", description: "", requirements: "", workflowKey: "standar" });

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

  const create = async () => {
    setBusy(true);
    setErr(null);
    try {
      await post("/staff/jobs", form);
      toast("success", "Draft lowongan dibuat.");
      setCreateOpen(false);
      q.reload();
    } catch (e) {
      setErr(e as ApiError);
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
                  {j.unitName} · {j.locationName} · {employmentLabel[j.employmentType]} · {j.applicationCount} lamaran
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
              <Select id={id} invalid={invalid} value={form.unitCode} onChange={(e) => setForm({ ...form, unitCode: e.target.value })}>
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
          <Field label="Workflow" required error={fe.workflowKey}>
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
