import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Host tambahan yang boleh membuka server dev/preview, mis. domain tunnel untuk demo (VITE_ALLOWED_HOSTS=a.ngrok-free.app).
// Kosong = hanya localhost (bawaan Vite).
const allowedHosts = (process.env.VITE_ALLOWED_HOSTS ?? "").split(",").map((h) => h.trim()).filter(Boolean);

// Dev: API diproksikan pada origin yang sama, sama seperti Nginx di produksi.
export default defineConfig({
  preview: { allowedHosts },
  plugins: [react(), tailwindcss()],
  server: {
    port: Number(process.env.VITE_PORT ?? 5173),
    strictPort: true,
    allowedHosts,
    // Keluaran Playwright tidak dipantau: menulis report saat dev server berjalan memicu EBUSY di Windows
    // dan mematikan proses Vite.
    watch: { ignored: ["**/playwright-report/**", "**/test-results/**"] },
    proxy: { "/api": { target: process.env.VITE_API_TARGET ?? "http://127.0.0.1:8080", changeOrigin: false } },
  },
  build: {
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks: { react: ["react", "react-dom", "react-router"] },
      },
    },
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
