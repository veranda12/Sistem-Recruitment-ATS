# Model Otorisasi

Deny-by-default di service dan di query. Hak akses = gabungan *grant* (permission pada scope). Scope: seluruh organisasi (unit dan lowongan kosong), satu unit, atau satu lowongan. Grant dibaca ulang dari database pada setiap request, sehingga pencabutan berlaku pada permintaan berikutnya. Objek di luar scope dijawab 404 agar ID tidak dapat ditebak; aksi tanpa izin pada objek yang terlihat dijawab 403.

## Matriks peran x permission (seed)

| Permission | Super Admin | HR Admin | Recruiter | Hiring Manager | Examiner | Kandidat |
|---|---|---|---|---|---|---|
| jobs.read / jobs.manage / jobs.publish | ya | ya / ya / ya | read | - | - | - |
| requisitions.read / manage / approve | ya | read, approve | read | read, manage | - | - |
| applications.read / transition / assign | ya | ya | read, transition | read | - | milik sendiri lewat `/me` |
| approvals.decide | ya | - | - | ya | - | - |
| identity.reveal (khusus) | ya | ya | - | - | - | - |
| pii.export (khusus) | ya | ya | - | - | - | - |
| workflows.read / manage / publish (khusus) | ya | ya | - | - | - | - |
| exams.manage / exams.grade | ya | manage | - | - | ya / ya | - |
| results.release (khusus) | ya | ya | - | - | - | - |
| users.manage / roles.manage / settings.manage / credentials.manage (khusus) | ya | - | - | - | - | - |
| audit.read | ya | ya (unit) | - | - | - | - |
| reports.read / reports.export | ya | ya | - | - | - | - |

Scope seed: HR Admin per unit (OPS, TI); Recruiter OPS per unit, Recruiter TI per lowongan; Hiring Manager per lowongan; Super Admin dan Examiner seluruh organisasi.

**Super Admin memegang semua izin** (sejak migrasi 0015), termasuk `applications.read`, `identity.reveal`, dan `pii.export`. Izin baru yang ditambahkan migrasi berikutnya otomatis diberikan lewat trigger `permissions_grant_super_admin`. Konsekuensinya, akun Super Admin dapat membuka data pribadi kandidat. Batasi jumlah pemegangnya, wajibkan MFA (`MFA_REQUIRED_ROLES`), dan pantau audit. Untuk admin sistem tanpa akses data kandidat, buat peran kustom berisi izin administrasi saja.

## Aturan khusus

| Aturan | Implementasi | Tes |
|---|---|---|
| HM hanya requisition dan keputusan miliknya | Grant lowongan; requisition terlihat bila unit dalam scope atau pemohon/HM = dirinya | `TestRequisitionApprovalAndJob`, `TestCandidateToRecruiterSlice` |
| Examiner tidak mendapat NIK/identitas | Tidak punya `applications.read`; review esai memakai kode peserta | `TestExamProtocolOnRealDB` |
| Reveal NIK | `identity.reveal` + alasan >= 10 karakter + audit | `TestCandidateToRecruiterSlice` |
| Self-approval | Penyetuju bukan pelaku yang memindahkan ke tahap persetujuan; requisition tidak disetujui pemohon | `TestApprovalGuardsOnRealDB`, `TestRequisitionApprovalAndJob` |
| Admin aktif terakhir | Lock baris super_admin aktif; cabut/nonaktifkan terakhir -> 409 | `TestAdminAccessSettingsAudit` |
| Tindakan massal | Undangan ujian memeriksa scope tiap lamaran; hasil per item | `TestExamProtocolOnRealDB` |
| File, ekspor, metrik | Dokumen: pemilik atau staf dalam scope lamaran pemilik; ekspor: scope saat request/generate/download; `/metrics` hanya port internal | `TestDocumentUploadScanAndScopedDownload`, `TestReportsMatchFixtureAndExport`, `TestAdminAccessSettingsAudit` |
| Dua unit, dua kandidat, ID valid | Recruiter OPS vs TI, kandidat A vs B memakai ID objek nyata | `TestCandidateToRecruiterSlice` |
