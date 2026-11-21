# Runbook Operasional

## Kesehatan

| Endpoint | Arti |
|---|---|
| `GET /readyz` (API, loopback) | 200 bila PostgreSQL dapat dihubungi. `dependencies.redis`: `ok`, `degraded` (dikonfigurasi tapi mati), `disabled`. Redis bukan syarat ready. |
| `GET /healthz` (API) | Proses hidup |
| `GET http://127.0.0.1:8081/healthz` (worker) | `outbox_pending`, `outbox_dead`, `oldest_pending_seconds`; terpisah dari readiness API ujian |

## Mode degraded: Redis mati

| Fungsi | Perilaku | Keputusan |
|---|---|---|
| Cache lowongan publik | Mati; dibaca langsung dari DB | Aman (TTL hanya 30 s) |
| Rate limit | Lokal per replika, kuota dibagi 2 | Nginx `limit_req` tetap aktif sebagai lapis edge |
| Login/registrasi/reset | Tetap melayani dengan limiter lokal | Diterima |
| Ujian (start/save/submit) | Tidak terpengaruh: Redis tidak di jalur durability | Diterima |
| Ekspor data pribadi | Ditolak 503 bila `STRICT_SENSITIVE_LIMITS=true` | Endpoint sensitif menolak bila perlindungan global tidak tersedia |
| Outbox | Tetap berjalan (worker membaca PostgreSQL langsung, ADR-0003) | - |

## Antrean dan dead-letter

- Halaman **Komunikasi dan antrean** (`/staf/komunikasi`) menampilkan backlog, dead-letter, dan riwayat email. Tombol *Coba lagi* = replay (teraudit `ops.outbox_replay`).
- Status email `ambiguous` = proses terhenti setelah mulai kirim dan sebelum hasil tercatat. **Jangan** kirim ulang otomatis; cek log SMTP relay lalu tandai manual.
- Alert `ATSOutboxLag` > 2 menit: cek worker berjalan (`systemctl status ats-worker`), log error, koneksi DB.

## Ujian sedang berlangsung

- Monitoring per jadwal: `/staf/ujian/:id` (agregat satu query; refresh manual).
- Peserta melaporkan "Menunggu koneksi": jawaban yang belum diterima server tersimpan di perangkat dan terkirim saat tersambung. Waktu tetap berjalan menurut server.
- Peserta melewati deadline saat offline: attempt difinalisasi worker expiry dengan jawaban committed terakhir; peserta mendapat bukti dengan alasan *waktu habis*. Keberatan: catat kode bukti, tinjau `exam_save_receipts` dan audit, keputusan di luar sistem (proses keberatan teraudit).
- Jangan me-restart API saat puncak submit kecuali perlu; restart aman (drain), tetapi menambah latensi.

## Insiden umum

| Gejala | Langkah |
|---|---|
| 5xx meningkat | `journalctl -u ats-api@* -o cat | jq 'select(.level=="ERROR")'`, cari `request_id` dari laporan pengguna |
| Pool DB jenuh (`ATSPoolSaturated`) | Cek query lambat (`pg_stat_statements`), lock (`pg_locks`), jangan langsung menaikkan `DB_MAX_CONNS` (anggaran koneksi) |
| Login ditolak 429 massal | Periksa apakah semua pengguna di balik satu NAT; sesuaikan zona `limit_req` Nginx dan limiter aplikasi |
| Dokumen tetap "menunggu pemindaian" | Cek `SCAN_COMMAND` dan ClamAV; event gagal masuk dead-letter setelah batas percobaan |
| Pengguna kehilangan perangkat MFA | Admin pengguna: halaman Pengguna -> *Reset MFA* dengan alasan verifikasi identitas (teraudit, sesi target dicabut) |
