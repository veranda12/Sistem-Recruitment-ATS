import type { Schemas } from "../../../lib/api";

export type Run = Schemas["CvParseRun"];
export type Suggestion = Schemas["CvSuggestion"];
export type Evidence = Schemas["CvEvidence"];
export type Profile = Schemas["Profile"];

type PartialDate = { year: number; month?: number; day?: number; precision: "day" | "month" | "year" };
export type PeriodValue = { start?: PartialDate | null; end?: PartialDate | null; current: boolean; raw?: string };

export const sectionTitle: Record<string, string> = {
  contact: "Kontak",
  summary: "Ringkasan",
  experience: "Pengalaman kerja",
  education: "Pendidikan",
  skills: "Keahlian",
  languages: "Bahasa",
  certifications: "Sertifikasi dan pelatihan",
  links: "Tautan profesional",
  projects: "Proyek",
};

const sectionOrder = ["contact", "summary", "experience", "education", "skills", "languages", "certifications", "links", "projects"];

export function fieldLabel(s: Pick<Suggestion, "section" | "field">): string {
  if (s.field === "name") return s.section === "certifications" ? "Nama sertifikat" : s.section === "projects" ? "Nama proyek" : "Nama";
  const m: Record<string, string> = {
    email: "Email",
    phone: "Telepon",
    city: "Kota",
    summary: "Ringkasan",
    title: "Jabatan",
    company: "Perusahaan",
    location: "Lokasi",
    period: "Periode",
    description: "Uraian",
    institution: "Institusi",
    level: "Jenjang",
    major: "Jurusan",
    gpa: "IPK",
    skill: "Keahlian",
    language: "Bahasa",
    url: "Tautan",
    issuer: "Penerbit",
    date: "Tanggal",
  };
  return m[s.field] ?? s.field;
}

const months = ["Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"];

/** Tanggal sesuai presisinya: "2021", "Feb 2022", "3 Feb 2022". Tahun saja tidak ditampilkan sebagai 1 Januari. */
export function fmtPartial(d: PartialDate | null | undefined): string {
  if (!d) return "";
  if (d.precision === "year" || !d.month) return String(d.year);
  if (d.precision === "month" || !d.day) return `${months[d.month - 1]} ${d.year}`;
  return `${d.day} ${months[d.month - 1]} ${d.year}`;
}

export function fmtPeriod(p: PeriodValue | null | undefined): string {
  if (!p) return "";
  const a = fmtPartial(p.start);
  const b = p.current ? "sekarang" : fmtPartial(p.end);
  if (a && b) return `${a} sampai ${b}`;
  return a || (b ? `lulus ${b}` : "");
}

/** Nilai efektif: hasil edit kandidat bila ada, selain itu hasil normalisasi parser. */
export function effective(s: Suggestion): unknown {
  return s.status === "edited" ? s.editedValue : s.normalized;
}

export function displayValue(s: Suggestion, v: unknown = effective(s)): string {
  if (v === null || v === undefined) return "";
  switch (s.field) {
    case "period":
      return fmtPeriod(v as PeriodValue);
    case "date":
      return fmtPartial(v as PartialDate);
    case "phone":
      return (v as { e164: string }).e164;
    case "url":
      return (v as { target: string }).target;
    case "language": {
      const l = v as { name: string; level: string };
      return l.level ? `${l.name} (${l.level})` : l.name;
    }
  }
  return String(v);
}

/** Bentuk isian form edit untuk periode/tanggal: "2022-02", "2021". */
export function isoOf(d: PartialDate | null | undefined): string {
  if (!d) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  if (d.precision === "year" || !d.month) return String(d.year);
  if (d.precision === "month" || !d.day) return `${d.year}-${pad(d.month)}`;
  return `${d.year}-${pad(d.month)}-${pad(d.day)}`;
}

export function needsCheck(s: Suggestion): boolean {
  return s.readDoubts.length > 0 || s.assocDoubts.length > 0 || s.raw === "";
}

export type Group = { key: string; section: string; title: string; items: Suggestion[]; doubtful: boolean };

/** Kelompok tampilan: kontak/ringkasan per field, entri riwayat per entri, daftar (skill, bahasa, tautan) satu kelompok. */
export function groups(run: Run): Group[] {
  const byKey = new Map<string, Group>();
  const listSections = new Set(["skills", "languages", "links"]);
  for (const s of run.suggestions) {
    const key = listSections.has(s.section) ? s.section : s.entryKey || s.section;
    let g = byKey.get(key);
    if (!g) {
      g = { key, section: s.section, title: entryTitle(run, s), items: [], doubtful: false };
      byKey.set(key, g);
    }
    g.items.push(s);
    g.doubtful ||= needsCheck(s);
  }
  const contactOrder = ["name", "email", "phone", "city"];
  const contact = byKey.get("contact");
  if (contact) contact.items.sort((a, b) => contactOrder.indexOf(a.field) - contactOrder.indexOf(b.field));
  return [...byKey.values()].sort((a, b) => sectionOrder.indexOf(a.section) - sectionOrder.indexOf(b.section));
}

function entryTitle(run: Run, s: Suggestion): string {
  if (!s.entryKey || ["skills", "languages", "links"].includes(s.section)) return sectionTitle[s.section] ?? s.section;
  const sib = run.suggestions.filter((x) => x.entryKey === s.entryKey);
  const pick = (f: string) => sib.find((x) => x.field === f)?.raw ?? "";
  const n = Number(/\[(\d+)\]/.exec(s.entryKey)?.[1] ?? 0) + 1;
  switch (s.section) {
    case "experience":
      return [pick("title"), pick("company")].filter(Boolean).join(" di ") || `Pengalaman ${n}`;
    case "education":
      return pick("institution") || `Pendidikan ${n}`;
    default:
      return pick("name") || `${sectionTitle[s.section]} ${n}`;
  }
}

/** Nilai profil saat ini untuk field tunggal (ditampilkan di samping usulan). */
export function currentValue(profile: Profile, s: Suggestion): string | null {
  if (s.section === "contact" || s.section === "summary") {
    switch (s.field) {
      case "name":
        return profile.fullName;
      case "phone":
        return profile.phone;
      case "email":
        return profile.contactEmail ?? "";
      case "city":
        return profile.city;
      case "summary":
        return profile.summary;
    }
  }
  return null;
}

export type Change = "fill" | "replace" | "same" | "add";

export function changeKind(profile: Profile, s: Suggestion): Change {
  const cur = currentValue(profile, s);
  if (cur === null) return "add";
  const next = s.field === "phone" ? (effective(s) as { e164: string } | null)?.e164 ?? "" : displayValue(s);
  if (!cur) return "fill";
  return norm(cur) === norm(next) ? "same" : "replace";
}

export const changeLabel: Record<Change, string> = {
  fill: "Akan mengisi kolom yang masih kosong",
  replace: "Akan mengganti nilai profil",
  same: "Sama dengan profil",
  add: "Akan ditambahkan",
};

const norm = (s: string) => s.trim().toLowerCase().replace(/\s+/g, " ");

/** Entri profil yang mirip (perusahaan + jabatan, atau institusi) untuk pilihan tambah/ganti/lewati. */
export function similarEntry(profile: Profile, g: Group): { index: number; label: string } | null {
  const pick = (f: string) => {
    const s = g.items.find((x) => x.field === f);
    return s ? String(effective(s) ?? "") : "";
  };
  if (g.section === "experience") {
    const c = norm(pick("company"));
    const t = norm(pick("title"));
    const i = profile.experiences.findIndex((e) => norm(e.company) === c && norm(e.title) === t);
    const e = profile.experiences[i];
    return e ? { index: i, label: `${e.title} di ${e.company}` } : null;
  }
  if (g.section === "education") {
    const inst = norm(pick("institution"));
    const i = profile.education.findIndex((e) => norm(e.institution) === inst);
    const e = profile.education[i];
    return e ? { index: i, label: e.institution } : null;
  }
  return null;
}

/** Diff dua versi profil untuk penyelesaian konflik (profil diubah di tab lain). */
export function profileDiff(before: Profile, after: Profile): string[] {
  const out: string[] = [];
  const scalar: [keyof Profile, string][] = [
    ["fullName", "Nama"],
    ["phone", "Telepon"],
    ["city", "Kota"],
    ["summary", "Ringkasan"],
    ["contactEmail", "Email kontak"],
  ];
  for (const [k, label] of scalar) {
    if ((before[k] ?? "") !== (after[k] ?? "")) out.push(`${label}: "${before[k] ?? ""}" menjadi "${after[k] ?? ""}"`);
  }
  const count = (label: string, a: unknown[] | null | undefined, b: unknown[] | null | undefined) => {
    const x = JSON.stringify(a ?? []);
    const y = JSON.stringify(b ?? []);
    if (x !== y) out.push(`${label}: ${(a ?? []).length} menjadi ${(b ?? []).length} entri, isinya berubah`);
  };
  count("Pengalaman kerja", before.experiences, after.experiences);
  count("Pendidikan", before.education, after.education);
  count("Keahlian", before.skills, after.skills);
  return out;
}

/** Teks status yang dilihat kandidat. */
export function statusText(run: Run): string {
  switch (run.status) {
    case "waiting_scan":
      return "Menunggu pemeriksaan file";
    case "queued":
      return "Dalam antrean";
    case "processing":
      if (run.stage === "reading" && run.pagesTotal) return `Membaca halaman ${Math.min(run.pagesDone + 1, run.pagesTotal)} dari ${run.pagesTotal}`;
      if (run.stage === "converting") return "Mengonversi dokumen Word";
      return "Memeriksa format file";
    case "ready_for_review":
      return "Siap diperiksa";
    case "partial":
      return "Sebagian halaman perlu diperiksa";
    case "failed":
      return "CV tidak dapat dibaca";
    case "applied":
      return "Sudah diterapkan ke profil";
    case "dismissed":
      return "Hasil diabaikan";
    case "cancelled":
      return "Dibatalkan karena dokumen dihapus";
  }
  return run.status;
}

export const inProgress = (r: Run) => r.status === "waiting_scan" || r.status === "queued" || r.status === "processing";

/** Lokasi DOCX yang bisa dibaca manusia: "tabel 1, baris 2, kolom 1" (tanpa nomor halaman rekaan). */
export function humanLocator(loc?: string): string {
  if (!loc) return "";
  const [part = "", rest = ""] = loc.split("#");
  const where = part.startsWith("header") ? "Header dokumen" : part.startsWith("footer") ? "Footer dokumen" : "Isi dokumen";
  const t = /t(\d+)\.r(\d+)\.c(\d+)/.exec(rest);
  if (t) return `${where}, tabel ${t[1]}, baris ${Number(t[2]) + 1}, kolom ${Number(t[3]) + 1}`;
  const tb = /textbox(\d+)/.exec(rest);
  if (tb) return `${where}, kotak teks ${tb[1]}`;
  const p = /p(\d+)/.exec(rest);
  return p ? `${where}, paragraf ${Number(p[1]) + 1}` : where;
}
