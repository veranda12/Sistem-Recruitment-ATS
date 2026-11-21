# Model Otorisasi

Deny-by-default di service dan di query. Hak akses = gabungan *grant* (permission pada scope). Scope: seluruh organisasi (unit dan lowongan kosong), satu unit, atau satu lowongan. Grant dibaca ulang dari database pada setiap request, sehingga pencabutan berlaku pada permintaan berikutnya. Objek di luar scope dijawab 404 agar ID tidak dapat ditebak; aksi tanpa izin pada objek yang terlihat dijawab 403.

## Matriks peran x permission (seed)

| Permission | Super Admin | HR Admin | Recruiter | Hiring Manager | Examiner | Kandidat |
|---|---|---|---|---|---|---|
| jobs.read / jobs.manage / jobs.publish | read | ya / ya / ya | read | - | - | - |
| requisitions.read / manage / approve | - | read, approve | read | read, manage | - | - |
| applications.read / transition / assign | - | ya | read, transition | read | - | milik sendiri lewat `/me` |
| approvals.decide | - | - | - | ya | - | - |
| identity.reveal (khusus) | - | ya | - | - | - | - |
| pii.export (khusus) | - | ya | - | - | - | - |
| workflows.read / manage / publish (khusus) | ya | ya | - | - | - | - |
| exams.manage / exams.grade | - | manage | - | - | ya / ya | - |
| results.release (khusus) | - | ya | - | - | - | - |
| users.manage / roles.manage / settings.manage / credentials.manage (khusus) | ya | - | - | - | - | - |
| audit.read | ya | ya (unit) | - | - | - | - |
| reports.read / reports.export | read | ya | - | - | - | - |

Scope seed: HR Admin per unit (OPS, TI); Recruiter OPS per unit, Recruiter TI per lowongan; Hiring Manager per lowongan; Super Admin dan Examiner seluruh organisasi.

**Super Admin tidak otomatis membuka identitas**: tidak memiliki `applications.read` maupun `identity.reveal`.

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
