-- name: CreateCVParseRun :one
-- Idempoten per (kandidat, dokumen, versi konfigurasi parser): permintaan ulang mengembalikan run yang sama.
INSERT INTO cv_parse_runs (org_id, candidate_user_id, document_id, parser_config_version, status, stage)
VALUES ($1, $2, $3, $4, $5, $5)
ON CONFLICT (candidate_user_id, document_id, parser_config_version) DO UPDATE SET updated_at = cv_parse_runs.updated_at
RETURNING *;

-- name: GetCVParseRun :one
SELECT * FROM cv_parse_runs WHERE id = $1;

-- name: GetCVParseRunForUpdate :one
SELECT * FROM cv_parse_runs WHERE id = $1 FOR UPDATE;

-- name: ListCVParseRunsForCandidate :many
SELECT r.*, d.file_name AS document_name FROM cv_parse_runs r JOIN documents d ON d.id = r.document_id
WHERE r.candidate_user_id = $1 ORDER BY r.created_at DESC LIMIT 10;

-- name: ReleaseWaitingCVRuns :execrows
-- Dipanggil setelah pemindaian dokumen bersih.
-- queued_at di-reset: waktu antre diukur sejak siap diproses, bukan sejak menunggu pemindaian.
UPDATE cv_parse_runs SET status = 'queued', stage = 'queued', next_attempt_at = now(), queued_at = now(), updated_at = now()
WHERE document_id = $1 AND status = 'waiting_scan';

-- name: FailWaitingCVRuns :execrows
UPDATE cv_parse_runs SET status = 'failed', stage = 'done', failure_reason = $2, failure_transient = false, finished_at = now(), updated_at = now()
WHERE document_id = $1 AND status = 'waiting_scan';

-- name: CancelCVRunsForDocument :many
-- Penghapusan dokumen: run berjalan dibatalkan; fence dinaikkan agar worker yang sedang memproses gagal menulis.
UPDATE cv_parse_runs SET status = 'cancelled', stage = 'done', fence = fence + 1, lease_owner = NULL, lease_until = NULL,
       finished_at = coalesce(finished_at, now()), updated_at = now()
WHERE document_id = $1 AND status NOT IN ('applied', 'dismissed', 'cancelled')
RETURNING id;

-- name: DeleteCVArtifactsForRun :exec
DELETE FROM cv_parse_artifacts WHERE run_id = $1;

-- name: DeleteCVSuggestionsForRun :exec
DELETE FROM cv_field_suggestions WHERE run_id = $1;

-- name: DeleteCVEvidenceForRun :exec
DELETE FROM cv_evidence_refs WHERE run_id = $1;

-- name: DueCVParseRuns :many
-- Run yang siap dikirim ke antrean: baru/antri dan belum dikirim baru-baru ini, atau lease kedaluwarsa.
SELECT id, fence FROM cv_parse_runs
WHERE (status = 'queued' AND next_attempt_at <= now() AND (dispatched_at IS NULL OR dispatched_at < now() - interval '2 minutes'))
   OR (status = 'processing' AND lease_until < now())
ORDER BY next_attempt_at
LIMIT $1;

-- name: MarkCVDispatched :exec
UPDATE cv_parse_runs SET dispatched_at = now() WHERE id = $1;

-- name: ClaimCVParseRun :one
-- Klaim dengan lease; fence naik. Run yang lease-nya habis boleh diambil alih.
UPDATE cv_parse_runs SET status = 'processing', stage = 'starting', fence = fence + 1, lease_owner = sqlc.arg(owner),
       lease_until = now() + make_interval(secs => sqlc.arg(lease_seconds)::int), heartbeat_at = now(), attempt = attempt + 1,
       started_at = coalesce(started_at, now()), claimed_at = now(),
       queue_ms = coalesce(queue_ms, (extract(epoch FROM now() - queued_at) * 1000)::int), updated_at = now()
WHERE cv_parse_runs.id = sqlc.arg(id)
  AND attempt < max_attempts
  AND ((status = 'queued' AND next_attempt_at <= now()) OR (status = 'processing' AND lease_until < now()))
  AND EXISTS (SELECT 1 FROM documents d WHERE d.id = cv_parse_runs.document_id AND d.deleted_at IS NULL AND d.scan_status = 'clean')
RETURNING *;

-- name: HeartbeatCVParseRun :execrows
UPDATE cv_parse_runs SET lease_until = now() + make_interval(secs => sqlc.arg(lease_seconds)::int), heartbeat_at = now(),
       stage = sqlc.arg(stage), pages_done = sqlc.arg(pages_done), pages_total = sqlc.narg(pages_total), updated_at = now()
WHERE id = sqlc.arg(id) AND fence = sqlc.arg(fence) AND status = 'processing';

-- name: FinishCVParseRun :execrows
-- Hanya pemegang fence terkini, dan hanya bila dokumen belum dihapus (job terlambat tidak menghidupkan file terhapus).
UPDATE cv_parse_runs SET status = sqlc.arg(status), stage = 'done', manifest = sqlc.arg(manifest), pages_total = sqlc.narg(pages_total),
       pages_done = coalesce(sqlc.narg(pages_total), pages_done), failure_reason = sqlc.narg(failure_reason), failure_transient = false,
       lease_owner = NULL, lease_until = NULL, finished_at = now(),
       process_ms = (extract(epoch FROM now() - claimed_at) * 1000)::int, updated_at = now()
WHERE cv_parse_runs.id = sqlc.arg(id) AND fence = sqlc.arg(fence) AND status = 'processing'
  AND EXISTS (SELECT 1 FROM documents d WHERE d.id = cv_parse_runs.document_id AND d.deleted_at IS NULL);

-- name: RetryCVParseRun :execrows
-- Gagal sementara: kembali ke antrean dengan backoff, atau dead-letter bila percobaan habis.
UPDATE cv_parse_runs SET
    status = CASE WHEN attempt >= max_attempts THEN 'failed' ELSE 'queued' END,
    stage = CASE WHEN attempt >= max_attempts THEN 'done' ELSE 'queued' END,
    failure_reason = sqlc.arg(failure_reason), failure_transient = true,
    dead_lettered_at = CASE WHEN attempt >= max_attempts THEN now() END,
    finished_at = CASE WHEN attempt >= max_attempts THEN now() END,
    next_attempt_at = now() + make_interval(secs => sqlc.arg(backoff_seconds)::int),
    dispatched_at = NULL, lease_owner = NULL, lease_until = NULL, updated_at = now()
WHERE id = sqlc.arg(id) AND fence = sqlc.arg(fence) AND status = 'processing';

-- name: ExpireExhaustedCVRuns :many
-- Lease habis dan percobaan habis (worker mati di tengah jalan berulang kali): dead-letter.
UPDATE cv_parse_runs SET status = 'failed', stage = 'done', failure_reason = 'attempts_exhausted', failure_transient = true,
       dead_lettered_at = now(), finished_at = now(), lease_owner = NULL, lease_until = NULL, fence = fence + 1, updated_at = now()
WHERE status = 'processing' AND lease_until < now() AND attempt >= max_attempts
RETURNING id;

-- name: ReplayCVParseRun :execrows
-- Replay terotorisasi atas run dead-letter.
UPDATE cv_parse_runs SET status = 'queued', stage = 'queued', attempt = 0, failure_reason = NULL, failure_transient = NULL,
       dead_lettered_at = NULL, finished_at = NULL, next_attempt_at = now(), dispatched_at = NULL, updated_at = now()
WHERE id = $1 AND status = 'failed';

-- name: CVQueueStats :one
SELECT count(*) FILTER (WHERE status IN ('queued', 'waiting_scan'))::int AS queued,
       count(*) FILTER (WHERE status = 'processing')::int AS processing,
       count(*) FILTER (WHERE dead_lettered_at IS NOT NULL AND status = 'failed')::int AS dead,
       coalesce(extract(epoch FROM now() - min(queued_at) FILTER (WHERE status = 'queued')), 0)::float8 AS oldest_queued_seconds
FROM cv_parse_runs;

-- name: InsertCVParsePage :exec
INSERT INTO cv_parse_pages (run_id, page_no, method, reason, status, failure_reason, width, height, scale, image_name)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: ListCVParsePages :many
SELECT * FROM cv_parse_pages WHERE run_id = $1 ORDER BY page_no;

-- name: GetCVParsePage :one
SELECT * FROM cv_parse_pages WHERE run_id = $1 AND page_no = $2;

-- name: InsertCVArtifact :exec
INSERT INTO cv_parse_artifacts (run_id, ir) VALUES ($1, $2);

-- name: InsertCVEvidence :one
INSERT INTO cv_evidence_refs (run_id, line_id, page_no, bbox, snippet, source, locator)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id;

-- name: ListCVEvidence :many
SELECT * FROM cv_evidence_refs WHERE run_id = $1;

-- name: InsertCVSuggestion :one
INSERT INTO cv_field_suggestions (run_id, path, section, entry_key, field, raw_value, normalized, read_doubts, assoc_doubts, sort)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id;

-- name: LinkCVSuggestionEvidence :exec
INSERT INTO cv_suggestion_evidence (suggestion_id, evidence_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: ListCVSuggestions :many
SELECT s.*, coalesce(array_agg(se.evidence_id) FILTER (WHERE se.evidence_id IS NOT NULL), '{}')::uuid[] AS evidence_ids
FROM cv_field_suggestions s LEFT JOIN cv_suggestion_evidence se ON se.suggestion_id = s.id
WHERE s.run_id = $1 GROUP BY s.id ORDER BY s.sort;

-- name: SetCVSuggestionDecision :execrows
UPDATE cv_field_suggestions SET status = $3, edited_value = $4, updated_at = now() WHERE id = $1 AND run_id = $2;

-- name: BumpCVReviewVersion :execrows
UPDATE cv_parse_runs SET review_version = review_version + 1, updated_at = now()
WHERE id = $1 AND review_version = $2 AND status IN ('ready_for_review', 'partial');

-- name: MarkCVRunApplied :execrows
UPDATE cv_parse_runs SET status = 'applied', applied_at = now(), updated_at = now()
WHERE id = $1 AND review_version = $2 AND status IN ('ready_for_review', 'partial');

-- name: MarkCVRunDismissed :execrows
UPDATE cv_parse_runs SET status = 'dismissed', updated_at = now() WHERE id = $1 AND status IN ('ready_for_review', 'partial', 'failed');

-- name: InsertCVReviewEvent :exec
INSERT INTO cv_review_events (run_id, suggestion_id, actor_user_id, action, detail) VALUES ($1, $2, $3, $4, $5);

-- name: LastCVApplyEvent :one
SELECT detail FROM cv_review_events WHERE run_id = $1 AND action = 'apply' ORDER BY id DESC LIMIT 1;

-- ---- Profil: field tambahan ----

-- name: TouchCandidateProfile :one
-- Menaikkan versi profil bila versi masih sama (optimistic concurrency untuk apply CV).
UPDATE candidate_profiles SET full_name = $3, phone = $4, city = $5, summary = $6, contact_email = $7,
       version = version + 1, updated_at = now()
WHERE user_id = $1 AND version = $2
RETURNING *;

-- name: SetContactEmail :exec
UPDATE candidate_profiles SET contact_email = $2 WHERE user_id = $1;

-- name: ListSkills :many
SELECT * FROM candidate_skills WHERE user_id = $1 ORDER BY created_at, id;

-- name: InsertSkill :exec
INSERT INTO candidate_skills (user_id, name) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: DeleteSkillsForUser :exec
DELETE FROM candidate_skills WHERE user_id = $1;

-- name: ListLanguages :many
SELECT * FROM candidate_languages WHERE user_id = $1 ORDER BY created_at, id;

-- name: InsertLanguage :exec
INSERT INTO candidate_languages (user_id, name, level) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING;

-- name: DeleteLanguagesForUser :exec
DELETE FROM candidate_languages WHERE user_id = $1;

-- name: ListLinks :many
SELECT * FROM candidate_links WHERE user_id = $1 ORDER BY created_at, id;

-- name: InsertLink :exec
INSERT INTO candidate_links (user_id, label, url) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING;

-- name: DeleteLinksForUser :exec
DELETE FROM candidate_links WHERE user_id = $1;

-- name: ListProjects :many
SELECT * FROM candidate_projects WHERE user_id = $1 ORDER BY created_at, id;

-- name: InsertProject :exec
INSERT INTO candidate_projects (user_id, name, description) VALUES ($1, $2, $3);

-- name: DeleteProjectsForUser :exec
DELETE FROM candidate_projects WHERE user_id = $1;

-- name: InsertExperienceFull :one
INSERT INTO experiences (user_id, company, title, location, start_date, start_precision, end_date, end_precision, is_current, description)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id;

-- name: DeleteExperience :execrows
DELETE FROM experiences WHERE id = $1 AND user_id = $2;

-- name: DeleteEducation :execrows
DELETE FROM education WHERE id = $1 AND user_id = $2;

-- name: InsertCertificationFull :exec
INSERT INTO certifications (user_id, name, issuer, issued_on, issued_precision) VALUES ($1, $2, $3, $4, $5);
