# Paket Kesiapan VAPT

Belum ada VAPT atau audit independen yang dilakukan. Paket ini menyiapkan pengujian oleh pihak ketiga.

## Arsitektur dan scope

- Arsitektur: `docs/architecture/architecture.md`, threat model: `docs/security/threat-model.md`.
- Scope usulan (staging khusus, bukan produksi): `https://<staging>/` (portal + back office + API `/api/v1`). Di luar scope: infrastruktur di luar host aplikasi, layanan pelanggan lain, serangan DoS volumetrik.
- Data staging: seed fiktif + peserta sintetis (`loadseed`); tidak memakai data pelamar nyata.

## Akun uji per peran (staging)

Dibuat oleh admin staging sesuai matriks `docs/product/authorization.md`: Super Admin, HR Admin unit A, HR Admin unit B, Recruiter unit A, Recruiter lowongan B, Hiring Manager, Examiner, Kandidat A, Kandidat B. Dua unit dan dua kandidat diperlukan untuk menguji IDOR lintas scope.

## Checklist kontrol untuk diuji

- [ ] Autentikasi: brute force login (limiter), enumerasi akun (respons identik, registrasi netral), reset sandi sekali pakai, MFA (replay, kode pemulihan sekali pakai)
- [ ] Sesi: fiksasi (rotasi saat login), revoke saat logout/reset/nonaktif, atribut cookie
- [ ] CSRF: mutasi tanpa token / origin asing ditolak
- [ ] Akses: IDOR lamaran, dokumen, ekspor, attempt ujian, interview lintas unit/kandidat (harus 404)
- [ ] Ujian: kebocoran kunci di DTO, replay opId, manipulasi revisi, submit ganda, perpanjangan waktu
- [ ] Upload: tipe berbahaya bersamaran, ukuran, path traversal nama berkas
- [ ] Injeksi: SQL, formula CSV, header email, template placeholder
- [ ] Header keamanan, CSP, HSTS, `/metrics` dan endpoint internal tidak terekspos

## Template bukti dan register temuan

| ID | Judul | Severity (CVSS) | Komponen | Bukti (request/response, requestId) | Status | Penanggung jawab | Target perbaikan | Retest (tanggal, hasil) |
|---|---|---|---|---|---|---|---|---|

Status: Baru -> Dikonfirmasi -> Diperbaiki -> Retest lulus -> Ditutup (atau Diterima sebagai risiko, dengan persetujuan pemilik).
