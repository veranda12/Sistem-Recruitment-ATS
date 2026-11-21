import { useState } from "react";
import { ApiError, post, put } from "../../lib/api";
import { fmtDateTime } from "../../lib/format";
import { useSession } from "../../lib/session";
import { useQuery } from "../../lib/useQuery";
import { Alert, Badge, Button, Dialog, Field, Input, PageHeader, PageState, Select, Textarea, useToast } from "../../ui";

type Assignment = { id: string; roleKey: string; roleName: string; unitName: string | null; jobTitle: string | null };
type User = { id: string; email: string; displayName: string; status: string; lastActivity: string; assignments: Assignment[] };
type Role = { key: string; name: string; permissions: string[] };

export function UsersPage() {
  const q = useQuery<{ items: User[] }>("/admin/users");
  const roles = useQuery<{ items: Role[]; catalog: Record<string, string> }>("/admin/roles");
  const org = useQuery<{ units: { code: string; name: string; active: boolean }[] }>("/admin/organization");
  const toast = useToast();
  const [err, setErr] = useState<ApiError | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [nu, setNu] = useState({ email: "", name: "" });
  const [assignFor, setAssignFor] = useState<User | null>(null);
  const [asg, setAsg] = useState({ roleKey: "recruiter", unitCode: "", reason: "" });
  const [busy, setBusy] = useState(false);

  const run = async (fn: () => Promise<unknown>, ok: string, after?: () => void) => {
    setBusy(true);
    setErr(null);
    try {
      await fn();
      toast("success", ok);
      after?.();
      q.reload();
    } catch (e) {
      setErr(e as ApiError);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="max-w-5xl">
      <PageHeader title="Pengguna dan peran" subtitle="Perubahan peran berlaku pada permintaan berikutnya pengguna tersebut." actions={<Button variant="primary" onClick={() => setCreateOpen(true)}>Tambah pengguna</Button>} />
      {err && !createOpen && !assignFor ? (
        <div className="mb-4">
          <Alert tone="danger">{err.message}</Alert>
        </div>
      ) : null}
      <PageState loading={q.loading} error={q.error} onRetry={q.reload}>
        <ul className="divide-y divide-line border-y border-line">
          {q.data?.items.map((u) => (
            <li key={u.id} className="py-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <p className="font-semibold">
                    {u.displayName} <span className="font-normal text-ink-2">{u.email}</span>
                  </p>
                  <p className="text-sm text-ink-2">Aktivitas terakhir {fmtDateTime(u.lastActivity)}</p>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <Badge status={u.status === "active" ? "published" : "closed"}>{u.status === "active" ? "Aktif" : "Nonaktif"}</Badge>
                  <Button size="sm" onClick={() => { setErr(null); setAssignFor(u); }}>
                    Beri peran
                  </Button>
                  <Button
                    size="sm"
                    onClick={() => run(() => post(`/admin/users/${u.id}/status`, { status: u.status === "active" ? "disabled" : "active" }), u.status === "active" ? "Pengguna dinonaktifkan." : "Pengguna diaktifkan.")}
                  >
                    {u.status === "active" ? "Nonaktifkan" : "Aktifkan"}
                  </Button>
                </div>
              </div>
              <ul className="mt-2 flex flex-wrap gap-2">
                {u.assignments.map((a) => (
                  <li key={a.id} className="flex items-center gap-1 rounded-sm border border-line px-2 py-1 text-sm">
                    {a.roleName}
                    {a.unitName ? ` · ${a.unitName}` : a.jobTitle ? ` · ${a.jobTitle}` : " · seluruh organisasi"}
                    <button
                      type="button"
                      className="ml-1 min-h-8 px-1 text-danger underline"
                      aria-label={`Cabut ${a.roleName} dari ${u.displayName}`}
                      onClick={() => run(() => post(`/admin/assignments/${a.id}/revoke`, { reason: "Dicabut melalui halaman pengguna" }), "Peran dicabut.")}
                    >
                      Cabut
                    </button>
                  </li>
                ))}
                {u.assignments.length === 0 ? <li className="text-sm text-ink-2">Belum memiliki peran.</li> : null}
              </ul>
            </li>
          ))}
        </ul>
      </PageState>

      {roles.data ? (
        <details className="mt-6">
          <summary className="cursor-pointer font-semibold">Matriks peran dan izin</summary>
          <div className="mt-3 overflow-x-auto">
            <table className="text-left text-sm">
              <thead>
                <tr>
                  <th scope="col" className="py-1 pr-3">Izin</th>
                  {roles.data.items.map((r) => (
                    <th key={r.key} scope="col" className="px-2 py-1">
                      {r.name}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {Object.entries(roles.data.catalog).map(([perm, desc]) => (
                  <tr key={perm} className="border-t border-line">
                    <th scope="row" className="py-1 pr-3 font-normal">
                      {desc}
                    </th>
                    {roles.data!.items.map((r) => (
                      <td key={r.key} className="px-2 py-1 text-center">
                        {r.permissions.includes(perm) ? "Ya" : "-"}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </details>
      ) : null}

      <Dialog open={createOpen} onOpenChange={setCreateOpen} title="Tambah pengguna internal" description="Pengguna menerima email untuk mengatur kata sandi sendiri. Tidak ada sandi bawaan.">
        {err && createOpen ? <Alert tone="danger">{err.fieldErrors.email ?? err.message}</Alert> : null}
        <div className="flex flex-col gap-3">
          <Field label="Nama" required>{({ id }) => <Input id={id} value={nu.name} onChange={(e) => setNu({ ...nu, name: e.target.value })} />}</Field>
          <Field label="Email" required>{({ id }) => <Input id={id} type="email" value={nu.email} onChange={(e) => setNu({ ...nu, email: e.target.value })} />}</Field>
        </div>
        <div className="mt-4 flex justify-end gap-2">
          <Button onClick={() => setCreateOpen(false)}>Batal</Button>
          <Button variant="primary" busy={busy} onClick={() => run(() => post("/admin/users", nu), "Pengguna dibuat dan undangan dikirim.", () => setCreateOpen(false))}>
            Simpan
          </Button>
        </div>
      </Dialog>

      <Dialog open={!!assignFor} onOpenChange={(o) => !o && setAssignFor(null)} title={`Beri peran: ${assignFor?.displayName ?? ""}`}>
        {err && assignFor ? <Alert tone="danger">{err.message}</Alert> : null}
        <div className="flex flex-col gap-3">
          <Field label="Peran" required>
            {({ id }) => (
              <Select id={id} value={asg.roleKey} onChange={(e) => setAsg({ ...asg, roleKey: e.target.value })}>
                {roles.data?.items.map((r) => (
                  <option key={r.key} value={r.key}>
                    {r.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="Unit" hint="Kosongkan untuk seluruh organisasi.">
            {({ id, describedBy }) => (
              <Select id={id} aria-describedby={describedBy} value={asg.unitCode} onChange={(e) => setAsg({ ...asg, unitCode: e.target.value })}>
                <option value="">Seluruh organisasi</option>
                {org.data?.units.filter((u) => u.active).map((u) => (
                  <option key={u.code} value={u.code}>
                    {u.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="Alasan">{({ id }) => <Textarea id={id} className="min-h-11" value={asg.reason} onChange={(e) => setAsg({ ...asg, reason: e.target.value })} />}</Field>
        </div>
        <div className="mt-4 flex justify-end gap-2">
          <Button onClick={() => setAssignFor(null)}>Batal</Button>
          <Button variant="primary" busy={busy} onClick={() => run(() => post(`/admin/users/${assignFor!.id}/assignments`, asg), "Peran diberikan.", () => setAssignFor(null))}>
            Simpan
          </Button>
        </div>
      </Dialog>
    </div>
  );
}

type Setting = { key: string; label: string; value: unknown; version: number; effectiveAt: string | null };
type Org = { organization: { name: string; version: number; profile: { tagline?: string; about?: string }; brand: { displayName?: string; primaryColor?: string } } };

export function SettingsPage() {
  const q = useQuery<{ items: Setting[] }>("/admin/settings");
  const org = useQuery<Org>("/admin/organization");
  const toast = useToast();
  const [edit, setEdit] = useState<Setting | null>(null);
  const [val, setVal] = useState("");
  const [reason, setReason] = useState("");
  const [eff, setEff] = useState("");
  const [err, setErr] = useState<ApiError | null>(null);
  const [o, setO] = useState<{ name: string; tagline: string; about: string; displayName: string; primaryColor: string } | null>(null);

  const save = async () => {
    if (!edit) return;
    setErr(null);
    try {
      await put(`/admin/settings/${edit.key}`, { value: Number(val), expectedVersion: edit.version, reason, effectiveAt: eff ? new Date(eff).toISOString() : undefined });
      toast("success", "Pengaturan disimpan.");
      setEdit(null);
      q.reload();
    } catch (e) {
      setErr(e as ApiError);
    }
  };
  const saveOrg = async () => {
    if (!o || !org.data) return;
    setErr(null);
    try {
      await put("/admin/organization", {
        name: o.name,
        version: org.data.organization.version,
        profile: { tagline: o.tagline, about: o.about },
        brand: { displayName: o.displayName, primaryColor: o.primaryColor },
      });
      toast("success", "Profil organisasi disimpan dan tampil di portal.");
      setO(null);
      org.reload();
    } catch (e) {
      setErr(e as ApiError);
    }
  };
  const od = org.data?.organization;

  return (
    <div className="max-w-4xl">
      <PageHeader title="Pengaturan" />
      {err && !edit ? (
        <div className="mb-4">
          <Alert tone="danger">{Object.values(err.fieldErrors)[0] ?? err.message}</Alert>
        </div>
      ) : null}
      <section className="mb-10">
        <h2 className="mb-3 text-[20px]">Profil organisasi dan brand</h2>
        {od && !o ? (
          <div>
            <p>
              {od.name} · {od.brand.displayName}
            </p>
            <p className="text-sm text-ink-2">{od.profile.tagline}</p>
            <Button className="mt-2" onClick={() => setO({ name: od.name, tagline: od.profile.tagline ?? "", about: od.profile.about ?? "", displayName: od.brand.displayName ?? "", primaryColor: od.brand.primaryColor ?? "#1557B0" })}>
              Ubah profil
            </Button>
          </div>
        ) : null}
        {o ? (
          <div className="flex flex-col gap-3">
            <Field label="Nama organisasi" required>{({ id }) => <Input id={id} value={o.name} onChange={(e) => setO({ ...o, name: e.target.value })} />}</Field>
            <Field label="Nama tampilan portal" required>{({ id }) => <Input id={id} value={o.displayName} onChange={(e) => setO({ ...o, displayName: e.target.value })} />}</Field>
            <Field label="Tagline">{({ id }) => <Input id={id} value={o.tagline} onChange={(e) => setO({ ...o, tagline: e.target.value })} />}</Field>
            <Field label="Tentang perusahaan">{({ id }) => <Textarea id={id} value={o.about} onChange={(e) => setO({ ...o, about: e.target.value })} />}</Field>
            <Field label="Warna utama" required hint="Format #RRGGBB.">
              {({ id, describedBy }) => <Input id={id} aria-describedby={describedBy} value={o.primaryColor} onChange={(e) => setO({ ...o, primaryColor: e.target.value })} />}
            </Field>
            <div className="flex gap-2">
              <Button variant="primary" onClick={saveOrg}>
                Simpan profil
              </Button>
              <Button onClick={() => setO(null)}>Batal</Button>
            </div>
          </div>
        ) : null}
      </section>
      <section>
        <h2 className="mb-3 text-[20px]">Kebijakan</h2>
        <p className="mb-3 text-sm text-ink-2">Nilai bertanda "belum ditetapkan" menunggu keputusan pemilik kebijakan. Setiap perubahan dicatat beserta versi, waktu efektif, dan alasan.</p>
        <PageState loading={q.loading} error={q.error} onRetry={q.reload}>
          <ul className="divide-y divide-line border-y border-line">
            {q.data?.items.map((s) => (
              <li key={s.key} className="flex flex-wrap items-center justify-between gap-2 py-3">
                <span>
                  {s.label}: <strong>{s.value === null ? "belum ditetapkan" : String(s.value)}</strong>
                  {s.version ? <span className="text-sm text-ink-2"> · versi {s.version}, berlaku {fmtDateTime(s.effectiveAt)}</span> : null}
                </span>
                <Button size="sm" onClick={() => { setErr(null); setVal(s.value === null ? "" : String(s.value)); setReason(""); setEff(""); setEdit(s); }}>
                  Ubah
                </Button>
              </li>
            ))}
          </ul>
        </PageState>
      </section>
      <Dialog open={!!edit} onOpenChange={(x) => !x && setEdit(null)} title={edit?.label ?? ""}>
        {err && edit ? <Alert tone="danger">{Object.values(err.fieldErrors)[0] ?? err.message}</Alert> : null}
        <div className="flex flex-col gap-3">
          <Field label="Nilai" required>{({ id }) => <Input id={id} inputMode="numeric" value={val} onChange={(e) => setVal(e.target.value)} />}</Field>
          <Field label="Berlaku mulai" hint="Kosongkan untuk berlaku sekarang.">{({ id, describedBy }) => <Input id={id} aria-describedby={describedBy} type="datetime-local" value={eff} onChange={(e) => setEff(e.target.value)} />}</Field>
          <Field label="Alasan perubahan" required>{({ id }) => <Textarea id={id} className="min-h-11" value={reason} onChange={(e) => setReason(e.target.value)} />}</Field>
        </div>
        <div className="mt-4 flex justify-end gap-2">
          <Button onClick={() => setEdit(null)}>Batal</Button>
          <Button variant="primary" onClick={save}>
            Simpan
          </Button>
        </div>
      </Dialog>
    </div>
  );
}

type AuditRow = { id: number; actorName: string | null; actorKind: string; action: string; targetType: string; targetId: string | null; outcome: string; reason: string | null; requestId: string | null; createdAt: string };

export function AuditPage() {
  const [action, setAction] = useState("");
  const [applied, setApplied] = useState("");
  const q = useQuery<{ items: AuditRow[]; nextCursor: string | null }>(`/admin/audit${applied ? `?action=${encodeURIComponent(applied)}` : ""}`);
  return (
    <div className="max-w-6xl">
      <PageHeader title="Audit" subtitle="Catatan read-only: pelaku, tindakan, target, waktu, hasil, dan kode permintaan." />
      <form className="mb-4 flex flex-wrap items-end gap-2" onSubmit={(e) => { e.preventDefault(); setApplied(action.trim()); }}>
        <Field label="Awalan tindakan" hint="Contoh: application., identity.reveal, admin.">
          {({ id, describedBy }) => <Input id={id} aria-describedby={describedBy} value={action} onChange={(e) => setAction(e.target.value)} />}
        </Field>
        <Button type="submit">Saring</Button>
      </form>
      <PageState loading={q.loading} error={q.error} onRetry={q.reload} empty={q.data?.items.length === 0} emptyText="Tidak ada catatan audit yang cocok.">
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-left text-sm">
            <caption className="sr-only">Catatan audit</caption>
            <thead>
              <tr className="border-b border-line text-ink-2">
                <th scope="col" className="py-2 pr-3">Waktu</th>
                <th scope="col" className="py-2 pr-3">Pelaku</th>
                <th scope="col" className="py-2 pr-3">Tindakan</th>
                <th scope="col" className="py-2 pr-3">Target</th>
                <th scope="col" className="py-2 pr-3">Hasil</th>
                <th scope="col" className="py-2">Alasan</th>
              </tr>
            </thead>
            <tbody>
              {q.data?.items.map((a) => (
                <tr key={a.id} className="border-b border-line align-top">
                  <td className="py-2 pr-3 whitespace-nowrap">{fmtDateTime(a.createdAt)}</td>
                  <td className="py-2 pr-3">{a.actorName ?? (a.actorKind === "system" ? "Sistem" : a.actorKind)}</td>
                  <td className="py-2 pr-3">{a.action}</td>
                  <td className="py-2 pr-3 break-all">
                    {a.targetType} {a.targetId?.slice(0, 8)}
                  </td>
                  <td className="py-2 pr-3">{a.outcome}</td>
                  <td className="py-2">{a.reason}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </PageState>
    </div>
  );
}

type Delivery = { id: string; templateKey: string; toName: string | null; status: string; attempts: number; lastError: string | null; createdAt: string; sentAt: string | null };
type Dead = { id: string; topic: string; attempts: number; lastError: string | null; createdAt: string };

export function CommsPage() {
  const q = useQuery<{ deliveries: Delivery[]; dead: Dead[]; outbox: { pending: number; dead: number; oldestPendingSeconds: number } }>("/admin/deliveries");
  const toast = useToast();
  const { can } = useSession();
  const status: Record<string, string> = { pending: "Menunggu", sent: "Terkirim", failed: "Gagal, akan dicoba lagi", ambiguous: "Perlu rekonsiliasi" };
  return (
    <div className="max-w-6xl">
      <PageHeader title="Komunikasi dan antrean" actions={<Button onClick={q.reload}>Perbarui</Button>} />
      <PageState loading={q.loading} error={q.error} onRetry={q.reload}>
        {q.data ? (
          <>
            <p className="mb-4">
              Antrean: {q.data.outbox.pending} menunggu, {q.data.outbox.dead} gagal permanen, event tertua {Math.round(q.data.outbox.oldestPendingSeconds)} detik.
            </p>
            {q.data.dead.length ? (
              <section className="mb-8">
                <h2 className="mb-2 text-[18px]">Gagal permanen (dead-letter)</h2>
                <ul className="divide-y divide-line border-y border-line">
                  {q.data.dead.map((d) => (
                    <li key={d.id} className="flex flex-wrap items-center justify-between gap-2 py-2 text-sm">
                      <span>
                        {d.topic} · {d.attempts} percobaan · {d.lastError}
                      </span>
                      {can("settings.manage") ? (
                        <Button
                          size="sm"
                          onClick={async () => {
                            try {
                              await post(`/admin/outbox/${d.id}/replay`);
                              toast("success", "Event dimasukkan kembali ke antrean.");
                              q.reload();
                            } catch (e) {
                              toast("danger", (e as ApiError).message);
                            }
                          }}
                        >
                          Coba lagi
                        </Button>
                      ) : null}
                    </li>
                  ))}
                </ul>
              </section>
            ) : null}
            <h2 className="mb-2 text-[18px]">Riwayat pengiriman email</h2>
            {q.data.deliveries.length === 0 ? (
              <p className="text-ink-2">Belum ada pengiriman.</p>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full border-collapse text-left text-sm">
                  <thead>
                    <tr className="border-b border-line text-ink-2">
                      <th scope="col" className="py-2 pr-3">Waktu</th>
                      <th scope="col" className="py-2 pr-3">Penerima</th>
                      <th scope="col" className="py-2 pr-3">Template</th>
                      <th scope="col" className="py-2 pr-3">Status</th>
                      <th scope="col" className="py-2">Keterangan</th>
                    </tr>
                  </thead>
                  <tbody>
                    {q.data.deliveries.map((d) => (
                      <tr key={d.id} className="border-b border-line">
                        <td className="py-2 pr-3 whitespace-nowrap">{fmtDateTime(d.createdAt)}</td>
                        <td className="py-2 pr-3">{d.toName ?? "-"}</td>
                        <td className="py-2 pr-3">{d.templateKey}</td>
                        <td className="py-2 pr-3">{status[d.status] ?? d.status}</td>
                        <td className="py-2">
                          {d.attempts} percobaan{d.lastError ? ` · ${d.lastError}` : ""}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        ) : null}
      </PageState>
    </div>
  );
}
