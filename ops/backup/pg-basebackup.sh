#!/usr/bin/env bash
# Base backup fisik harian untuk PITR. Membutuhkan archive WAL aktif (lihat postgresql-archive.conf).
# Jalankan sebagai user postgres dari cron/systemd timer. Simpan ke volume terenkripsi terpisah dari server DB.
set -euo pipefail
DEST="${BACKUP_ROOT:-/backup/ats}/base/$(date +%Y%m%dT%H%M%S)"
mkdir -p "$DEST"
pg_basebackup -D "$DEST" -Ft -z -X stream -c fast --checkpoint=fast -l "ats-$(date +%F)" --manifest-checksums=SHA256
# Verifikasi checksum manifest base backup (PostgreSQL 13+).
pg_verifybackup -n "$DEST" >/dev/null 2>&1 || pg_verifybackup "$DEST" || { echo "verifikasi base backup gagal"; exit 1; }
# Retensi: simpan 14 base backup terakhir (sesuaikan dengan kebijakan retensi yang disetujui).
ls -1d "${BACKUP_ROOT:-/backup/ats}"/base/* | sort | head -n -14 | xargs -r rm -rf
echo "Base backup: $DEST"
