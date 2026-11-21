#!/usr/bin/env bash
# Smoke pascadeploy melalui Nginx: halaman portal, fallback SPA, readiness API, lowongan publik, /api tidak ter-fallback.
set -euo pipefail
BASE="${1:-https://localhost}"
CURL="curl -fsS --max-time 10 ${SMOKE_INSECURE:+-k}"

$CURL "$BASE/" | grep -q '<div id="root">' && echo "OK portal"
$CURL "$BASE/lowongan/contoh-deep-link" | grep -q '<div id="root">' && echo "OK fallback SPA"
$CURL "$BASE/api/v1/readyz" | grep -q '"database":"ok"' && echo "OK readyz"
$CURL "$BASE/api/v1/jobs" | grep -q '"items"' && echo "OK lowongan publik"
code=$(curl -s -o /dev/null -w '%{http_code}' ${SMOKE_INSECURE:+-k} "$BASE/api/v1/tidak-ada")
[ "$code" = "404" ] && echo "OK /api tidak di-fallback ke SPA ($code)"
code=$(curl -s -o /dev/null -w '%{http_code}' ${SMOKE_INSECURE:+-k} "$BASE/metrics")
[ "$code" != "200" ] && echo "OK /metrics tidak publik ($code)"
