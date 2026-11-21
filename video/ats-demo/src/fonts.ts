import { cancelRender, continueRender, delayRender, staticFile } from "remotion";

// Plus Jakarta Sans lokal (sama dengan font aplikasi, dari @fontsource), tanpa akses jaringan saat render.
export const FONT_FAMILY = "Plus Jakarta Sans";

let loaded = false;
export function loadFonts() {
  if (loaded) return;
  loaded = true;
  const handle = delayRender("Memuat font Plus Jakarta Sans");
  Promise.all(
    [400, 500, 600, 700].map((w) => {
      const face = new FontFace(FONT_FAMILY, `url(${staticFile(`fonts/plus-jakarta-sans-latin-${w}-normal.woff2`)}) format("woff2")`, {
        weight: String(w),
      });
      return face.load().then((f) => document.fonts.add(f));
    }),
  )
    .then(() => continueRender(handle))
    .catch((e: unknown) => cancelRender(new Error(`Font lokal gagal dimuat dari public/fonts: ${String(e)}`)));
}
