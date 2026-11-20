import * as RDialog from "@radix-ui/react-dialog";
import {
  createContext,
  forwardRef,
  useCallback,
  useContext,
  useId,
  useMemo,
  useState,
  type ButtonHTMLAttributes,
  type InputHTMLAttributes,
  type ReactNode,
  type SelectHTMLAttributes,
  type TextareaHTMLAttributes,
} from "react";
import type { ApiError } from "../lib/api";

export function cx(...c: (string | false | null | undefined)[]) {
  return c.filter(Boolean).join(" ");
}

type Variant = "primary" | "secondary" | "ghost" | "danger";
const variants: Record<Variant, string> = {
  primary: "bg-primary text-white hover:bg-primary-hover border border-primary",
  secondary: "bg-white text-primary border border-line-strong hover:bg-surface",
  ghost: "bg-transparent text-primary border border-transparent hover:bg-primary-soft",
  danger: "bg-danger text-white border border-danger hover:opacity-90",
};

export const Button = forwardRef<
  HTMLButtonElement,
  ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; busy?: boolean; size?: "md" | "sm" }
>(function Button({ variant = "secondary", busy, size = "md", className, children, disabled, ...rest }, ref) {
  return (
    <button
      ref={ref}
      className={cx(
        "inline-flex items-center justify-center gap-2 rounded-md font-semibold disabled:cursor-not-allowed disabled:opacity-60",
        size === "md" ? "min-h-11 px-4 text-[15px]" : "min-h-9 px-3 text-sm",
        variants[variant],
        className,
      )}
      disabled={disabled || busy}
      aria-busy={busy || undefined}
      {...rest}
    >
      {busy ? <span className="sr-only">Memproses. </span> : null}
      {children}
    </button>
  );
});

type FieldProps = {
  label: string;
  required?: boolean;
  hint?: string;
  error?: string;
  children: (ids: { id: string; describedBy?: string; invalid: boolean }) => ReactNode;
};

/** Field: label dengan penanda wajib/opsional, hint, dan pesan error yang terhubung ke input. */
export function Field({ label, required, hint, error, children }: FieldProps) {
  const id = useId();
  const hintId = hint ? `${id}-hint` : undefined;
  const errId = error ? `${id}-err` : undefined;
  const describedBy = [hintId, errId].filter(Boolean).join(" ") || undefined;
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-sm font-semibold text-ink">
        {label} <span className="font-normal text-ink-2">{required ? "(wajib)" : "(opsional)"}</span>
      </label>
      {hint ? (
        <p id={hintId} className="text-[13px] text-ink-2">
          {hint}
        </p>
      ) : null}
      {children({ id, describedBy, invalid: !!error })}
      {error ? (
        <p id={errId} className="text-[13px] font-medium text-danger">
          {error}
        </p>
      ) : null}
    </div>
  );
}

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement> & { invalid?: boolean }>(function Input(
  { invalid, className, ...rest },
  ref,
) {
  return <input ref={ref} className={cx("field-input", className)} aria-invalid={invalid || undefined} {...rest} />;
});

export function Select({ invalid, className, ...rest }: SelectHTMLAttributes<HTMLSelectElement> & { invalid?: boolean }) {
  return <select className={cx("field-input", className)} aria-invalid={invalid || undefined} {...rest} />;
}

export function Textarea({ invalid, className, ...rest }: TextareaHTMLAttributes<HTMLTextAreaElement> & { invalid?: boolean }) {
  return <textarea className={cx("field-input min-h-24", className)} aria-invalid={invalid || undefined} {...rest} />;
}

type Tone = "info" | "success" | "warning" | "danger";
const tones: Record<Tone, string> = {
  info: "bg-primary-soft text-ink border-primary",
  success: "bg-success-soft text-ink border-success",
  warning: "bg-warning-soft text-ink border-warning",
  danger: "bg-danger-soft text-ink border-danger",
};

/** Notifikasi persisten di halaman. danger memakai role=alert, lainnya role=status. */
export function Alert({ tone = "info", title, children, action }: { tone?: Tone; title?: string; children?: ReactNode; action?: ReactNode }) {
  return (
    <div role={tone === "danger" ? "alert" : "status"} className={cx("rounded-md border-l-4 px-4 py-3", tones[tone])}>
      {title ? <p className="font-semibold">{title}</p> : null}
      {children ? <div className="text-sm text-ink">{children}</div> : null}
      {action ? <div className="mt-2">{action}</div> : null}
    </div>
  );
}

const badgeTones: Record<string, string> = {
  active: "bg-primary-soft text-primary",
  published: "bg-success-soft text-success",
  hired: "bg-success-soft text-success",
  withdrawn: "bg-surface text-ink-2",
  closed: "bg-surface text-ink-2",
  draft: "bg-warning-soft text-warning",
  rejected: "bg-danger-soft text-danger",
};

export function Badge({ status, children }: { status: string; children: ReactNode }) {
  return (
    <span className={cx("inline-flex items-center rounded-sm px-2 py-0.5 text-[13px] font-semibold", badgeTones[status] ?? "bg-surface text-ink-2")}>
      {children}
    </span>
  );
}

/** Dialog modal (Radix): focus trap, Escape menutup, fokus kembali ke pemicu. */
export function Dialog({
  open,
  onOpenChange,
  title,
  description,
  children,
}: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  title: string;
  description?: string;
  children: ReactNode;
}) {
  return (
    <RDialog.Root open={open} onOpenChange={onOpenChange}>
      <RDialog.Portal>
        <RDialog.Overlay className="fixed inset-0 z-40 bg-[rgba(24,38,56,0.45)]" />
        <RDialog.Content
          className="fixed left-1/2 top-1/2 z-50 max-h-[90dvh] w-[calc(100%-32px)] max-w-lg -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-lg border border-line bg-white p-6"
          aria-describedby={description ? undefined : undefined}
        >
          <RDialog.Title className="text-lg font-bold">{title}</RDialog.Title>
          {description ? <RDialog.Description className="mt-1 text-sm text-ink-2">{description}</RDialog.Description> : <RDialog.Description className="sr-only">{title}</RDialog.Description>}
          <div className="mt-4">{children}</div>
        </RDialog.Content>
      </RDialog.Portal>
    </RDialog.Root>
  );
}

// Toast: diumumkan lewat live region polite; error memakai assertive.
type Toast = { id: number; tone: Tone; text: string };
const ToastCtx = createContext<(tone: Tone, text: string) => void>(() => {});

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<Toast[]>([]);
  const push = useCallback((tone: Tone, text: string) => {
    const id = Date.now() + Math.random();
    setItems((xs) => [...xs.slice(-2), { id, tone, text }]);
    setTimeout(() => setItems((xs) => xs.filter((x) => x.id !== id)), 6000);
  }, []);
  const polite = items.filter((i) => i.tone !== "danger");
  const assertive = items.filter((i) => i.tone === "danger");
  return (
    <ToastCtx.Provider value={push}>
      {children}
      <div className="pointer-events-none fixed bottom-4 left-1/2 z-50 flex w-[calc(100%-32px)] max-w-md -translate-x-1/2 flex-col gap-2">
        <div aria-live="polite" className="flex flex-col gap-2">
          {polite.map((t) => (
            <div key={t.id} className={cx("pointer-events-auto rounded-md border-l-4 px-4 py-3 text-sm", tones[t.tone])}>
              {t.text}
            </div>
          ))}
        </div>
        <div aria-live="assertive" className="flex flex-col gap-2">
          {assertive.map((t) => (
            <div key={t.id} className={cx("pointer-events-auto rounded-md border-l-4 px-4 py-3 text-sm", tones[t.tone])}>
              {t.text}
            </div>
          ))}
        </div>
      </div>
    </ToastCtx.Provider>
  );
}
export const useToast = () => useContext(ToastCtx);

/** State halaman standar: memuat, gagal, forbidden, kosong. */
export function PageState({
  loading,
  error,
  empty,
  emptyText,
  onRetry,
  children,
}: {
  loading: boolean;
  error: ApiError | null;
  empty?: boolean;
  emptyText?: string;
  onRetry?: () => void;
  children: ReactNode;
}) {
  if (loading)
    return (
      <p role="status" className="py-8 text-ink-2">
        Memuat data...
      </p>
    );
  if (error) {
    if (error.status === 403) return <Alert tone="warning" title="Akses ditolak">Anda tidak memiliki akses ke halaman ini.</Alert>;
    if (error.status === 404) return <Alert tone="warning" title="Tidak ditemukan">{error.message}</Alert>;
    return (
      <Alert
        tone="danger"
        title={error.status === 0 ? "Tidak ada koneksi" : "Gagal memuat"}
        action={onRetry ? <Button onClick={onRetry}>Coba lagi</Button> : null}
      >
        {error.message}
        {error.requestId ? <span className="block text-[13px] text-ink-2">Kode permintaan: {error.requestId}</span> : null}
      </Alert>
    );
  }
  if (empty) return <p className="rounded-md border border-dashed border-line px-4 py-8 text-center text-ink-2">{emptyText ?? "Belum ada data."}</p>;
  return <>{children}</>;
}

export function PageHeader({ title, subtitle, actions }: { title: string; subtitle?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="mb-6 flex flex-wrap items-end justify-between gap-3">
      <div>
        <h1 className="text-[26px] md:text-[28px]">{title}</h1>
        {subtitle ? <p className="mt-1 text-ink-2">{subtitle}</p> : null}
      </div>
      {actions ? <div className="flex flex-wrap gap-2">{actions}</div> : null}
    </div>
  );
}

/** Daftar label/nilai untuk tampilan ringkas dan mobile. */
export function DL({ items }: { items: [string, ReactNode][] }) {
  return (
    <dl className="grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-[minmax(140px,max-content)_1fr]">
      {items.map(([k, v]) => (
        <div key={k} className="contents">
          <dt className="text-sm text-ink-2">{k}</dt>
          <dd className="text-[15px]">{v}</dd>
        </div>
      ))}
    </dl>
  );
}

export function useFieldErrors() {
  const [errors, setErrors] = useState<Record<string, string>>({});
  return useMemo(() => ({ errors, setErrors, clear: () => setErrors({}) }), [errors]);
}
