import { describe, expect, it } from "vitest";
import { changeKind, displayValue, fmtPartial, groups, humanLocator, isoOf, profileDiff, similarEntry, statusText, type Profile, type Run, type Suggestion } from "./model";

const sug = (p: Partial<Suggestion>): Suggestion => ({
  id: p.path ?? "x",
  path: "x",
  section: "contact",
  entryKey: "",
  field: "name",
  raw: "",
  normalized: null,
  status: "pending",
  readDoubts: [],
  assocDoubts: [],
  evidenceIds: [],
  ...p,
});

const profile = (p: Partial<Profile> = {}): Profile =>
  ({
    fullName: "Sinta",
    phone: "",
    birthDate: null,
    city: "Bandung",
    address: "",
    summary: "",
    education: [],
    experiences: [],
    certifications: [],
    contactEmail: "",
    skills: [],
    languages: [],
    links: [],
    projects: [],
    version: 3,
    email: "sinta@contoh.test",
    emailVerified: true,
    identity: { status: "linked" },
    complete: true,
    missing: [],
    updatedAt: "2026-11-21T00:00:00Z",
    ...p,
  }) as Profile;

describe("model review CV", () => {
  it("tanggal mengikuti presisi; tahun saja tidak menjadi 1 Januari", () => {
    expect(fmtPartial({ year: 2021, precision: "year" })).toBe("2021");
    expect(fmtPartial({ year: 2022, month: 2, precision: "month" })).toBe("Feb 2022");
    expect(isoOf({ year: 2022, month: 2, precision: "month" })).toBe("2022-02");
    const s = sug({ field: "period", section: "experience", normalized: { start: { year: 2022, month: 2, precision: "month" }, current: true } });
    expect(displayValue(s)).toBe("Feb 2022 sampai sekarang");
  });

  it("nilai edit kandidat menggantikan hasil parser", () => {
    const s = sug({ field: "phone", normalized: { e164: "+6281234567801" }, status: "edited", editedValue: { e164: "+6281234567809" } });
    expect(displayValue(s)).toBe("+6281234567809");
    expect(changeKind(profile({ phone: "" }), s)).toBe("fill");
    expect(changeKind(profile({ phone: "+6281234567809" }), s)).toBe("same");
    expect(changeKind(profile({ phone: "+6281111111111" }), s)).toBe("replace");
  });

  it("mengelompokkan entri dan menandai yang perlu diperiksa", () => {
    const run = {
      suggestions: [
        sug({ path: "experience[0].title", section: "experience", entryKey: "experience[0]", field: "title", raw: "Backend Engineer", normalized: "Backend Engineer" }),
        sug({ path: "experience[0].company", section: "experience", entryKey: "experience[0]", field: "company", raw: "PT A", normalized: "PT A", assocDoubts: ["Jabatan dan perusahaan ditentukan dari urutan"] }),
        sug({ path: "skills[0]", section: "skills", entryKey: "skills[0]", field: "skill", raw: "Go", normalized: "Go" }),
        sug({ path: "skills[1]", section: "skills", entryKey: "skills[1]", field: "skill", raw: "SQL", normalized: "SQL" }),
        sug({ path: "contact.name", raw: "Sinta", normalized: "Sinta" }),
      ],
    } as Run;
    const g = groups(run);
    expect(g.map((x) => x.key)).toEqual(["contact", "experience[0]", "skills"]);
    expect(g[1]!.title).toBe("Backend Engineer di PT A");
    expect(g[1]!.doubtful).toBe(true);
    expect(g[2]!.items).toHaveLength(2);
  });

  it("mengenali entri profil yang mirip", () => {
    const run = {
      suggestions: [
        sug({ section: "experience", entryKey: "experience[0]", field: "title", raw: "Kasir", normalized: "Kasir" }),
        sug({ section: "experience", entryKey: "experience[0]", field: "company", raw: "Toko Jaya", normalized: "Toko Jaya" }),
      ],
    } as Run;
    const p = profile({ experiences: [{ company: "toko jaya", title: "KASIR", startDate: "2016-01-01", endDate: null, description: "" }] });
    expect(similarEntry(p, groups(run)[0]!)?.index).toBe(0);
  });

  it("diff profil untuk konflik dari tab lain", () => {
    const d = profileDiff(profile(), profile({ city: "Jakarta", experiences: [{ company: "A", title: "B", startDate: "2020-01-01", endDate: null, description: "" }] }));
    expect(d.some((x) => x.startsWith("Kota"))).toBe(true);
    expect(d.some((x) => x.startsWith("Pengalaman kerja"))).toBe(true);
  });

  it("status dan locator dalam bahasa kandidat", () => {
    expect(statusText({ status: "processing", stage: "reading", pagesDone: 1, pagesTotal: 4 } as Run)).toBe("Membaca halaman 2 dari 4");
    expect(statusText({ status: "partial" } as Run)).toBe("Sebagian halaman perlu diperiksa");
    expect(humanLocator("document.xml#t1.r1.c0.p0")).toBe("Isi dokumen, tabel 1, baris 2, kolom 1");
    expect(humanLocator("header1.xml#p1")).toBe("Header dokumen, paragraf 2");
  });
});
