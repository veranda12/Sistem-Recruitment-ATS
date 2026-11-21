#!/usr/bin/env bash
# Membangun artefak rilis Linux: binary statis (api, worker, migrate, seed) + aset frontend ber-hash.
# Hasil: dist/ats-<versi>-linux-amd64.tar.gz dan .sha256. Jalankan dari root repo.
set -euo pipefail

VERSION="${1:-$(git describe --tags --always --dirty 2>/dev/null || date +%Y%m%d%H%M)}"
OUT="dist/ats-${VERSION}-linux-amd64"
rm -rf "$OUT" && mkdir -p "$OUT/bin" "$OUT/web" "$OUT/ops"

echo "==> backend (CGO_ENABLED=0, GOOS=linux, GOARCH=amd64)"
( cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o "../$OUT/bin/" ./cmd/... )

echo "==> frontend"
( cd frontend && npm ci --no-audit --no-fund && npm run build )
cp -r frontend/dist/. "$OUT/web/"

cp -r ops/nginx ops/systemd ops/linux ops/monitoring ops/backup "$OUT/ops/"
cp .env.example "$OUT/ats.env.example"
echo "$VERSION" > "$OUT/VERSION"

tar -C dist -czf "${OUT}.tar.gz" "$(basename "$OUT")"
( cd dist && sha256sum "$(basename "$OUT").tar.gz" > "$(basename "$OUT").tar.gz.sha256" )
echo "Artefak: ${OUT}.tar.gz"
