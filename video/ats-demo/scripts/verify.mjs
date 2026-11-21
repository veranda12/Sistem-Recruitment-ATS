// Memeriksa MP4 akhir: codec, resolusi, fps, format piksel, jumlah frame, durasi, dan frame hitam.
// Juga mengekstrak frame pertama dan terakhir ke out/verify/ untuk diperiksa mata.
//
//   npm run verify [-- path/ke/video.mp4]
import { execFileSync, spawnSync } from "node:child_process";
import { existsSync, mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const { DURATION, FPS } = await import("../src/timeline.ts");
const file = process.argv[2] ?? join(root, "out/ats-product-demo.mp4");
if (!existsSync(file)) {
  console.error(`MP4 tidak ditemukan: ${file}. Jalankan: npm run render`);
  process.exit(1);
}
// FFmpeg/ffprobe bawaan Remotion, jadi tidak perlu instalasi terpisah.
const remotion = (args) =>
  execFileSync(process.execPath, [join(root, "node_modules/@remotion/cli/remotion-cli.js"), ...args], { cwd: root, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] });

const probe = JSON.parse(
  remotion(["ffprobe", "-v", "error", "-count_frames", "-select_streams", "v:0", "-show_entries",
    "stream=codec_name,profile,width,height,pix_fmt,r_frame_rate,nb_read_frames:format=duration,size", "-of", "json", file]),
);
const s = probe.streams[0];
const dur = Number(probe.format.duration);
const checks = [
  ["codec H.264", s.codec_name === "h264", s.codec_name],
  ["resolusi 1920x1080", s.width === 1920 && s.height === 1080, `${s.width}x${s.height}`],
  [`${FPS} fps`, s.r_frame_rate === `${FPS}/1`, s.r_frame_rate],
  ["pixel format yuv420p", s.pix_fmt === "yuv420p", s.pix_fmt],
  [`${DURATION} frame`, Number(s.nb_read_frames) === DURATION, s.nb_read_frames],
  [`durasi ${DURATION / FPS} detik`, Math.abs(dur - DURATION / FPS) < 0.05, `${dur.toFixed(3)} s`],
];

const out = join(root, "out/verify");
mkdirSync(out, { recursive: true });
// FFmpeg bawaan Remotion versi minimal (tanpa filter blackdetect/select), jadi frame didecode ke piksel abu-abu
// 64x36 lalu dinilai di sini: rata-rata terlalu gelap berarti layar hitam, sebaran terlalu kecil berarti frame polos.
const W = 64, H = 36;
const raw = execFileSync(process.execPath, [join(root, "node_modules/@remotion/cli/remotion-cli.js"), "ffmpeg", "-hide_banner", "-loglevel", "error",
  "-i", file, "-s", `${W}x${H}`, "-pix_fmt", "gray", "-c:v", "rawvideo", "-f", "image2pipe", "-"], { cwd: root, maxBuffer: 64 * 1024 * 1024 });
const n = Math.floor(raw.length / (W * H));
const dark = [];
const flat = [];
for (let f = 0; f < n; f++) {
  const px = raw.subarray(f * W * H, (f + 1) * W * H);
  let sum = 0;
  for (const v of px) sum += v;
  const mean = sum / px.length;
  let varSum = 0;
  for (const v of px) varSum += (v - mean) ** 2;
  if (mean < 24) dark.push(f);
  if (Math.sqrt(varSum / px.length) < 4) flat.push(f);
}
checks.push(["frame terdekode", n === DURATION, String(n)]);
checks.push(["tanpa layar hitam", dark.length === 0, dark.length ? `frame ${dark.slice(0, 10).join(", ")}` : "tidak ada"]);
checks.push(["tanpa frame polos", flat.length === 0, flat.length ? `frame ${flat.slice(0, 10).join(", ")}` : "tidak ada"]);
const ff = (args) => spawnSync(process.execPath, [join(root, "node_modules/@remotion/cli/remotion-cli.js"), "ffmpeg", "-hide_banner", "-loglevel", "error", "-y", ...args], { cwd: root });
ff(["-i", file, "-frames:v", "1", join(out, "first.png")]);
ff(["-sseof", "-0.1", "-i", file, "-frames:v", "1", join(out, "last.png")]);

let ok = true;
for (const [name, pass, value] of checks) {
  console.log(`${pass ? "OK   " : "GAGAL"} ${name.padEnd(22)} ${value}`);
  ok &&= pass;
}
console.log(`ukuran ${(Number(probe.format.size) / 1e6).toFixed(1)} MB; frame pertama/terakhir: ${out}`);
process.exit(ok ? 0 : 1);
