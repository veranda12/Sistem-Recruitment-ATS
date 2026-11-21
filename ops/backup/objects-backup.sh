#!/usr/bin/env bash
# Snapshot object storage (dokumen kandidat, ekspor). Untuk MinIO/S3 aktifkan bucket versioning + object lock;
# untuk penyimpanan filesystem gunakan restic ke repositori terenkripsi (kata sandi restic disimpan terpisah dari server).
set -euo pipefail
: "${RESTIC_REPOSITORY:?set RESTIC_REPOSITORY}" "${RESTIC_PASSWORD_FILE:?set RESTIC_PASSWORD_FILE}"
restic backup /var/lib/ats/objects --tag ats-objects --exclude /var/lib/ats/objects/exports
restic forget --tag ats-objects --keep-daily 14 --keep-weekly 8 --prune
restic check --read-data-subset=5%
