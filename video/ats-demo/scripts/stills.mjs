// Render still pemeriksaan: awal, tengah, dan akhir setiap shot (termasuk frame sebelum dan sesudah tiap cut)
// pada resolusi asli 1920x1080 dan pratinjau 960x540. Keluaran: out/stills/{full,preview}/fNNNN-<shot>.png
//
//   npm run stills            semua shot
//   npm run stills -- 600 745 frame tertentu saja
import { bundle } from "@remotion/bundler";
import { renderStill, selectComposition } from "@remotion/renderer";
import { mkdirSync, rmSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const out = join(root, "out/stills");
const serveUrl = await bundle({ entryPoint: join(root, "src/index.ts"), publicDir: join(root, "public") });
const composition = await selectComposition({ serveUrl, id: "AtsProductDemo" });

// Timing dibaca dari bundle agar selalu sama dengan timeline.ts.
const { SHOTS } = await import("../src/timeline.ts").catch(() => ({ SHOTS: null }));
let frames = process.argv.slice(2).map(Number).filter((n) => Number.isInteger(n));
const label = new Map();
if (frames.length === 0) {
  if (!SHOTS) throw new Error("Tidak dapat membaca src/timeline.ts; berikan nomor frame sebagai argumen.");
  for (const s of SHOTS) {
    for (const f of [s.from, s.from + Math.floor(s.dur / 2), s.from + s.dur - 1]) {
      frames.push(f);
      label.set(f, s.still);
    }
  }
}
frames = [...new Set(frames)].sort((a, b) => a - b);
rmSync(out, { recursive: true, force: true });
for (const [dir, scale] of [["full", 1], ["preview", 0.5]]) {
  mkdirSync(join(out, dir), { recursive: true });
  for (const f of frames) {
    const name = `f${String(f).padStart(4, "0")}${label.has(f) ? "-" + label.get(f) : ""}.png`;
    await renderStill({ composition, serveUrl, frame: f, output: join(out, dir, name), scale, imageFormat: "png" });
  }
}
console.log(`${frames.length} frame x 2 ukuran -> ${out}`);
