# Deployment Linux dan Rollback

> Konfigurasi di bawah ditulis untuk Ubuntu 22.04/24.04 atau RHEL 9 dengan systemd dan Nginx >= 1.25. **Belum divalidasi di server Linux nyata** dalam pekerjaan ini (lingkungan pengembang Windows tanpa Docker aktif). Artefak rilis telah dibangun dan diperiksa (binary ELF statis). Jalankan `nginx -t` dan uji staging sebelum produksi.

## Topologi contoh

```
Nginx :443 (TLS, aset, limit edge) -> ats-api@8080, ats-api@8082 (loopback) -> PostgreSQL 17
                                      ats-worker (outbox, email, expiry, ekspor, pindai) -> SMTP relay, ClamAV
Prometheus (loopback 18080/18082/19081, postgres_exporter 9187) -> Grafana
```

## Persiapan server (sekali)

1. PostgreSQL 17: buat role aplikasi (tanpa superuser) dan role migrasi (pemilik skema). Atur `max_connections` sesuai `docs/architecture/architecture.md` (anggaran koneksi). Aktifkan archive WAL (`ops/backup/postgresql-archive.conf`) dan `pg_stat_statements`.
2. `sudo ops/linux/install-server.sh`: user sistem `ats`, direktori `/opt/ats`, `/var/lib/ats`, `/etc/ats/ats.env` (0640 root:ats), unit systemd, konfigurasi Nginx (`nginx -t`).
3. Isi `/etc/ats/ats.env` dari `ats.env.example`:
   - `APP_ENV=production`, `PUBLIC_ORIGIN=https://<domain>` (wajib https), `TRUSTED_PROXIES=127.0.0.1`
   - Kunci baru: `head -c 32 /dev/urandom | base64` untuk `SESSION_KEY`, `NIK_ENC_KEY`, `NIK_INDEX_KEY` (ketiganya berbeda). Simpan salinan di brankas terpisah dari backup.
   - `DB_MAX_CONNS` per replika, `REDIS_URL`, `SMTP_ADDR`, `MAIL_FROM`, `OBJECT_DIR=/var/lib/ats/objects`, `SCAN_COMMAND="clamdscan --no-summary --fdpass"`
   - `MFA_REQUIRED_ROLES` (default produksi `super_admin,hr_admin`), `RATE_LIMIT_SCALE` harus 1 (konfigurasi menolak nilai lain di produksi)
   - Kredensial migrasi di `/etc/ats/migrate.env` (`DATABASE_URL` role migrasi), 0640 root:ats.
4. **Tidak ada seed di produksi** (perintah seed menolak `APP_ENV=production`). Akun Super Admin pertama dibuat DBA dengan prosedur di `docs/handover/admin-manual.md`.

## Rilis

```bash
# Di mesin build (repo bersih, tag versi):
ops/linux/build-release.sh v1.0.0            # dist/ats-v1.0.0-linux-amd64.tar.gz + .sha256
# Di server:
sudo PUBLIC_HOST=karier.contoh.id ops/linux/deploy.sh /tmp/ats-v1.0.0-linux-amd64.tar.gz
```

`deploy.sh`: verifikasi checksum -> ekstrak ke `/opt/ats/releases/<versi>` -> **migrasi sekali** (`ats-migrate.service`, binary rilis baru) -> alihkan symlink `current` -> salin aset web -> restart replika API bergiliran dengan menunggu `/readyz` -> restart worker -> smoke pascadeploy (`ops/linux/smoke.sh`: portal, fallback SPA, readyz, lowongan, `/api` tidak di-fallback, `/metrics` tidak publik).

## Rollback

- Otomatis: bila smoke gagal, `deploy.sh` mengembalikan symlink ke rilis sebelumnya dan me-restart layanan.
- Manual: `ln -sfn /opt/ats/releases/<versi-lama> /opt/ats/current && systemctl restart ats-api@8080 ats-api@8082 ats-worker`.
- Skema **tidak** di-rollback. Migrasi harus kompatibel mundur (expand/backfill/contract) sehingga binary lama tetap berjalan pada skema baru. Perubahan destruktif (drop kolom) hanya di rilis berikutnya setelah tidak ada binary yang memakainya.

## Graceful shutdown

API: SIGTERM -> berhenti menerima koneksi, menguras request berjalan maks 20 s (`TimeoutStopSec=30`). Worker: berhenti mengklaim outbox, menyelesaikan pekerjaan dalam lease 60 s (`TimeoutStopSec=75`). Event yang belum selesai diklaim ulang setelah lease habis (delivery minimal sekali, consumer idempoten).

## Dua replika dan resume ujian

API stateless (sesi di PostgreSQL, tanpa sticky session). Resume attempt dapat dilayani replika mana pun. **Belum dibuktikan di staging dua replika**; uji yang disarankan: jalankan `tests/load/exam.js` profil baseline ke Nginx dengan dua replika, matikan satu replika di tengah run, pastikan `reconcile.sql` nol selisih.

## Observability

- `/metrics` hanya di loopback (`METRICS_ADDR`, `WORKER_METRICS_ADDR`). Label: route template, method, kelas status; tanpa user/attempt ID.
- `ops/monitoring/prometheus.yml`, `alerts.yml`, `grafana/ats-dashboard.json` (rate, latency p95 per route ujian, error, pool, lock wait via postgres_exporter, outbox lag/backlog/dead, commit save/submit, memori).
- Log JSON ke journald (`journalctl -u ats-api@8080 -o cat`), berisi request ID; tanpa NIK, jawaban, cookie, token, atau URL bertanda tangan.
- Tracing OpenTelemetry: **belum diimplementasikan** (direncanakan; konfigurasi melalui env standar OTEL_*).
