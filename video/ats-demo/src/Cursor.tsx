// Satu kursor panah; ujungnya tepat di (x, y). Klik ditandai tekanan kecil tanpa pantulan.
export const Cursor: React.FC<{ x: number; y: number; opacity: number; press: number }> = ({ x, y, opacity, press }) => (
  <svg
    width={34}
    height={48}
    viewBox="0 0 17 24"
    style={{ position: "absolute", left: x - 1.5, top: y - 1.5, opacity, scale: press, transformOrigin: "1.5px 1.5px", overflow: "visible" }}
  >
    <path d="M1.5 1.5 L1.5 19.5 L6.2 15.2 L9.4 22.3 L12.4 21 L9.3 14.1 L15.5 14.1 Z" fill="#182638" stroke="#ffffff" strokeWidth={1.4} strokeLinejoin="round" />
  </svg>
);
