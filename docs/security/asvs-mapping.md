# Pemetaan Kontrol Keamanan

Acuan yang dipilih: **OWASP ASVS 4.0.3** (https://owasp.org/www-project-application-security-verification-standard/) dan **OWASP Top 10:2021** (https://owasp.org/Top10/). Versi yang lebih baru mungkin telah terbit; versi acuan untuk audit pelanggan perlu dikonfirmasi. Tabel ini adalah pemetaan internal pengembang, **bukan** hasil audit independen dan bukan klaim kepatuhan.

| Area (ASVS bab / Top 10) | Kontrol yang diimplementasikan | Pengujian | Bukti | Risiko sisa |
|---|---|---|---|---|
| V2 Autentikasi / A07 | Argon2id (m=19 MiB, t=2, p=1) dengan semaphore memori; respons login identik untuk kegagalan; rate limit login; tanpa sandi default (seed menolak produksi, staf baru via tautan atur sandi) | `TestCandidateToRecruiterSlice`, `TestAdminAccessSettingsAudit` | STATUS | MFA belum ada |
| V3 Sesi / A07 | Token 32 byte acak, digest SHA-256 di DB, cookie HttpOnly + SameSite=Lax + Secure (https), idle + absolute expiry, rotasi saat login, revoke saat logout/reset/nonaktif | `TestCandidateToRecruiterSlice`, `TestAdminAccessSettingsAudit` | STATUS | - |
| V4 Akses / A01 | Deny-by-default, scope di query, 404 di luar scope, permission khusus (reveal, pii.export, publish workflow, release result, credentials) | Test integrasi multi-unit multi-kandidat | `docs/product/authorization.md` | - |
| V5 Validasi & encoding / A03 | sqlc terparameter, allowlist filter/sort, validasi per field dengan fieldErrors, React escaping, tanpa `dangerouslySetInnerHTML`, CSP `script-src 'self'`, placeholder email allowlist, netralisasi formula CSV | Unit + integrasi | `TestSafeCell`, `ParseImport` | Rich text tidak didukung (sengaja) |
| V6 Kriptografi / A02 | AES-256-GCM nonce acak + AAD, HMAC-SHA256 blind index kunci terpisah, kunci divalidasi saat start (berbeda satu sama lain) | `nik_test.go` | - | Rotasi kunci manual |
| V7 Log / A09 | JSON log dengan request ID, route template (bukan URL mentah), tanpa NIK/jawaban/cookie/token/URL bertanda tangan; audit append-only | Peninjauan kode `AccessLog`, trigger audit | `TestAdminAccessSettingsAudit` (DELETE audit ditolak) | - |
| V8 Data / A02 | DTO peserta tanpa kunci; kolom rahasia `json:"-"` di model; `Cache-Control: no-store` | `TestExamProtocolOnRealDB` | - | - |
| V12 Berkas / A04 | Batas ukuran per route, signature byte, nama acak, karantina, status pindai, unduh bertanda tangan 60 s dengan cek scope | `TestDocumentUploadScanAndScopedDownload` | - | Butuh pemindai produksi |
| V13 API / A05 | CSRF token terikat sesi + cek Origin + Sec-Fetch-Site, body limit, error terstruktur tanpa detail internal, OpenAPI | `TestCandidateToRecruiterSlice` (403 tanpa token) | `contracts/openapi.yaml` | - |
| V14 Konfigurasi / A05 | Konfigurasi tervalidasi saat start, HSTS + header keamanan, `/metrics` internal, systemd hardening, `RATE_LIMIT_SCALE` wajib 1 di produksi | `TestAdminAccessSettingsAudit` (`/metrics` 404) | `ops/` | Konfigurasi Nginx/systemd belum divalidasi di server nyata |
| A06 Komponen | Lockfile npm + go.sum, inventaris lisensi, pemindaian kerentanan | `govulncheck`, `npm audit` | `docs/handover/dependencies.md` | Lihat hasil pindai |
| A08 Integritas | Checksum artefak rilis, manifest backup SHA-256, digest receipt diverifikasi DB | Drill restore | `docs/operations/backup-restore.md` | Tanda tangan artefak belum ada |
| A10 SSRF | Tidak ada fetch URL dari input pengguna; rule DSL tanpa callback URL | Validasi workflow | `definition_test.go` | - |
