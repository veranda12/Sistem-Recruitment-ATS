import { AbsoluteFill, Easing, Img, cancelRender, interpolate, staticFile, useCurrentFrame } from "remotion";
import type { Box, ManifestShot } from "./footage";
import { MAX_ZOOM, STAGE, type Focus, type Point, type Rect, type Shot } from "./timeline";
import { Cursor } from "./Cursor";
import { color } from "./theme";

const ease = Easing.bezier(0.45, 0, 0.2, 1);

function resolveFocus(f: Focus, boxes: Record<string, Box>): Rect {
  if ("rect" in f) return f.rect;
  const pad = f.pad ?? 24;
  const bs = f.boxes.map((n) => boxes[n]!);
  const x0 = Math.min(...bs.map((b) => b.x)) - pad;
  const y0 = Math.min(...bs.map((b) => b.y)) - pad;
  const x1 = Math.max(...bs.map((b) => b.x + b.w)) + pad;
  const y1 = Math.max(...bs.map((b) => b.y + b.h)) + pad;
  return [x0, y0, x1 - x0, y1 - y0];
}

function resolvePoint(p: Point, boxes: Record<string, Box>): [number, number] {
  if (typeof p !== "string") return p;
  const b = boxes[p]!;
  return [b.x + b.w / 2, b.y + b.h / 2];
}

// Kamera: persegi fokus dimuat utuh di area panggung lalu dijepit agar gambar selalu menutup panggung.
function camera([x, y, w, h]: Rect, view: { width: number; height: number }) {
  const k = Math.min(MAX_ZOOM, STAGE.w / w, STAGE.h / h);
  const iw = view.width * k;
  const ih = view.height * k;
  const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v));
  const tx = iw >= STAGE.w ? clamp(STAGE.w / 2 - (x + w / 2) * k, STAGE.w - iw, 0) : (STAGE.w - iw) / 2;
  const ty = ih >= STAGE.h ? clamp(STAGE.h / 2 - (y + h / 2) * k, STAGE.h - ih, 0) : (STAGE.h - ih) / 2;
  return { k, tx, ty, iw };
}

export const ShotView: React.FC<{ shot: Shot; data: ManifestShot; view: { width: number; height: number } }> = ({ shot, data, view }) => {
  const frame = useCurrentFrame();
  const start = resolveFocus(shot.focus, data.boxes);
  let rect = start;
  if (shot.reframe) {
    const end = resolveFocus(shot.reframe.to, data.boxes);
    const t = interpolate(frame, [shot.reframe.at, shot.reframe.at + shot.reframe.dur], [0, 1], {
      extrapolateLeft: "clamp",
      extrapolateRight: "clamp",
      easing: ease,
    });
    rect = start.map((v, i) => v + (end[i]! - v) * t) as Rect;
  }
  const cam = camera(rect, view);

  let cursor: React.ReactNode = null;
  if (shot.cursor) {
    const c = shot.cursor;
    const [fx, fy] = resolvePoint(c.from, data.boxes);
    const [toX, toY] = resolvePoint(c.to, data.boxes);
    const t = interpolate(frame, [c.moveAt, c.moveAt + c.moveDur], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp", easing: ease });
    cursor = (
      <Cursor
        x={cam.tx + (fx + (toX - fx) * t) * cam.k}
        y={cam.ty + (fy + (toY - fy) * t) * cam.k}
        opacity={
          c.fadeIn
            ? interpolate(frame, [Math.max(0, c.moveAt - 8), Math.max(8, c.moveAt)], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" })
            : 1
        }
        press={interpolate(frame, [c.clickAt, c.clickAt + 3, c.clickAt + 9], [1, 0.86, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" })}
      />
    );
  }

  return (
    <AbsoluteFill style={{ width: STAGE.w, height: STAGE.h, overflow: "hidden", backgroundColor: color.bg }}>
      <Img
        src={staticFile(`footage/${data.file}`)}
        onError={() => cancelRender(new Error(`Still hilang: public/footage/${data.file}. Jalankan ulang capture.`))}
        style={{ position: "absolute", left: cam.tx, top: cam.ty, width: cam.iw, height: "auto", maxWidth: "none" }}
      />
      {cursor}
    </AbsoluteFill>
  );
};
