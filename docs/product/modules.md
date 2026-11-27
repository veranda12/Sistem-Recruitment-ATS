# Spesifikasi Modul

Ringkas per modul: layar, aktor, precondition, API (relatif `/api/v1`, detail payload/error di `contracts/openapi.yaml`), tabel/constraint, scope akses, validasi, kejadian bersamaan, dan tes penerimaan yang benar-benar ada. Status uji aktual ada di `docs/STATUS.md`.

## Karier publik

| Aspek | Isi |
|---|---|
| Layar | `/` beranda + daftar + filter (kata kunci, unit, lokasi, jenis kerja, track) di URL; label track di kartu dan detail; `/lowongan/:slug` detail + CTA Lamar |
| Aktor | Publik, kandidat |
| API | `GET /org`, `GET /jobs?unit&location&type&track&q&cursor&limit<=50`, `GET /jobs/{slug}` |
| Data | `job_openings` (status `draft/published/closed`, `track`), indeks parsial lowongan terbit |
| Akses | Hanya `published` dan `closed` (detail); draft = 404 |
| Validasi | Filter allowlist (422 untuk nilai lain), kata kunci <= 100 karakter, cursor opaque |
| Bersamaan | Cache publik TTL 30 s di Redis bila ada, diinvalidasi saat publish/close; status apply selalu diperiksa di transaksi |
| Tes | `TestCandidateToRecruiterSlice`; E2E `portal karier: daftar, filter di URL...`; `TestPublicJobsTrackFilter`, `TestDraftJobTrackHiddenFromPublic`; E2E `track.spec.ts` (filter track di URL, ponsel 390 px) |

## Akun dan profil

| Aspek | Isi |
|---|---|
| Layar | `/daftar`, `/verifikasi-email`, `/masuk`, `/lupa-sandi`, `/atur-ulang-sandi`, `/akun/profil` (identitas, pendidikan, pengalaman, dokumen), `/akun` (lamaran + timeline) |
| API | `POST /auth/register|login|logout|verify-email|resend-verification|forgot-password|reset-password`, `GET /auth/session`, `GET|PUT /me/profile`, `GET|POST /me/documents`, `DELETE /me/documents/{id}` |
| Data | `users`, `sessions`, `email_tokens` (digest), `candidate_profiles` (version), `candidate_identities` (`UNIQUE(org_id, nik_blind_index)`), `identity_claims` |
| Akses | `/me/*` hanya pemilik; dokumen hanya pemilik atau staf dalam scope lamaran pemilik |
| Validasi | NIK 16 digit ASCII (spasi/titik/strip diabaikan); sandi >= 10 huruf+angka; profil lengkap (telepon, tanggal lahir, kota, pendidikan) sebelum melamar; dokumen <= 5 MiB, signature PDF/DOCX/JPG/PNG |
| Bersamaan | Registrasi serentak NIK sama -> satu master, sisanya klaim manual; edit profil basi -> 409 |
| Tes | `TestConcurrentSameNIKRegistration`, `TestCandidateToRecruiterSlice`, `TestDocumentUploadScanAndScopedDownload`; E2E registrasi->verifikasi->profil->apply, unggah CV |

## ATS (requisition, lowongan, lamaran, interview, keputusan)

| Aspek | Isi |
|---|---|
| Layar | `/staf/permintaan` (track, alasan, line, target tanggal masuk), `/staf/lowongan` (track, kuota, pilih permintaan yang disetujui, ubah kuota), `/staf/lamaran` (tabel / kartu di ponsel, filter track), `/staf/lamaran/:id` (snapshot, histori, interview, scorecard, dokumen, tindakan) |
| Aktor | HR Admin, Recruiter, Hiring Manager |
| API | `GET|POST /requisitions` (filter `unitCode`), `POST /requisitions/{id}/{submit|cancel|approve|reject}`, `GET|POST /staff/jobs` (track, quota, requisitionId), `PATCH /staff/jobs/{id}` (hanya kuota, `expectedVersion`), `POST /staff/jobs/{id}/{publish|close}`, `GET /applications?track=`, `GET /applications/{id}`, `POST /applications/{id}/transitions|approvals|reveal-identity|interviews|decision-notice`, `POST /interviews/{id}/reschedule|cancel|evaluations`, `GET /staff/interviews` |
| Data | `requisitions` (track, line, target_join_date, reason_category; `UNIQUE (org_id, id, track)`), `job_openings` (track, quota; FK komposit ke requisition agar track sama), `job_openings.requisition_id`, `job_applications` (version), `transition_history`, `approvals`, `interviews`, `interview_panelists` (exclusion constraint), `interview_changes`, `evaluations` |
| Akses | Scope unit/lowongan per permission; di luar scope = 404; HM hanya lowongan yang ditugaskan + requisition miliknya |
| Validasi | Transisi: `expectedVersion`, edge harus ada di versi workflow lamaran, guard rule; tolak wajib alasan; interview: zona IANA, 15-480 menit, 1-10 interviewer aktif; track dari allowlist (OPERATOR, STAFF, MT_S1_D4); total kuota lowongan draft/published per requisition tidak melebihi headcount (422 di aplikasi); lowongan closed tidak bisa diubah kuotanya; target tanggal masuk tidak boleh lewat saat diajukan (tanggal hari ini di zona organisasi) |
| Bersamaan | Kuota dihitung setelah requisition di-lock `FOR UPDATE`, sehingga dua pembuatan lowongan paralel hanya satu yang berhasil; lamaran di-lock `FOR UPDATE`; versi basi 409; interviewer bentrok ditolak DB (termasuk serentak); lamaran tidak aktif membatalkan interview terjadwal; pemberitahuan keputusan sekali |
| Tes | `TestInterviewsScorecardDecision`, `TestRequisitionApprovalAndJob`, `TestApprovalGuardsOnRealDB`; E2E `keputusan rekrutmen`; `TestTrackRequisitionCreateRead`, `TestJobInheritsRequisitionTrack`, `TestJobQuotaWithinHeadcount`, `TestJobQuotaRace`, `TestTrackScopeNotFound`, `TestTrackQuotaAudit`; E2E `track.spec.ts` (permintaan MT, disetujui, lowongan, batas kuota) |

## Workflow

| Aspek | Isi |
|---|---|
| Layar | `/staf/workflow`, `/staf/workflow/:id` (editor formulir dapat dioperasikan keyboard, inspector syarat, validasi, simulasi, versi, rollback) |
| API | `GET /workflows`, `GET /workflows/{id}`, `PUT /workflows/{id}/draft`, `POST /workflows/{id}/validate|simulate|publish|rollback`, `GET /workflows/{id}/versions` |
| Data | `workflows` (draft + revision), `workflow_versions` (immutable), `workflow_stages`, `workflow_activations` |
| Akses | Baca `workflows.read`, ubah `workflows.manage`, terbit/rollback `workflows.publish` |
| Validasi | Satu start, terminal eksplisit (tepat satu `rejected`), tanpa siklus, target ada, semua terjangkau, cabang bersyarat wajib default, terminal tanpa edge keluar, approval wajib peran + kuorum; rule allowlist field/operator, kedalaman <= 4, node <= 32, evaluasi <= 100 per event; null = tidak memenuhi |
| Bersamaan | Publish mengunci workflow (nomor versi unik); draft basi 409; lamaran berjalan tetap di versinya; event ber-execution ID unik |
| Tes | `definition_test.go`, `TestWorkflowPublishVersioningAndPin`, `TestWorkflowEventBranchesOnRealDB` |

## Assessment (ujian)

| Aspek | Isi |
|---|---|
| Layar | Kandidat: `/akun/ujian`, `/ujian/:attemptId` (soal, opsi berlabel, status simpan, sisa waktu, navigator dijawab/ragu, drawer di ponsel, bukti penerimaan). Staf: `/staf/bank-soal`, `/staf/ujian`, `/staf/ujian/:id` (monitoring, undang, rilis), `/staf/penilaian/:attemptId` |
| API | Kandidat: `GET /me/exam-invitations`, `POST /exam-invitations/{id}/start`, `GET /exam-attempts/{id}`, `PUT /exam-attempts/{id}/answers`, `POST /exam-attempts/{id}/submit|heartbeat`. Staf: `/exam-banks...`, `/questions/{id}/versions`, `/exam-packages/{key}/versions`, `/exam-schedules...`, `/exam-attempts/{id}/review|essay-grades` |
| Data | lihat ERD bagian ujian |
| Akses | Peserta hanya attempt miliknya; examiner tanpa identitas kandidat; rilis butuh `results.release` per scope lamaran |
| Validasi | Batch <= 50 item, body <= 256 KiB, opsi harus milik item, pilihan tunggal <= 1, esai <= 10.000 karakter; impor CSV divalidasi per baris dengan pratinjau |
| Bersamaan | Lock attempt yang sama untuk save/submit/expiry; deadline dari jam DB setelah lock; receipt per opId; submit sekali; worker expiry vs submit serentak -> satu finalisasi |
| Tes | `TestExamProtocolOnRealDB`, `TestExamBranchesDeadlineAndExpiry`, `TestBankImportAndPackageVersioning`, `bank_test.go`; Vitest `autosave.test.ts`, `draftStore.test.ts`; E2E ujian (offline/reconnect/refresh/submit), ponsel; k6 + rekonsiliasi |

## Dashboard dan pelaporan

| Aspek | Isi |
|---|---|
| Layar | `/staf/laporan` (kohort tanggal di zona organisasi, funnel per tahap, waktu per tahap, definisi penyebut, ekspor) |
| API | `GET /reports/recruitment?from&to&job`, `POST|GET /reports/exports`, `POST /reports/exports/{id}/link`, `GET /reports/download` |
| Akses | Scope `reports.read`; ekspor funnel `reports.export`; ekspor data pribadi `pii.export` + limiter global (503 bila Redis tidak ada dan mode ketat). Scope diperiksa saat request, generate (hak akses terbaru pemohon), dan download |
| Validasi | Rentang <= 2 tahun; sel CSV dinetralkan dari formula injection; tautan 60 detik terikat pengguna; berkas dihapus setelah 24 jam |
| Tes | `TestReportsMatchFixtureAndExport` (fixture: kandidat dua lamaran + satu withdrawal), `TestSafeCell`; E2E unduh ekspor |

## Akses, komunikasi, pengaturan

| Aspek | Isi |
|---|---|
| Layar | `/staf/pengguna` (pengguna, peran berscope, matriks), `/staf/pengaturan` (organisasi, brand, kebijakan berversi), `/staf/komunikasi` (riwayat email, dead-letter, replay), `/staf/audit` |
| API | `/admin/users`, `/admin/users/{id}/assignments|status`, `/admin/assignments/{id}/revoke`, `/admin/roles`, `/admin/audit`, `/admin/settings[/{key}[/history]]`, `/admin/organization`, `/admin/units|locations/{id}/active`, `/admin/deliveries`, `/admin/outbox/{id}/replay` |
| Aturan | Cegah penghapusan/penonaktifan Super Admin aktif terakhir (lock baris); staf baru tanpa sandi default (undangan atur sandi); pencabutan berlaku request berikutnya; pengaturan dengan `expectedVersion`, waktu efektif, alasan, riwayat; master data dinonaktifkan tanpa menghapus histori |
| Komunikasi | Template berversi di kode dengan placeholder allowlist (teks polos), inbox kandidat, `email_deliveries` dengan status `pending/sent/failed/ambiguous`, dedup per event; email tidak pernah dikirim di dalam transaksi |
| Tes | `TestAdminAccessSettingsAudit`; E2E admin |
