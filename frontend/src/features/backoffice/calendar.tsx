import { useState } from "react";
import { Link } from "react-router";
import { getOrgTimeZone } from "../../lib/format";
import { useQuery } from "../../lib/useQuery";
import { Button, PageHeader, PageState, cx } from "../../ui";

type CalItem = {
  id: string;
  applicationId: string;
  startsAt: string;
  endsAt: string;
  timezone: string;
  mode: string;
  location: string;
  status: string;
  candidateName: string;
  jobTitle: string;
  interviewerNames: string[];
};

const modeLabel: Record<string, string> = { onsite: "Tatap muka", video: "Video", phone: "Telepon" };
const zoneAbbr: Record<string, string> = { "Asia/Jakarta": "WIB", "Asia/Makassar": "WITA", "Asia/Jayapura": "WIT" };
const DAY = 86_400_000;

/** Tanggal kalender (YYYY-MM-DD) suatu waktu di zona organisasi. */
function dayKey(d: Date, tz: string) {
  return new Intl.DateTimeFormat("sv-SE", { timeZone: tz, year: "numeric", month: "2-digit", day: "2-digit" }).format(d);
}

/** Senin 00.00 (zona organisasi) dari minggu yang memuat `d`. */
function weekStart(d: Date, tz: string) {
  const [y, m, day] = dayKey(d, tz).split("-").map(Number) as [number, number, number];
  const utcMidnight = Date.UTC(y, m - 1, day);
  const weekday = (new Date(utcMidnight).getUTCDay() + 6) % 7; // 0 = Senin
  // Selisih zona organisasi terhadap UTC pada tanggal itu, agar batas minggu tepat pukul 00.00 waktu setempat.
  const offset = Date.parse(new Date(utcMidnight).toLocaleString("en-US", { timeZone: tz })) - Date.parse(new Date(utcMidnight).toLocaleString("en-US", { timeZone: "UTC" }));
  return new Date(utcMidnight - weekday * DAY - offset);
}

function timeRange(it: CalItem) {
  const f = new Intl.DateTimeFormat("id-ID", { hour: "2-digit", minute: "2-digit", timeZone: it.timezone });
  return `${f.format(new Date(it.startsAt))}–${f.format(new Date(it.endsAt))} ${zoneAbbr[it.timezone] ?? it.timezone}`;
}

/** Kalender interview mingguan: interview dalam akses Anda dan interview tempat Anda menjadi interviewer. */
export function CalendarPage() {
  const tz = getOrgTimeZone();
  const [start, setStart] = useState(() => weekStart(new Date(), tz));
  const [mine, setMine] = useState(false);
  const end = new Date(start.getTime() + 7 * DAY);
  const q = useQuery<{ items: CalItem[] }>(`/interviews?from=${encodeURIComponent(start.toISOString())}&to=${encodeURIComponent(end.toISOString())}${mine ? "&mine=true" : ""}`);
  const days = Array.from({ length: 7 }, (_, i) => new Date(start.getTime() + i * DAY + 12 * 3_600_000));
  const today = dayKey(new Date(), tz);
  const range = `${new Intl.DateTimeFormat("id-ID", { day: "numeric", month: "long", timeZone: tz }).format(days[0])} sampai ${new Intl.DateTimeFormat("id-ID", { dateStyle: "long", timeZone: tz }).format(days[6])}`;

  return (
    <div>
      <PageHeader title="Kalender interview" subtitle={range} />
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <Button size="sm" onClick={() => setStart(new Date(start.getTime() - 7 * DAY))}>
          Minggu sebelumnya
        </Button>
        <Button size="sm" onClick={() => setStart(weekStart(new Date(), tz))}>
          Minggu ini
        </Button>
        <Button size="sm" onClick={() => setStart(new Date(start.getTime() + 7 * DAY))}>
          Minggu berikutnya
        </Button>
        <label className="ml-auto flex min-h-11 items-center gap-2 text-sm">
          <input type="checkbox" className="h-5 w-5" checked={mine} onChange={(e) => setMine(e.target.checked)} />
          Hanya interview saya
        </label>
      </div>
      <PageState loading={q.loading} error={q.error} onRetry={q.reload}>
        <div className="grid gap-3 lg:grid-cols-7">
          {days.map((d) => {
            const key = dayKey(d, tz);
            const items = (q.data?.items ?? []).filter((it) => dayKey(new Date(it.startsAt), tz) === key);
            return (
              <section key={key} aria-label={new Intl.DateTimeFormat("id-ID", { dateStyle: "full", timeZone: tz }).format(d)} className={cx("min-w-0 rounded-md border p-2", key === today ? "border-primary" : "border-line")}>
                <h2 className={cx("mb-2 text-sm font-semibold", key === today ? "text-primary" : "")}>
                  {new Intl.DateTimeFormat("id-ID", { weekday: "short", day: "numeric", month: "short", timeZone: tz }).format(d)}
                </h2>
                {items.length === 0 ? <p className="text-[13px] text-ink-2">Tidak ada interview.</p> : null}
                <ul className="flex flex-col gap-2">
                  {items.map((it) => (
                    <li key={it.id} className="rounded-md border border-line bg-surface p-2 text-[13px]">
                      <p className="font-semibold">{timeRange(it)}</p>
                      <Link className="font-semibold text-primary hover:underline" to={`/staf/lamaran/${it.applicationId}`}>
                        {it.candidateName}
                      </Link>
                      <p className="text-ink-2">{it.jobTitle}</p>
                      <p className="text-ink-2">
                        {modeLabel[it.mode] ?? it.mode}
                        {it.status === "completed" ? " · Selesai" : ""}
                      </p>
                      {it.location ? <p className="break-words text-ink-2">{it.location}</p> : null}
                      <p className="text-ink-2">Interviewer: {it.interviewerNames.join(", ") || "-"}</p>
                    </li>
                  ))}
                </ul>
              </section>
            );
          })}
        </div>
      </PageState>
    </div>
  );
}
