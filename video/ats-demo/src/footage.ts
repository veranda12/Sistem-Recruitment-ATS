import { useEffect, useState } from "react";
import { cancelRender, continueRender, delayRender, staticFile } from "remotion";
import { SHOTS } from "./timeline";

export type Box = { x: number; y: number; w: number; h: number };
export type ManifestShot = { id: string; file: string; route: string; actor: string; state: string; boxes: Record<string, Box> };
export type Manifest = { source: { viewport: { width: number; height: number } }; shots: ManifestShot[] };

const MISSING = "Footage belum ada. Jalankan stack demo dan capture dulu (lihat video/ats-demo/README.md):";

// Memuat manifest footage sekali. Render gagal dengan pesan spesifik bila manifest, still, atau kotak elemen
// yang dirujuk timeline tidak ada, sehingga tidak pernah ada frame dengan media kosong.
export function useManifest(): Manifest | null {
  const [handle] = useState(() => delayRender("Memuat manifest footage"));
  const [m, setM] = useState<Manifest | null>(null);
  useEffect(() => {
    fetch(staticFile("footage/manifest.json"))
      .then((r) => {
        if (!r.ok) throw new Error(`${MISSING} public/footage/manifest.json (${r.status})`);
        return r.json() as Promise<Manifest>;
      })
      .then((data) => {
        const ids = new Map(data.shots.map((s) => [s.id, s]));
        for (const shot of SHOTS) {
          const ms = ids.get(shot.still);
          if (!ms) throw new Error(`${MISSING} still "${shot.still}" tidak ada di manifest`);
          const names = [
            ...("boxes" in shot.focus ? shot.focus.boxes : []),
            ...(shot.reframe && "boxes" in shot.reframe.to ? shot.reframe.to.boxes : []),
            ...(shot.cursor ? [shot.cursor.from, shot.cursor.to].filter((p): p is string => typeof p === "string") : []),
          ];
          for (const n of names) {
            if (!ms.boxes[n]) throw new Error(`Kotak elemen "${n}" tidak ada pada still "${shot.still}" (manifest)`);
          }
        }
        setM(data);
        continueRender(handle);
      })
      .catch((e: unknown) => cancelRender(e instanceof Error ? e : new Error(String(e))));
  }, [handle]);
  return m;
}
