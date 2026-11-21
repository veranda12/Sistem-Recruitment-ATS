#!/usr/bin/env bash
# Deploy satu rilis di server Linux (dijalankan sebagai root di server target).
#   1. ekstrak ke /opt/ats/releases/<versi>
#   2. migrasi SEKALI per rilis (unit oneshot ats-migrate) dengan binary rilis baru
#   3. alihkan symlink /opt/ats/current, restart replika API bergiliran, lalu worker
#   4. smoke pascadeploy; gagal -> kembalikan symlink ke rilis sebelumnya (rollback binary)
# Migrasi wajib kompatibel mundur (expand/backfill/contract) agar rollback binary aman tanpa down migration.
set -euo pipefail

TARBALL="$1"
REPLICAS="${REPLICAS:-8080 8082}"
BASE=/opt/ats
NAME="$(basename "$TARBALL" .tar.gz)"
DEST="$BASE/releases/$NAME"

sha256sum -c "${TARBALL}.sha256"
mkdir -p "$BASE/releases"
tar -C "$BASE/releases" -xzf "$TARBALL"
chown -R root:ats "$DEST" && chmod -R o-rwx "$DEST"

PREV="$(readlink -f "$BASE/current" 2>/dev/null || true)"
ln -sfn "$DEST" "$BASE/next"

echo "==> migrasi (sekali)"
ATS_RELEASE="$DEST" systemctl start ats-migrate.service
systemctl is-failed --quiet ats-migrate.service && { echo "migrasi gagal; rilis tidak diaktifkan"; exit 1; }

ln -sfn "$DEST" "$BASE/current"
rsync -a --delete "$DEST/web/" /var/www/ats/

for port in $REPLICAS; do
  echo "==> restart ats-api@$port"
  systemctl restart "ats-api@$port"
  for i in $(seq 1 30); do curl -fsS "http://127.0.0.1:$port/readyz" >/dev/null && break; sleep 1; done
done
systemctl restart ats-worker

if ! "$DEST/ops/linux/smoke.sh" "https://${PUBLIC_HOST:-localhost}"; then
  echo "smoke gagal -> rollback binary ke $PREV"
  if [ -n "$PREV" ]; then
    ln -sfn "$PREV" "$BASE/current"
    rsync -a --delete "$PREV/web/" /var/www/ats/
    for port in $REPLICAS; do systemctl restart "ats-api@$port"; done
    systemctl restart ats-worker
  fi
  exit 1
fi
echo "Rilis $NAME aktif."
