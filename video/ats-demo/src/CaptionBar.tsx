import { interpolate, useCurrentFrame } from "remotion";
import { CAPTION_BAND } from "./timeline";
import { FONT_FAMILY } from "./fonts";
import { color } from "./theme";

// Caption di satu area tetap (pita bawah), maksimal dua baris. Berganti bersama hard cut, tanpa gerak.
export const CaptionText: React.FC<{ text: string }> = ({ text }) => {
  return (
    <div
      style={{
        position: "absolute",
        left: CAPTION_BAND.padX,
        top: CAPTION_BAND.y,
        height: CAPTION_BAND.h,
        width: 1920 - 2 * CAPTION_BAND.padX,
        display: "flex",
        alignItems: "center",
        fontFamily: FONT_FAMILY,
        fontWeight: 600,
        fontSize: 46,
        lineHeight: 1.25,
        color: color.ink,
      }}
    >
      {text}
    </div>
  );
};

export const Closing: React.FC<{ text: string }> = ({ text }) => {
  const frame = useCurrentFrame();
  return (
    <div
      style={{
        position: "absolute",
        right: CAPTION_BAND.padX,
        top: CAPTION_BAND.y,
        height: CAPTION_BAND.h,
        display: "flex",
        alignItems: "center",
        fontFamily: FONT_FAMILY,
        fontWeight: 600,
        fontSize: 26,
        color: color.ink2,
        opacity: interpolate(frame, [0, 15], [0, 1], { extrapolateRight: "clamp" }),
      }}
    >
      {text}
    </div>
  );
};
