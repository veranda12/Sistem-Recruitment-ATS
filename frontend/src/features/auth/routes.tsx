import { useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import { Link, Route, Routes, useNavigate, useSearchParams } from "react-router";
import { ApiError, post } from "../../lib/api";
import { useSession } from "../../lib/session";
import { Alert, Button, Field, Input } from "../../ui";

export default function AuthRoutes() {
  return (
    <Routes>
      <Route path="masuk" element={<Login />} />
      <Route path="daftar" element={<Register />} />
      <Route path="verifikasi-email" element={<VerifyEmail />} />
      <Route path="lupa-sandi" element={<Forgot />} />
      <Route path="atur-ulang-sandi" element={<Reset />} />
    </Routes>
  );
}

function Card({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="mx-auto max-w-md">
      <h1 className="mb-6 text-[26px]">{title}</h1>
      {children}
    </div>
  );
}

/** Hanya path internal yang boleh menjadi tujuan kembali (cegah open redirect). */
function safeNext(next: string | null): string | null {
  return next && next.startsWith("/") && !next.startsWith("//") ? next : null;
}

function useForm() {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<ApiError | null>(null);
  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    setErr(null);
    try {
      await fn();
    } catch (e) {
      setErr(e as ApiError);
    } finally {
      setBusy(false);
    }
  };
  return { busy, err, run, fe: err?.fieldErrors ?? {} };
}

function Login() {
  const { login } = useSession();
  const nav = useNavigate();
  const [params] = useSearchParams();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const f = useForm();
  const needCode = f.err?.code === "mfa_required" || f.err?.code === "mfa_invalid" || code !== "";
  const errRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (f.err) errRef.current?.focus();
  }, [f.err]);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    f.run(async () => {
      const s = await login(email, password, code || undefined);
      nav(safeNext(params.get("next")) ?? (s.user.kind === "staff" ? "/staf" : "/akun"), { replace: true });
    });
  };
  return (
    <Card title="Masuk">
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        {f.err ? (
          <div ref={errRef} tabIndex={-1}>
            <Alert tone="danger" title="Tidak dapat masuk">
              {f.err.message}
            </Alert>
          </div>
        ) : null}
        <Field label="Email" required>
          {({ id, describedBy }) => <Input id={id} aria-describedby={describedBy} type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} required />}
        </Field>
        <Field label="Kata sandi" required>
          {({ id, describedBy }) => (
            <Input id={id} aria-describedby={describedBy} type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
          )}
        </Field>
        {needCode ? (
          <Field label="Kode verifikasi" required hint="Kode 6 digit dari aplikasi autentikator, atau kode pemulihan.">
            {({ id, describedBy }) => <Input id={id} aria-describedby={describedBy} autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} />}
          </Field>
        ) : null}
        <Button type="submit" variant="primary" busy={f.busy}>
          Masuk
        </Button>
        <div className="flex flex-wrap justify-between gap-2 text-sm">
          <Link to="/lupa-sandi" className="text-primary underline">
            Lupa kata sandi?
          </Link>
          <Link to={`/daftar${params.get("next") ? `?next=${encodeURIComponent(params.get("next")!)}` : ""}`} className="text-primary underline">
            Belum punya akun? Daftar
          </Link>
        </div>
      </form>
    </Card>
  );
}

function Register() {
  const [v, setV] = useState({ fullName: "", email: "", nik: "", password: "", acceptPrivacy: false });
  const [done, setDone] = useState(false);
  const f = useForm();
  const set = (k: keyof typeof v) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setV((s) => ({ ...s, [k]: e.target.type === "checkbox" ? e.target.checked : e.target.value }));

  if (done)
    return (
      <Card title="Periksa email Anda">
        <Alert tone="success" title="Pendaftaran diproses">
          Kami mengirim tautan verifikasi ke {v.email}. Buka tautan tersebut, lalu masuk untuk melengkapi profil.
        </Alert>
        <p className="mt-4">
          <Link to="/masuk" className="font-semibold text-primary underline">
            Ke halaman masuk
          </Link>
        </p>
      </Card>
    );
  return (
    <Card title="Buat akun kandidat">
      <p className="mb-4 text-ink-2">Satu akun untuk semua lamaran Anda. Profil diisi sekali dan dipakai untuk setiap lowongan.</p>
      <form
        noValidate
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          f.run(async () => {
            await post("/auth/register", v);
            setDone(true);
          });
        }}
      >
        {f.err && !Object.keys(f.fe).length ? <Alert tone="danger">{f.err.message}</Alert> : null}
        {Object.keys(f.fe).length ? <Alert tone="danger" title="Periksa kembali isian yang ditandai." /> : null}
        <Field label="Nama lengkap sesuai KTP" required error={f.fe.fullName}>
          {({ id, describedBy, invalid }) => <Input id={id} aria-describedby={describedBy} invalid={invalid} autoComplete="name" value={v.fullName} onChange={set("fullName")} />}
        </Field>
        <Field label="Email" required error={f.fe.email}>
          {({ id, describedBy, invalid }) => <Input id={id} aria-describedby={describedBy} invalid={invalid} type="email" autoComplete="email" value={v.email} onChange={set("email")} />}
        </Field>
        <Field label="NIK" required hint="16 digit sesuai KTP. Disimpan terenkripsi dan ditampilkan tersamar." error={f.fe.nik}>
          {({ id, describedBy, invalid }) => (
            <Input id={id} aria-describedby={describedBy} invalid={invalid} inputMode="numeric" autoComplete="off" maxLength={24} value={v.nik} onChange={set("nik")} />
          )}
        </Field>
        <Field label="Kata sandi" required hint="Minimal 10 karakter, memuat huruf dan angka." error={f.fe.password}>
          {({ id, describedBy, invalid }) => (
            <Input id={id} aria-describedby={describedBy} invalid={invalid} type="password" autoComplete="new-password" value={v.password} onChange={set("password")} />
          )}
        </Field>
        <div>
          <label className="flex min-h-11 items-start gap-3">
            <input type="checkbox" className="mt-1 h-5 w-5" checked={v.acceptPrivacy} onChange={set("acceptPrivacy")} aria-invalid={!!f.fe.acceptPrivacy || undefined} />
            <span>Saya telah membaca dan menyetujui pemberitahuan privasi pemrosesan data pelamar.</span>
          </label>
          {f.fe.acceptPrivacy ? <p className="text-[13px] font-medium text-danger">{f.fe.acceptPrivacy}</p> : null}
        </div>
        <Button type="submit" variant="primary" busy={f.busy}>
          Daftar
        </Button>
      </form>
    </Card>
  );
}

function VerifyEmail() {
  const [params] = useSearchParams();
  const token = params.get("token") ?? "";
  const [state, setState] = useState<"idle" | "ok" | "fail">("idle");
  const [msg, setMsg] = useState("");
  const { refresh } = useSession();
  const started = useRef(false);
  useEffect(() => {
    if (started.current) return;
    started.current = true;
    post("/auth/verify-email", { token })
      .then(() => {
        setState("ok");
        return refresh();
      })
      .catch((e: ApiError) => {
        setState("fail");
        setMsg(e.message);
      });
  }, [token, refresh]);
  return (
    <Card title="Verifikasi email">
      {state === "idle" ? <p role="status">Memverifikasi...</p> : null}
      {state === "ok" ? (
        <Alert tone="success" title="Email terverifikasi">
          <Link to="/akun/profil" className="font-semibold text-primary underline">
            Lanjut lengkapi profil
          </Link>
        </Alert>
      ) : null}
      {state === "fail" ? <Alert tone="danger" title="Verifikasi gagal">{msg}</Alert> : null}
    </Card>
  );
}

function Forgot() {
  const [email, setEmail] = useState("");
  const [msg, setMsg] = useState("");
  const f = useForm();
  return (
    <Card title="Lupa kata sandi">
      {msg ? <Alert tone="success">{msg}</Alert> : null}
      <form
        className="mt-4 flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          f.run(async () => setMsg((await post<{ message: string }>("/auth/forgot-password", { email })).message));
        }}
      >
        {f.err ? <Alert tone="danger">{f.err.message}</Alert> : null}
        <Field label="Email akun" required>
          {({ id }) => <Input id={id} type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} />}
        </Field>
        <Button type="submit" variant="primary" busy={f.busy}>
          Kirim tautan
        </Button>
      </form>
    </Card>
  );
}

function Reset() {
  const [params] = useSearchParams();
  const [password, setPassword] = useState("");
  const [done, setDone] = useState(false);
  const f = useForm();
  if (done)
    return (
      <Card title="Kata sandi diperbarui">
        <Alert tone="success">Semua sesi sebelumnya sudah dikeluarkan. Silakan masuk dengan kata sandi baru.</Alert>
        <Link to="/masuk" className="mt-4 inline-block font-semibold text-primary underline">
          Masuk
        </Link>
      </Card>
    );
  return (
    <Card title="Atur ulang kata sandi">
      <form
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          f.run(async () => {
            await post("/auth/reset-password", { token: params.get("token") ?? "", password });
            setDone(true);
          });
        }}
      >
        {f.err && !f.fe.password ? <Alert tone="danger">{f.err.message}</Alert> : null}
        <Field label="Kata sandi baru" required hint="Minimal 10 karakter, memuat huruf dan angka." error={f.fe.password}>
          {({ id, describedBy, invalid }) => (
            <Input id={id} aria-describedby={describedBy} invalid={invalid} type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} />
          )}
        </Field>
        <Button type="submit" variant="primary" busy={f.busy}>
          Simpan kata sandi
        </Button>
      </form>
    </Card>
  );
}
