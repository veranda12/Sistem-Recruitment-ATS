import { AbsoluteFill, Sequence } from "remotion";
import { useManifest } from "./footage";
import { loadFonts } from "./fonts";
import { ShotView } from "./ShotView";
import { CaptionText, Closing } from "./CaptionBar";
import { CAPTIONS, CAPTION_BAND, CLOSING, DURATION, SHOTS, STAGE } from "./timeline";
import { color } from "./theme";

loadFonts();

export const AtsProductDemo: React.FC = () => {
  const manifest = useManifest();
  if (!manifest) return null;
  const byId = new Map(manifest.shots.map((s) => [s.id, s]));
  const view = manifest.source.viewport;
  return (
    <AbsoluteFill style={{ backgroundColor: color.bg }}>
      {/* Layar aplikasi: hard cut antar shot. */}
      <AbsoluteFill style={{ width: STAGE.w, height: STAGE.h }}>
        {SHOTS.map((shot) => (
          <Sequence key={shot.still} name={shot.still} from={shot.from} durationInFrames={shot.dur} layout="none">
            <ShotView shot={shot} data={byId.get(shot.still)!} view={view} />
          </Sequence>
        ))}
      </AbsoluteFill>
      <div
        style={{
          position: "absolute",
          left: 0,
          top: CAPTION_BAND.y,
          width: "100%",
          height: CAPTION_BAND.h,
          backgroundColor: color.surface,
          borderTop: `2px solid ${color.line}`,
        }}
      />
      {CAPTIONS.map((c) => (
        <Sequence key={c.from} name={`Caption: ${c.text}`} from={c.from} durationInFrames={c.dur} layout="none">
          <CaptionText text={c.text} />
        </Sequence>
      ))}
      <Sequence name="Penutup" from={CLOSING.from} durationInFrames={DURATION - CLOSING.from} layout="none">
        <Closing text={CLOSING.text} />
      </Sequence>
    </AbsoluteFill>
  );
};
