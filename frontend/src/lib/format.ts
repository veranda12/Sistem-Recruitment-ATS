// Tanggal ditampilkan menurut zona organisasi (default Asia/Jakarta); server mengirim waktu absolut.
let orgTimeZone = "Asia/Jakarta";
export function setOrgTimeZone(tz: string) {
  orgTimeZone = tz;
}
export function getOrgTimeZone() {
  return orgTimeZone;
}

const zoneLabel: Record<string, string> = { "Asia/Jakarta": "WIB", "Asia/Makassar": "WITA", "Asia/Jayapura": "WIT" };

export function fmtDate(iso: string | null | undefined): string {
  if (!iso) return "-";
  return new Intl.DateTimeFormat("id-ID", { day: "numeric", month: "long", year: "numeric", timeZone: orgTimeZone }).format(new Date(iso));
}

export function fmtDateTime(iso: string | null | undefined): string {
  if (!iso) return "-";
  const s = new Intl.DateTimeFormat("id-ID", {
    day: "numeric",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    timeZone: orgTimeZone,
  }).format(new Date(iso));
  return `${s} ${zoneLabel[orgTimeZone] ?? orgTimeZone}`;
}

export function fmtTime(d: Date): string {
  return new Intl.DateTimeFormat("id-ID", { hour: "2-digit", minute: "2-digit", second: "2-digit", timeZone: orgTimeZone }).format(d);
}

export const employmentLabel: Record<string, string> = {
  full_time: "Karyawan tetap",
  contract: "Kontrak",
  internship: "Magang",
  part_time: "Paruh waktu",
};

export const trackLabel: Record<string, string> = {
  OPERATOR: "Operator",
  STAFF: "Staff",
  MT_S1_D4: "Management Trainee S1/D4",
};

export const reasonLabel: Record<string, string> = {
  penambahan: "Penambahan",
  penggantian: "Penggantian",
  proyek_baru: "Proyek baru",
};

export const statusLabel: Record<string, string> = {
  active: "Aktif",
  withdrawn: "Ditarik",
  rejected: "Tidak lolos",
  hired: "Diterima",
  draft: "Draft",
  published: "Terbit",
  closed: "Ditutup",
};
