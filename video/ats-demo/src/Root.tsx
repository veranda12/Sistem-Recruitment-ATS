import { Composition } from "remotion";
import { AtsProductDemo } from "./AtsProductDemo";
import { DURATION, FPS, HEIGHT, WIDTH } from "./timeline";

export const RemotionRoot: React.FC = () => (
  <Composition id="AtsProductDemo" component={AtsProductDemo} durationInFrames={DURATION} fps={FPS} width={WIDTH} height={HEIGHT} />
);
