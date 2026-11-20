import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

// Uji kontras WCAG 2.x untuk setiap pasangan warna yang benar-benar dipakai UI.
const css = readFileSync(resolve(__dirname, "../index.css"), "utf8");
const token = (name: string): string => {
  const m = css.match(new RegExp(`--color-${name}:\\s*(#[0-9a-fA-F]{6})`));
  if (!m?.[1]) throw new Error(`token ${name} tidak ada`);
  return m[1];
};

function luminance(hex: string): number {
  const c = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255);
  const [r, g, b] = c.map((v) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4)) as [number, number, number];
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}
export function contrast(a: string, b: string): number {
  const [l1, l2] = [luminance(a), luminance(b)].sort((x, y) => y - x) as [number, number];
  return (l1 + 0.05) / (l2 + 0.05);
}

// [teks/elemen, latar, minimum]. 4.5 untuk teks normal, 3 untuk batas komponen UI dan fokus.
const pairs: [string, string, number][] = [
  ["ink", "bg", 4.5],
  ["ink", "surface", 4.5],
  ["ink-2", "bg", 4.5],
  ["ink-2", "surface", 4.5],
  ["primary", "bg", 4.5],
  ["primary", "surface", 4.5],
  ["primary", "primary-soft", 4.5],
  ["danger", "bg", 4.5],
  ["danger", "danger-soft", 4.5],
  ["success", "success-soft", 4.5],
  ["success", "bg", 4.5],
  ["warning", "warning-soft", 4.5],
  ["line-strong", "bg", 3],
  ["primary", "bg", 3],
];

describe("kontras token", () => {
  it.each(pairs)("%s di atas %s >= %d", (fg, bg, min) => {
    expect(contrast(token(fg), token(bg))).toBeGreaterThanOrEqual(min);
  });
  it("teks putih di tombol primer", () => {
    expect(contrast("#ffffff", token("primary"))).toBeGreaterThanOrEqual(4.5);
    expect(contrast("#ffffff", token("primary-hover"))).toBeGreaterThanOrEqual(4.5);
    expect(contrast("#ffffff", token("danger"))).toBeGreaterThanOrEqual(4.5);
  });
});
