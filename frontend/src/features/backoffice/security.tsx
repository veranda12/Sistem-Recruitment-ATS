import { useState } from "react";
import { ApiError, post } from "../../lib/api";
import { fmtDateTime } from "../../lib/format";
import { useSession } from "../../lib/session";
import { useQuery } from "../../lib/useQuery";
import { Alert, Button, Field, Input, PageHeader, PageState } from "../../ui";

type Status = { enabled: boolean; enabledAt: string | null; recoveryCodesLeft: number; required: boolean };

/** Aktivasi verifikasi dua langkah (TOTP). Rahasia dan kode pemulihan hanya ditampilkan sekali. */
export function SecurityPage({ forced = false }: { forced?: boolean }) {
  const q = useQuery<Status>("/auth/mfa");
  const { refresh } = useSession();
  const [setup, setSetup] = useState<{ secret: string; otpauthUri: string } | null>(null);
  const [code, setCode] = useState("");
  const [codes, setCodes] = useState<string[] | null>(null);
  const [err, setErr] = useState<ApiError | null>(null);
  const [busy, setBusy] = useState(false);

  const start = async () => {
    setErr(null);
    setBusy(true);
    try {
      setSetup(await post("/auth/mfa/setup"));
    } catch (e) {
      setErr(e as ApiError);
    } finally {
      setBusy(false);
    }
  };
  const enable = async () => {
    setErr(null);
    setBusy(true);
    try {
      const r = await post<{ recoveryCodes: string[] }>("/auth/mfa/enable", { code });
      setCodes(r.recoveryCodes);
      setSetup(null);
      q.reload();
    } catch (e) {
      setErr(e as ApiError);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="max-w-2xl">
      <PageHeader title="Keamanan akun" subtitle="Verifikasi dua langkah memakai aplikasi autentikator (TOTP), misalnya aplikasi autentikator di ponsel." />
      {forced ? (
        <div className="mb-4">
          <Alert tone="warning" title="Verifikasi dua langkah wajib untuk peran Anda">
            Aktifkan terlebih dahulu sebelum memakai back office.
          </Alert>
        </div>
      ) : null}
      {err ? (
        <div className="mb-4">
          <Alert tone="danger">{err.fieldErrors.code ?? err.message}</Alert>
        </div>
      ) : null}
      <PageState loading={q.loading} error={q.error} onRetry={q.reload}>
        {codes ? (
          <Alert tone="success" title="Verifikasi dua langkah aktif">
            <p>Simpan kode pemulihan berikut di tempat aman. Setiap kode hanya dapat dipakai sekali dan tidak akan ditampilkan lagi.</p>
            <ul className="mt-2 grid grid-cols-2 gap-1 font-semibold tracking-wide">
              {codes.map((c) => (
                <li key={c}>{c}</li>
              ))}
            </ul>
            <Button className="mt-3" variant="primary" onClick={() => refresh()}>
              Saya sudah menyimpan kode
            </Button>
          </Alert>
        ) : q.data?.enabled ? (
          <Alert tone="success" title="Verifikasi dua langkah aktif">
            Aktif sejak {fmtDateTime(q.data.enabledAt)}. Sisa kode pemulihan: {q.data.recoveryCodesLeft}. Bila perangkat hilang, minta admin pengguna mereset MFA Anda.
          </Alert>
        ) : setup ? (
          <div className="flex flex-col gap-4">
            <ol className="list-decimal pl-5">
              <li>Di aplikasi autentikator, pilih tambah akun lalu masukkan kunci secara manual.</li>
              <li>Masukkan kode 6 digit yang muncul untuk memastikan perangkat sudah benar.</li>
            </ol>
            <div>
              <p className="text-sm text-ink-2">Kunci rahasia</p>
              <p className="break-all text-[17px] font-semibold tracking-wider">{setup.secret.match(/.{1,4}/g)?.join(" ")}</p>
              <p className="mt-2 break-all text-[13px] text-ink-2">{setup.otpauthUri}</p>
            </div>
            <Field label="Kode 6 digit" required>
              {({ id }) => <Input id={id} inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={code} onChange={(e) => setCode(e.target.value)} />}
            </Field>
            <Button variant="primary" busy={busy} onClick={enable}>
              Aktifkan
            </Button>
          </div>
        ) : (
          <Button variant="primary" busy={busy} onClick={start}>
            Mulai aktivasi
          </Button>
        )}
      </PageState>
    </div>
  );
}
