import { useEffect, useState } from "react";
import { Link, Route, Routes, useNavigate, useParams, useSearchParams } from "react-router";
import { ApiError, get, newIdempotencyKey, post, type Schemas } from "../../lib/api";
import { employmentLabel, fmtDate, trackLabel } from "../../lib/format";
import { useSession } from "../../lib/session";
import { useQuery } from "../../lib/useQuery";
import { Alert, Badge, Button, Dialog, PageState, Select, Input } from "../../ui";

type JobList = { items: Schemas["PublicJob"][]; nextCursor: string | null };

export default function CareerRoutes() {
  return (
    <Routes>
      <Route index element={<Home />} />
      <Route path="lowongan/:slug" element={<JobDetail />} />
      <Route path="*" element={<Alert tone="warning" title="Halaman tidak ditemukan">Periksa kembali alamat yang Anda buka.</Alert>} />
    </Routes>
  );
}

function Home() {
  const org = useQuery<Schemas["PublicOrg"]>("/org");
  const [params, setParams] = useSearchParams();
  const filters = { unit: params.get("unit") ?? "", location: params.get("location") ?? "", type: params.get("type") ?? "", track: params.get("track") ?? "", q: params.get("q") ?? "" };
  const qs = new URLSearchParams(Object.entries(filters).filter(([, v]) => v)).toString();
  const [items, setItems] = useState<Schemas["PublicJob"][]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [more, setMore] = useState(false);
  const [error, setError] = useState<ApiError | null>(null);
  const [q, setQ] = useState(filters.q);

  useEffect(() => {
    const ctrl = new AbortController();
    setLoading(true);
    setError(null);
    get<JobList>(`/jobs?${qs}`, ctrl.signal)
      .then((d) => {
        setItems(d.items);
        setCursor(d.nextCursor);
      })
      .catch((e) => e.name !== "AbortError" && setError(e))
      .finally(() => setLoading(false));
    return () => ctrl.abort();
  }, [qs]);

  const loadMore = async () => {
    if (!cursor) return;
    setMore(true);
    try {
      const d = await get<JobList>(`/jobs?${qs}${qs ? "&" : ""}cursor=${encodeURIComponent(cursor)}`);
      setItems((xs) => [...xs, ...d.items]);
      setCursor(d.nextCursor);
    } catch (e) {
      setError(e as ApiError);
    } finally {
      setMore(false);
    }
  };

  const setFilter = (k: string, v: string) => {
    const next = new URLSearchParams(params);
    if (v) next.set(k, v);
    else next.delete(k);
    setParams(next);
  };
  const filtered = Object.values(filters).some(Boolean);

  return (
    <div>
      <section className="mb-8 border-b border-line pb-8">
        <h1 className="text-[28px] md:text-[32px]">Bergabung dengan {org.data?.name ?? "kami"}</h1>
        <p className="mt-2 max-w-3xl text-[16px] text-ink-2">{org.data?.profile.tagline}</p>
        {org.data?.profile.about ? <p className="mt-3 max-w-3xl">{org.data.profile.about}</p> : null}
      </section>

      <h2 className="mb-3 text-[22px]">Lowongan terbuka</h2>
      <form
        role="search"
        aria-label="Saring lowongan"
        className="mb-6 grid grid-cols-1 gap-3 rounded-lg border border-line bg-surface p-4 sm:grid-cols-2 lg:grid-cols-[2fr_1fr_1fr_1fr_1fr_auto]"
        onSubmit={(e) => {
          e.preventDefault();
          setFilter("q", q.trim());
        }}
      >
        <label className="flex flex-col gap-1 text-sm font-semibold">
          Kata kunci
          <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Contoh: operator" maxLength={100} />
        </label>
        <label className="flex flex-col gap-1 text-sm font-semibold">
          Unit
          <Select value={filters.unit} onChange={(e) => setFilter("unit", e.target.value)}>
            <option value="">Semua unit</option>
            {org.data?.units.map((u) => (
              <option key={u.code} value={u.code}>
                {u.name}
              </option>
            ))}
          </Select>
        </label>
        <label className="flex flex-col gap-1 text-sm font-semibold">
          Lokasi
          <Select value={filters.location} onChange={(e) => setFilter("location", e.target.value)}>
            <option value="">Semua lokasi</option>
            {org.data?.locations.map((l) => (
              <option key={l.code} value={l.code}>
                {l.city}
              </option>
            ))}
          </Select>
        </label>
        <label className="flex flex-col gap-1 text-sm font-semibold">
          Jenis kerja
          <Select value={filters.type} onChange={(e) => setFilter("type", e.target.value)}>
            <option value="">Semua jenis</option>
            {Object.entries(employmentLabel).map(([k, v]) => (
              <option key={k} value={k}>
                {v}
              </option>
            ))}
          </Select>
        </label>
        <label className="flex flex-col gap-1 text-sm font-semibold">
          Track
          <Select value={filters.track} onChange={(e) => setFilter("track", e.target.value)}>
            <option value="">Semua track</option>
            {Object.entries(trackLabel).map(([k, v]) => (
              <option key={k} value={k}>
                {v}
              </option>
            ))}
          </Select>
        </label>
        <div className="flex items-end gap-2">
          <Button type="submit" variant="primary">
            Cari
          </Button>
          {filtered ? (
            <Button
              type="button"
              variant="ghost"
              onClick={() => {
                setQ("");
                setParams(new URLSearchParams());
              }}
            >
              Hapus filter
            </Button>
          ) : null}
        </div>
      </form>

      <PageState
        loading={loading}
        error={error}
        empty={items.length === 0}
        emptyText={filtered ? "Tidak ada lowongan yang cocok dengan filter ini." : "Belum ada lowongan terbuka saat ini."}
      >
        <p className="mb-3 text-sm text-ink-2" aria-live="polite">
          Menampilkan {items.length} lowongan{cursor ? " (masih ada lagi)" : ""}
        </p>
        <ul className="divide-y divide-line border-y border-line">
          {items.map((j) => (
            <li key={j.id} className="py-4">
              <Link to={`/lowongan/${j.slug}`} className="text-[18px] font-semibold text-primary hover:underline">
                {j.title}
              </Link>
              <p className="mt-1 text-[15px] text-ink-2">
                {j.unit.name} · {j.location.city} · {employmentLabel[j.employmentType]} · Track {trackLabel[j.track]}
              </p>
              <p className="mt-1 text-sm text-ink-2">Batas lamaran: {j.closesAt ? fmtDate(j.closesAt) : "Sampai posisi terisi"}</p>
            </li>
          ))}
        </ul>
        {cursor ? (
          <div className="mt-4">
            <Button onClick={loadMore} busy={more}>
              Muat lebih banyak
            </Button>
          </div>
        ) : null}
      </PageState>
    </div>
  );
}

function JobDetail() {
  const { slug = "" } = useParams();
  const job = useQuery<Schemas["PublicJobDetail"]>(`/jobs/${encodeURIComponent(slug)}`);
  const { session } = useSession();
  const nav = useNavigate();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<ApiError | null>(null);
  const [idemKey] = useState(() => newIdempotencyKey("apply"));

  const startApply = () => {
    if (!session) {
      nav(`/masuk?next=${encodeURIComponent(`/lowongan/${slug}?lamar=1`)}`);
      return;
    }
    setErr(null);
    setOpen(true);
  };
  const [params] = useSearchParams();
  useEffect(() => {
    if (params.get("lamar") === "1" && session?.user.kind === "candidate" && job.data?.acceptingApplications) setOpen(true);
  }, [params, session, job.data]);

  const submit = async () => {
    setBusy(true);
    setErr(null);
    try {
      const r = await post<{ applicationId: string }>(`/jobs/${encodeURIComponent(slug)}/apply`, {}, { "Idempotency-Key": idemKey });
      nav(`/akun/lamaran/${r.applicationId}?baru=1`);
    } catch (e) {
      setErr(e as ApiError);
    } finally {
      setBusy(false);
    }
  };

  return (
    <PageState loading={job.loading} error={job.error} onRetry={job.reload}>
      {job.data ? (
        <article className="max-w-3xl">
          <Link to="/" className="text-sm text-primary hover:underline">
            Kembali ke daftar lowongan
          </Link>
          <h1 className="mt-2 text-[28px] md:text-[32px]">{job.data.title}</h1>
          <p className="mt-1 text-ink-2">
            {job.data.unit.name} · {job.data.location.name}, {job.data.location.city} · {employmentLabel[job.data.employmentType]} · Track {trackLabel[job.data.track]}
          </p>
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <Badge status={job.data.acceptingApplications ? "published" : "closed"}>{job.data.acceptingApplications ? "Menerima lamaran" : "Lowongan ditutup"}</Badge>
            <span className="text-sm text-ink-2">Batas lamaran: {job.data.closesAt ? fmtDate(job.data.closesAt) : "Sampai posisi terisi"}</span>
          </div>
          {!job.data.acceptingApplications ? (
            <div className="mt-4">
              <Alert tone="warning" title="Lowongan ini sudah ditutup">
                Lamaran baru tidak diterima. Lihat lowongan lain yang masih terbuka.
              </Alert>
            </div>
          ) : null}
          <section className="mt-6">
            <h2 className="text-[20px]">Deskripsi pekerjaan</h2>
            <p className="mt-2 whitespace-pre-line">{job.data.description}</p>
          </section>
          <section className="mt-6">
            <h2 className="text-[20px]">Kualifikasi</h2>
            <p className="mt-2 whitespace-pre-line">{job.data.requirements}</p>
          </section>
          {job.data.acceptingApplications && session?.user.kind !== "staff" ? (
            <div className="sticky bottom-0 mt-8 border-t border-line bg-white py-3">
              <Button variant="primary" onClick={startApply}>
                Lamar posisi ini
              </Button>
            </div>
          ) : null}
          <Dialog open={open} onOpenChange={setOpen} title={`Lamar ${job.data.title}`} description="Data profil Anda saat ini akan disimpan sebagai salinan lamaran.">
            {err ? <ApplyError err={err} /> : null}
            <div className="mt-4 flex flex-wrap justify-end gap-2">
              <Button onClick={() => setOpen(false)}>Batal</Button>
              {err?.code !== "profile_incomplete" && err?.code !== "already_applied" ? (
                <Button variant="primary" onClick={submit} busy={busy}>
                  Kirim lamaran
                </Button>
              ) : null}
            </div>
          </Dialog>
        </article>
      ) : null}
    </PageState>
  );
}

function ApplyError({ err }: { err: ApiError }) {
  const details = err.details as { applicationId?: string; missing?: string[] } | undefined;
  if (err.code === "profile_incomplete")
    return (
      <Alert tone="warning" title={err.message}>
        <Link className="font-semibold text-primary underline" to="/akun/profil">
          Lengkapi profil
        </Link>
      </Alert>
    );
  if (err.code === "already_applied" && details?.applicationId)
    return (
      <Alert tone="info" title={err.message}>
        <Link className="font-semibold text-primary underline" to={`/akun/lamaran/${details.applicationId}`}>
          Lihat lamaran Anda
        </Link>
      </Alert>
    );
  return <Alert tone="danger" title="Lamaran belum terkirim">{err.message}</Alert>;
}
