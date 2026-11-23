# Known Issues dan Keterbatasan

Daftar jujur hal yang belum dikerjakan, belum diuji, atau dibatasi lingkungan. Status uji aktual: `docs/STATUS.md`.

## Belum diimplementasikan

| Area | Keterangan | Dampak |
|---|---|---|
| Tracing OpenTelemetry | Log JSON + metrik Prometheus ada; tracing belum | Korelasi lintas proses via request ID saja |
| Asynq | Diganti worker outbox PostgreSQL (ADR-0003) | Latensi dispatch ~1 detik |
| Tanda tangan artefak rilis | Checksum SHA-256 saja | - |
| Login SSO (OIDC/SAML) | Tidak dikerjakan pada putaran ini | Staf masuk dengan email + sandi + MFA |
| Pengiriman WhatsApp sungguhan | Kanal, preferensi kandidat (opt-in), template, dan log siap; pengirim hanya mode `file` (dev). `WHATSAPP_MODE=meta` ditolak konfigurasi | Butuh akun WhatsApp Business API + template yang disetujui Meta |
| Proctoring kamera | Hanya log perilaku (pindah tab, keluar layar penuh, tempel teks, kehilangan fokus) sebagai catatan untuk penguji, bukan bukti kecurangan | - |
| Sebagian endpoint admin di kontrak OpenAPI | Endpoint baru (template pesan, peran kustom, migrasi workflow, papan/kalender, privasi) sudah ada di `contracts/openapi.yaml`; endpoint admin lama (pengguna, penugasan, pengaturan, audit, daftar peran) belum dideskripsikan, meski teruji integrasi + E2E | Klien pihak ketiga perlu membaca handler |
| Retensi otomatis | Tersedia, **mati secara bawaan**; aktifkan setelah keputusan legal (`docs/product/privacy.md`) | - |

## Belum diuji / dibatasi lingkungan

| Hal | Alasan |
|---|---|
| Redis dengan dua instance API (cache publik, limiter global) | Redis tersedia di dev (manual, `redis-start.ps1`) dan diuji dengan antrean CV; skenario dua instance API belum diuji |
| Dua replika API + resume lintas instance | Tidak ada staging; desain stateless, skrip uji tersedia |
| Nginx, systemd, TLS, PITR WAL, restic/MinIO | Tidak ada server Linux; artefak dibangun, konfigurasi belum dijalankan |
| Pemindai malware (ClamAV) di produksi | Di dev Windows: ClamAV 1.5.5 + clamd teruji lewat `CommandScanner` (bersih, terinfeksi via signature lokal, berkas tak terbaca -> tertahan). `main.cvd` belum terunduh karena rate limit CDN (hanya `daily` + `bytecode`); belum diuji di Linux dengan `--fdpass` |
| Zoom 200% dan screen reader nyata | Diuji: keyboard, skip link, label, live region, viewport 390/1440 lewat Playwright; belum diuji dengan NVDA/VoiceOver |
| Uji beban di staging | Uji beban dijalankan di satu laptop (API, DB, generator bersama); lihat `docs/operations/load-test-report.md`. Tidak ada klaim kapasitas produksi |
