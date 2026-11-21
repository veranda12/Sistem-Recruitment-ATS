// Inventaris dependency + lisensi untuk serah terima buyout. Jalankan dari root repo: node ops/tools/dependency-inventory.mjs
// Menulis docs/handover/dependencies.md. Lisensi Go dideteksi dari isi berkas LICENSE di module cache (heuristik kata kunci);
// lisensi npm dari field "license" package.json. Hasil "PERIKSA MANUAL" wajib ditinjau.
import { execFileSync } from "node:child_process";
import { existsSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const root = process.cwd();
// npm di Windows adalah npm.cmd sehingga butuh shell; go dijalankan tanpa shell agar argumen template tidak ditafsirkan.
const sh = (cmd, args, cwd, shell = false) => execFileSync(cmd, args, { cwd, encoding: "utf8", maxBuffer: 64 << 20, shell });

function classify(text) {
  const t = text.replace(/\s+/g, " ");
  if (/Apache License,? Version 2\.0/i.test(t)) return "Apache-2.0";
  if (/Mozilla Public License,? v(ersion)? ?2\.0/i.test(t)) return "MPL-2.0";
  if (/GNU (Lesser|Library) General Public License/i.test(t)) return "LGPL (PERIKSA MANUAL)";
  if (/GNU General Public License/i.test(t)) return "GPL (PERIKSA MANUAL)";
  if (/Permission is hereby granted, free of charge/i.test(t)) return "MIT";
  if (/Redistribution and use in source and binary forms/i.test(t)) {
    if (/Neither the name/i.test(t)) return "BSD-3-Clause";
    return "BSD-2-Clause";
  }
  if (/ISC License|Permission to use, copy, modify, and\/or distribute/i.test(t)) return "ISC";
  return "PERIKSA MANUAL";
}

// Go: modul yang benar-benar terhubung ke binary (go list -deps), bukan seluruh grafik.
const goMods = new Map();
const deps = sh("go", ["list", "-deps", "-f", "{{if .Module}}{{.Module.Path}}@{{.Module.Version}}|{{.Module.Dir}}{{end}}", "./..."], join(root, "backend"))
  .split("\n").map((l) => l.trim()).filter(Boolean);
for (const line of deps) {
  const [id, dir] = line.split("|");
  if (!id || id.startsWith("ats@") || goMods.has(id)) continue;
  let lic = "PERIKSA MANUAL";
  if (dir && existsSync(dir)) {
    const f = readdirSync(dir).find((n) => /^(LICEN[CS]E|COPYING)/i.test(n));
    if (f) lic = classify(readFileSync(join(dir, f), "utf8"));
  }
  goMods.set(id, lic);
}

// npm: dependency produksi (bundel frontend), termasuk transitif.
const tree = JSON.parse(sh("npm", ["ls", "--omit=dev", "--all", "--json"], join(root, "frontend"), process.platform === "win32"));
const npm = new Map();
(function walk(node) {
  for (const [name, d] of Object.entries(node.dependencies || {})) {
    const key = `${name}@${d.version}`;
    if (npm.has(key)) continue;
    let lic = "PERIKSA MANUAL";
    const pj = join(root, "frontend", "node_modules", name, "package.json");
    if (existsSync(pj)) {
      const p = JSON.parse(readFileSync(pj, "utf8"));
      lic = typeof p.license === "string" ? p.license : p.license?.type || "PERIKSA MANUAL";
    }
    npm.set(key, lic);
    walk(d);
  }
})(tree);

const count = (m) => [...m.values()].reduce((a, l) => ((a[l] = (a[l] || 0) + 1), a), {});
const table = (m) => ["| Paket | Lisensi |", "|---|---|", ...[...m.entries()].sort().map(([k, v]) => `| ${k} | ${v} |`)].join("\n");
const out = `# Inventaris Dependency dan Lisensi

Dihasilkan oleh \`node ops/tools/dependency-inventory.mjs\` pada ${new Date().toISOString().slice(0, 10)}. Hanya dependency yang ikut ke artefak produksi (Go: paket yang terhubung ke binary; npm: dependency produksi frontend). Tool pengembangan (Vite, TypeScript, Playwright, ESLint, sqlc, k6) tidak ikut terdistribusi.

Font: Plus Jakarta Sans (paket \`@fontsource/plus-jakarta-sans\`), lisensi SIL Open Font License 1.1, di-self-host.

## Ringkasan

| Ekosistem | Jumlah | Lisensi |
|---|---|---|
| Go | ${goMods.size} | ${Object.entries(count(goMods)).map(([k, v]) => `${k}: ${v}`).join(", ")} |
| npm (produksi) | ${npm.size} | ${Object.entries(count(npm)).map(([k, v]) => `${k}: ${v}`).join(", ")} |

Butir "PERIKSA MANUAL" wajib ditinjau sebelum serah terima. Tidak ada klaim kelayakan hukum; tinjauan akhir oleh Legal pelanggan.

## Go

${table(goMods)}

## npm (produksi)

${table(npm)}
`;
writeFileSync(join(root, "docs", "handover", "dependencies.md"), out);
console.log(`Go ${goMods.size} modul, npm ${npm.size} paket ->`, "docs/handover/dependencies.md");
console.log("Go:", count(goMods), "npm:", count(npm));
