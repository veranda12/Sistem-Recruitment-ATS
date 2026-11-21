# Known Issues dan Keterbatasan

Daftar jujur hal yang belum dikerjakan, belum diuji, atau dibatasi lingkungan. Status uji aktual: `docs/STATUS.md`.

## Belum diimplementasikan

| Area | Keterangan | Dampak |
|---|---|---|
| Editor template email | Template berversi di kode dengan placeholder allowlist; belum ada UI edit/preview | Perubahan teks email butuh rilis |
| Eksekusi retensi otomatis | Pengaturan retensi tersimpan berversi, belum ada job penghapusan/anonimisasi | Menunggu keputusan legal; jalankan manual |
| Perkakas permintaan subjek data | Prosedur tertulis (`privacy.md`), belum ada UI/perintah | Dilakukan DBA dengan audit |
| Tracing OpenTelemetry | Log JSON + metrik Prometheus ada; tracing belum | Korelasi lintas proses via request ID saja |
| Asynq | Diganti worker outbox PostgreSQL (ADR-0003) | Latensi dispatch ~1 detik |
| Migrasi lamaran berjalan ke versi workflow baru | Lamaran tetap pada versinya; mapping stage + dry-run belum dibuat | Rollback hanya berlaku untuk lamaran baru |
| Board kanban lamaran, kalender interview visual | Tersedia daftar/tabel dan daftar interview; tanpa tampilan kanban/kalender | UX |
| Shortlist dan penugasan recruiter per lamaran | Kolom ada di skema, belum ada endpoint/UI | Gunakan scope penugasan peran |
| Editor peran kustom | Peran sistem tetap; penugasan berscope dapat dikelola | - |
| Tanda tangan artefak rilis | Checksum SHA-256 saja | - |

## Belum diuji / dibatasi lingkungan

| Hal | Alasan |
|---|---|
| Redis (cache publik, limiter global lintas instance) | Redis tidak tersedia di mesin pengembang; seluruh uji berjalan mode degraded |
| Dua replika API + resume lintas instance | Tidak ada staging; desain stateless, skrip uji tersedia |
| Nginx, systemd, TLS, PITR WAL, restic/MinIO | Tidak ada server Linux; artefak dibangun, konfigurasi belum dijalankan |
| Race detector Go | Tidak ada gcc (cgo) di Windows |
| Pemindai malware produksi (ClamAV) | Hanya pemindai dev EICAR |
| Zoom 200% dan screen reader nyata | Diuji: keyboard, skip link, label, live region, viewport 390/1440 lewat Playwright; belum diuji dengan NVDA/VoiceOver |
| Uji beban di staging | Uji beban dijalankan di satu laptop (API, DB, generator bersama); lihat `docs/operations/load-test-report.md`. Tidak ada klaim kapasitas produksi |
