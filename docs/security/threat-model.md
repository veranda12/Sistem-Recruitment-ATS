# Threat Model (defensif)

Ruang lingkup: aplikasi ATS sebagaimana di repository ini, dipasang on-premise di infrastruktur pelanggan. Bukan hasil VAPT; tidak ada klaim sertifikasi atau kepatuhan.

## Trust boundary

```
[Kandidat (internet, perangkat tak terkelola)] --HTTPS--> [Nginx] --loopback--> [API replika] --> [PostgreSQL]
[Staf (jaringan kantor/internet)]              --HTTPS--> [Nginx]                     |            ^
                                                                                      v            |
                                                         [Worker] -- SMTP relay pelanggan ---------+
                                                            |-- pemindai malware (eksternal) -- [Object storage privat]
[Operator (SSH/console)] --> systemd, env file, backup, kunci
```

| Boundary | Data melintas | Ancaman utama | Kontrol |
|---|---|---|---|
| Browser kandidat -> Nginx | Kredensial, NIK, jawaban ujian, berkas | Credential stuffing, CSRF, XSS, upload berbahaya, scraping lowongan | Limiter edge + aplikasi, Argon2id, cookie HttpOnly/SameSite, token CSRF + cek Origin, CSP ketat tanpa inline script, signature berkas + karantina + pindai, cache publik pendek |
| Browser staf -> Nginx | Akses data pelamar, keputusan | Pengambilalihan akun, eskalasi lintas unit, IDOR | Scope per grant di query, 404 di luar scope, reveal NIK beralasan + audit, idle timeout staf 30 menit. **MFA belum ada (lihat risiko sisa)** |
| Nginx -> API | Request | Spoof header IP | `TRUSTED_PROXIES` eksplisit, X-Forwarded-For hanya dari Nginx |
| API -> PostgreSQL | Semua data | SQL injection, kebocoran koneksi, lock berkepanjangan | Query terparameter (sqlc), sort/filter allowlist, `statement_timeout`, `lock_timeout`, pool terbatas |
| Worker -> SMTP / pemindai | Email, berkas | Pengiriman ganda, header injection, pemindai mati | Outbox + dedup + status ambigu, subject satu baris, placeholder allowlist; tanpa pemindai dokumen tetap `pending` |
| Operator -> server | Kunci, backup | Kebocoran kunci, backup tidak terenkripsi | Env file 0640 root:ats, kunci terpisah dari backup, volume backup terenkripsi (prosedur) |

## Aset dan ancaman spesifik

| Aset | Ancaman | Kontrol | Bukti |
|---|---|---|---|
| NIK | Bocor dari DB/backup/log | AES-256-GCM + AAD org, blind index HMAC berkunci terpisah, tersamar di UI, reveal beralasan + audit, tidak di-log | `nik_test.go`, `TestCandidateToRecruiterSlice` |
| Integritas jawaban ujian | ACK palsu, jawaban hilang, submit ganda, perpanjangan waktu | ACK setelah commit, receipt per opId, revisi + konflik eksplisit, deadline jam DB setelah lock, satu submission (PK), digest snapshot dihitung DB | `TestExamProtocolOnRealDB`, `TestExamBranchesDeadlineAndExpiry`, k6 + `reconcile.sql` |
| Kerahasiaan soal/kunci | Kunci bocor ke peserta | DTO peserta tanpa kunci/bobot/rubric/seed/ID opsi asli, `no-store` | `TestExamProtocolOnRealDB` (cek string) |
| Keputusan rekrutmen | Manipulasi tahap, self-approval | Edge workflow + guard, expected version, kuorum, larangan self-approval, histori append-only | `TestApprovalGuardsOnRealDB` |
| Akun staf | Pengambilalihan | Argon2id, rate limit login, rotasi sesi saat login, revoke saat reset/nonaktif | `TestAdminAccessSettingsAudit` |
| Ekspor | Formula injection, kebocoran lintas scope | `SafeCell`, scope 3 kali, tautan 60 s terikat pengguna, TTL 24 jam | `TestReportsMatchFixtureAndExport` |
| Ketersediaan saat ujian | Overload, Redis mati | Prioritas operasi (deadline per kelas), batas batch/body, limiter lokal saat Redis mati, Retry-After | k6 baseline; mode degraded aktif di semua uji (Redis tidak ada) |

## Risiko sisa (perlu keputusan/pekerjaan)

| Risiko | Status | Usulan |
|---|---|---|
| MFA staf berprivilege dan recovery code | **Belum diimplementasikan** | Prioritas tinggi sebelum go-live |
| Pemindai malware nyata | Integrasi via `SCAN_COMMAND`; dev memakai pemindai EICAR-saja | Pasang ClamAV (`clamdscan`) di produksi |
| Redis lintas instance | Tidak diuji (tidak tersedia di lingkungan dev) | Uji di staging dengan 2 replika + Redis |
| Rotasi kunci NIK | Prosedur tertulis (`docs/operations/key-rotation.md`), belum ada perkakas otomatis | Kembangkan perintah rotasi bila diperlukan |
| Race detector Go | Tidak dapat dijalankan (tanpa gcc di Windows) | Jalankan `go test -race` di CI Linux |
| Proctoring / anti-kecurangan | Di luar lingkup | Keputusan pelanggan |
| Template email dapat diedit admin | Template di kode (versi), belum ada editor | Bila dibutuhkan, tambah editor dengan validasi placeholder yang sudah ada |
