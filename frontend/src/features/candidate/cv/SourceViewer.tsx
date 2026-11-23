import { useState } from "react";
import { Button } from "../../../ui";
import {
  displayValue,
  fieldLabel,
  humanLocator,
  type Evidence,
  type Run,
  type Suggestion,
} from "./model";

/** Preview sumber: gambar halaman dengan sorotan kotak bukti, atau kutipan + lokasi untuk Word. Bukan HTML dokumen aktif. */
export function SourceViewer({
  run,
  s,
  evidenceById,
  imageBase,
}: {
  run: Run;
  s: Suggestion | null;
  evidenceById: Map<string, Evidence>;
  /** Awalan URL gambar halaman: /api/v1/me/cv-parses/:id (kandidat) atau /api/v1/cv-parses/:id (staf). */
  imageBase: string;
}) {
  if (!s) {
    return (
      <p className="rounded-md border border-dashed border-line p-6 text-sm text-ink-2">
        Pilih "Lihat sumber" pada sebuah usulan untuk melihat bagian CV yang
        mendukungnya.
      </p>
    );
  }
  const evs = s.evidenceIds
    .map((id) => evidenceById.get(id))
    .filter((e): e is Evidence => !!e);
  if (evs.length === 0)
    return (
      <p className="text-sm text-ink-2">Usulan ini tidak memiliki bukti.</p>
    );
  const pageNo = evs[0]!.page;
  const page = run.pages.find((p) => p.no === pageNo);
  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm font-semibold">
        {fieldLabel(s)}: {displayValue(s)}
      </p>
      {evs.map((e) => (
        <blockquote
          key={e.id}
          className="border-l-4 border-primary bg-surface px-3 py-2 text-sm"
        >
          <p className="whitespace-pre-line">{e.snippet}</p>
          <footer className="mt-1 text-[12px] text-ink-2">
            {e.source === "docx"
              ? humanLocator(e.locator)
              : `Halaman ${e.page}${e.source === "ocr" ? ", dibaca dari gambar" : ""}`}
          </footer>
        </blockquote>
      ))}
      {page?.hasImage && page.scale ? (
        <PageCrop
          imageBase={imageBase}
          pageNo={pageNo}
          scale={page.scale}
          evs={evs.filter((e) => e.page === pageNo && e.bbox)}
        />
      ) : null}
    </div>
  );
}

/** Potongan CV yang diperbesar di sekitar bukti (dengan konteks), ditambah opsi halaman penuh. */
function PageCrop({
  imageBase,
  pageNo,
  scale,
  evs,
}: {
  imageBase: string;
  pageNo: number;
  scale: number;
  evs: Evidence[];
}) {
  const [natural, setNatural] = useState<{ w: number; h: number } | null>(null);
  const [full, setFull] = useState(false);
  const src = `${imageBase}/pages/${pageNo}/image`;
  const boxes = evs.map(
    (e) => e.bbox!.map((v) => v * scale) as [number, number, number, number],
  );
  const region = (() => {
    if (!natural || boxes.length === 0) return null;
    const x0 = Math.max(0, Math.min(...boxes.map((b) => b[0])) - 24);
    const x1 = Math.min(natural.w, Math.max(...boxes.map((b) => b[2])) + 24);
    const y0 = Math.max(0, Math.min(...boxes.map((b) => b[1])) - 40);
    const y1 = Math.min(natural.h, Math.max(...boxes.map((b) => b[3])) + 40);
    // lebar minimal 45% halaman agar teks sekitar ikut terbaca
    const minW = natural.w * 0.45;
    const cx0 =
      x1 - x0 < minW
        ? Math.max(0, Math.min(natural.w - minW, (x0 + x1) / 2 - minW / 2))
        : x0;
    const cx1 = x1 - x0 < minW ? cx0 + minW : x1;
    return { x0: cx0, x1: cx1, y0, y1 };
  })();
  const mark = (
    b: [number, number, number, number],
    r: { x0: number; x1: number; y0: number; y1: number },
    i: number,
  ) => (
    <span
      key={i}
      aria-hidden
      className="absolute rounded-sm border-2 border-primary bg-primary/15"
      style={{
        left: `${((b[0] - 3 - r.x0) / (r.x1 - r.x0)) * 100}%`,
        top: `${((b[1] - 3 - r.y0) / (r.y1 - r.y0)) * 100}%`,
        width: `${((b[2] - b[0] + 6) / (r.x1 - r.x0)) * 100}%`,
        height: `${((b[3] - b[1] + 6) / (r.y1 - r.y0)) * 100}%`,
      }}
    />
  );
  return (
    <div className="flex flex-col gap-2">
      {/* gambar tersembunyi hanya untuk mengetahui ukuran asli preview */}
      {!natural ? (
        <img
          src={src}
          alt=""
          className="hidden"
          onLoad={(ev) =>
            setNatural({
              w: ev.currentTarget.naturalWidth,
              h: ev.currentTarget.naturalHeight,
            })
          }
        />
      ) : null}
      {region ? (
        // Border di pembungkus luar: aspect-ratio berlaku pada kotak tanpa border, sehingga persentase posisi
        // gambar dan sorotan memakai dasar yang sama (border di kotak yang sama menggeser sorotan ~1 baris).
        <div className="overflow-hidden rounded-md border border-line bg-white">
          <div
            className="relative w-full overflow-hidden"
            style={{
              aspectRatio: `${region.x1 - region.x0} / ${region.y1 - region.y0}`,
            }}
          >
            <img
              src={src}
              alt={`Potongan halaman ${pageNo} CV di sekitar bagian sumber`}
              className="absolute max-w-none"
              style={{
                width: `${(natural!.w / (region.x1 - region.x0)) * 100}%`,
                left: `${(-region.x0 / (region.x1 - region.x0)) * 100}%`,
                top: `${(-region.y0 / (region.y1 - region.y0)) * 100}%`,
              }}
            />
            {boxes.map((b, i) => mark(b, region, i))}
          </div>
        </div>
      ) : null}
      <div>
        <Button
          size="sm"
          variant="ghost"
          aria-expanded={full}
          onClick={() => setFull((f) => !f)}
        >
          {full ? "Sembunyikan halaman penuh" : "Lihat halaman penuh"}
        </Button>
      </div>
      {full && natural ? (
        <div className="relative overflow-hidden rounded-md border border-line">
          <img
            src={src}
            alt={`Halaman ${pageNo} CV dengan bagian sumber ditandai`}
            className="block w-full"
          />
          {boxes.map((b, i) =>
            mark(b, { x0: 0, x1: natural.w, y0: 0, y1: natural.h }, i),
          )}
        </div>
      ) : null}
    </div>
  );
}
