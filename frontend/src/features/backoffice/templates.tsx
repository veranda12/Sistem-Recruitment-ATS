import { useEffect, useRef, useState } from "react";
import { ApiError, post, put, type Schemas } from "../../lib/api";
import { fmtDateTime } from "../../lib/format";
import { useQuery } from "../../lib/useQuery";
import { Alert, Button, Field, Input, PageHeader, PageState, cx, useToast } from "../../ui";

type Tpl = Schemas["MessageTemplate"];
type Version = { version: number; subject: string; body: string; isDefault: boolean; note: string; createdAt: string; authorName: string | null };

const channelLabel: Record<string, string> = { email: "Email", whatsapp: "WhatsApp" };

/** Editor template pesan: email dan WhatsApp, berversi, dengan pratinjau data contoh dan riwayat. */
export function TemplatesPage() {
  const list = useQuery<{ items: Tpl[] }>("/admin/message-templates");
  const [sel, setSel] = useState<string | null>(null);
  const items = list.data?.items ?? [];
  const current = items.find((t) => `${t.key}/${t.channel}` === sel) ?? items[0];

  return (
    <div className="max-w-6xl">
      <PageHeader title="Template pesan" subtitle="Ubah isi email dan WhatsApp yang dikirim ke kandidat. Setiap simpanan menjadi versi baru dan dicatat di audit." />
      <PageState loading={list.loading} error={list.error} onRetry={list.reload}>
        <div className="grid gap-6 lg:grid-cols-[18rem_1fr]">
          <nav aria-label="Daftar template">
            <ul className="flex flex-col gap-1">
              {items.map((t) => {
                const id = `${t.key}/${t.channel}`;
                const active = current && `${current.key}/${current.channel}` === id;
                return (
                  <li key={id}>
                    <button
                      type="button"
                      aria-current={active ? "true" : undefined}
                      onClick={() => setSel(id)}
                      className={cx("w-full rounded-md px-3 py-2 text-left", active ? "bg-primary-soft text-primary" : "hover:bg-surface")}
                    >
                      <span className="block font-semibold">{t.label}</span>
                      <span className="text-[13px] text-ink-2">
                        {channelLabel[t.channel]} · {t.customized ? `Diubah, versi ${t.version}` : "Bawaan"}
                      </span>
                    </button>
                  </li>
                );
              })}
            </ul>
          </nav>
          {current ? <Editor key={`${current.key}/${current.channel}/${current.version}`} tpl={current} onSaved={list.reload} /> : null}
        </div>
      </PageState>
    </div>
  );
}

function Editor({ tpl, onSaved }: { tpl: Tpl; onSaved: () => void }) {
  const toast = useToast();
  const [subject, setSubject] = useState(tpl.subject);
  const [body, setBody] = useState(tpl.body);
  const [note, setNote] = useState("");
  const [preview, setPreview] = useState<{ subject: string; body: string } | null>(null);
  const [previewErr, setPreviewErr] = useState<ApiError | null>(null);
  const [err, setErr] = useState<ApiError | null>(null);
  const [busy, setBusy] = useState(false);
  const bodyRef = useRef<HTMLTextAreaElement>(null);
  const path = `/admin/message-templates/${tpl.key}/${tpl.channel}`;
  const versions = useQuery<{ items: Version[] }>(`${path}/versions`);
  const isEmail = tpl.channel === "email";

  // Pratinjau server (validasi yang sama dengan saat menyimpan), ditunda sejenak saat mengetik.
  useEffect(() => {
    const t = setTimeout(() => {
      post<{ subject: string; body: string }>(`${path}/preview`, { subject, body })
        .then((p) => {
          setPreview(p);
          setPreviewErr(null);
        })
        .catch((e: ApiError) => setPreviewErr(e));
    }, 400);
    return () => clearTimeout(t);
  }, [path, subject, body]);

  const insert = (name: string) => {
    const el = bodyRef.current;
    const token = `{{${name}}}`;
    if (!el) return setBody((b) => b + token);
    const { selectionStart: s, selectionEnd: e } = el;
    setBody(body.slice(0, s) + token + body.slice(e));
    requestAnimationFrame(() => {
      el.focus();
      el.setSelectionRange(s + token.length, s + token.length);
    });
  };

  const save = async (extra: Record<string, unknown> = {}) => {
    setBusy(true);
    setErr(null);
    try {
      await put(path, { subject, body, note, expectedVersion: tpl.version, ...extra });
      toast("success", "Template tersimpan sebagai versi baru.");
      onSaved();
    } catch (e) {
      setErr(e as ApiError);
    } finally {
      setBusy(false);
    }
  };

  return (
    <section aria-label={`Ubah template ${tpl.label}`} className="min-w-0">
      <h2 className="text-[20px]">
        {tpl.label} <span className="text-[15px] font-normal text-ink-2">· {channelLabel[tpl.channel]}</span>
      </h2>
      <p className="mb-4 text-sm text-ink-2">Dikirim {tpl.when.charAt(0).toLowerCase() + tpl.when.slice(1)}.</p>
      {err ? (
        <div className="mb-3">
          <Alert tone="danger" title={err.message} />
        </div>
      ) : null}
      <div className="grid gap-6 xl:grid-cols-2">
        <div className="flex min-w-0 flex-col gap-3">
          {isEmail ? (
            <Field label="Subjek" required error={err?.fieldErrors.subject}>
              {({ id, invalid }) => <Input id={id} invalid={invalid} value={subject} maxLength={200} onChange={(e) => setSubject(e.target.value)} />}
            </Field>
          ) : null}
          <Field label="Isi pesan" required error={err?.fieldErrors.body}>
            {({ id, invalid }) => (
              <textarea
                id={id}
                ref={bodyRef}
                aria-invalid={invalid || undefined}
                className="field-input min-h-56"
                value={body}
                maxLength={5000}
                onChange={(e) => setBody(e.target.value)}
              />
            )}
          </Field>
          <div>
            <p className="mb-1 text-sm font-semibold">Sisipkan data</p>
            <div className="flex flex-wrap gap-2">
              {Object.entries(tpl.placeholders).map(([k, label]) => (
                <Button key={k} size="sm" variant="ghost" onClick={() => insert(k)} title={label}>
                  {label}
                  {tpl.required.includes(k) ? " (wajib)" : ""}
                </Button>
              ))}
            </div>
          </div>
          <Field label="Catatan perubahan" hint="Opsional, tersimpan di riwayat versi.">
            {({ id, describedBy }) => <Input id={id} aria-describedby={describedBy} value={note} maxLength={500} onChange={(e) => setNote(e.target.value)} />}
          </Field>
          <div className="flex flex-wrap gap-2">
            <Button variant="primary" busy={busy} onClick={() => save()}>
              Simpan versi baru
            </Button>
            {tpl.customized ? (
              <Button busy={busy} onClick={() => save({ resetToDefault: true })}>
                Kembalikan ke bawaan
              </Button>
            ) : null}
          </div>
        </div>
        <div className="min-w-0">
          <p className="mb-1 text-sm font-semibold">Pratinjau dengan data contoh</p>
          {previewErr ? (
            <Alert tone="warning" title={Object.values(previewErr.fieldErrors)[0] ?? previewErr.message} />
          ) : preview ? (
            <div className="rounded-md border border-line bg-surface p-4 text-[15px]">
              {isEmail ? <p className="mb-3 font-semibold">{preview.subject}</p> : null}
              <p className="whitespace-pre-line break-words">{preview.body}</p>
            </div>
          ) : null}
          <h3 className="mb-2 mt-6 font-semibold">Riwayat versi</h3>
          <PageState loading={versions.loading} error={versions.error} onRetry={versions.reload} empty={versions.data?.items.length === 0} emptyText="Belum pernah diubah. Isi di atas adalah template bawaan.">
            <ul className="divide-y divide-line border-y border-line text-sm">
              {versions.data?.items.map((v) => (
                <li key={v.version} className="flex flex-wrap items-center justify-between gap-2 py-2">
                  <span>
                    Versi {v.version}
                    {v.isDefault ? " (bawaan)" : ""} · {fmtDateTime(v.createdAt)} · {v.authorName ?? "-"}
                    {v.note ? <span className="block text-ink-2">{v.note}</span> : null}
                  </span>
                  {v.version !== tpl.version ? (
                    <Button size="sm" busy={busy} onClick={() => save({ restoreVersion: v.version })}>
                      Pulihkan
                    </Button>
                  ) : (
                    <span className="text-ink-2">Berlaku</span>
                  )}
                </li>
              ))}
            </ul>
          </PageState>
        </div>
      </div>
    </section>
  );
}
