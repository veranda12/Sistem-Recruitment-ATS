-- Laporan memakai scope yang sama dengan daftar lamaran (deny-by-default).
-- Kohort = lamaran dengan applied_at dalam [from, to) yang dihitung dari tanggal zona organisasi.

-- name: FunnelByStage :many
-- Jumlah lamaran unik yang PERNAH mencapai tiap tahap (dari histori append-only), per lowongan.
SELECT j.id AS job_id, j.title AS job_title, th.to_stage AS stage_key, count(DISTINCT th.application_id)::bigint AS reached
FROM job_applications a
JOIN job_openings j ON j.id = a.job_id
JOIN transition_history th ON th.application_id = a.id
WHERE a.org_id = @org_id
  AND (@org_wide::bool OR a.unit_id = ANY(@unit_ids::uuid[]) OR a.job_id = ANY(@job_ids::uuid[]))
  AND a.applied_at >= @from_at::timestamptz AND a.applied_at < @to_at::timestamptz
  AND (sqlc.narg(job_id)::uuid IS NULL OR a.job_id = sqlc.narg(job_id))
GROUP BY j.id, j.title, th.to_stage
ORDER BY j.title, th.to_stage;

-- name: StatusByJob :many
SELECT j.id AS job_id, j.title AS job_title,
       count(*)::bigint AS applications,
       count(DISTINCT a.candidate_user_id)::bigint AS candidates,
       count(*) FILTER (WHERE a.status = 'active')::bigint AS active,
       count(*) FILTER (WHERE a.status = 'withdrawn')::bigint AS withdrawn,
       count(*) FILTER (WHERE a.status = 'rejected')::bigint AS rejected,
       count(*) FILTER (WHERE a.status = 'hired')::bigint AS hired
FROM job_applications a JOIN job_openings j ON j.id = a.job_id
WHERE a.org_id = @org_id
  AND (@org_wide::bool OR a.unit_id = ANY(@unit_ids::uuid[]) OR a.job_id = ANY(@job_ids::uuid[]))
  AND a.applied_at >= @from_at::timestamptz AND a.applied_at < @to_at::timestamptz
  AND (sqlc.narg(job_id)::uuid IS NULL OR a.job_id = sqlc.narg(job_id))
GROUP BY j.id, j.title
ORDER BY j.title;

-- name: StageDurations :many
-- Rata-rata dan median jam di tiap tahap untuk perpindahan yang sudah selesai (tahap berikutnya tercatat).
WITH t AS (
    SELECT th.application_id, th.to_stage, th.created_at,
           lead(th.created_at) OVER (PARTITION BY th.application_id ORDER BY th.id) AS left_at
    FROM transition_history th
    JOIN job_applications a ON a.id = th.application_id
    WHERE a.org_id = @org_id
      AND (@org_wide::bool OR a.unit_id = ANY(@unit_ids::uuid[]) OR a.job_id = ANY(@job_ids::uuid[]))
      AND a.applied_at >= @from_at::timestamptz AND a.applied_at < @to_at::timestamptz
      AND (sqlc.narg(job_id)::uuid IS NULL OR a.job_id = sqlc.narg(job_id))
)
SELECT to_stage AS stage_key, count(*)::bigint AS samples,
       (avg(extract(epoch FROM left_at - created_at)) / 3600)::float8 AS avg_hours,
       (percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM left_at - created_at)) / 3600)::float8 AS median_hours
FROM t WHERE left_at IS NOT NULL AND left_at > created_at
GROUP BY to_stage ORDER BY to_stage;

-- name: ApplicationsForExport :many
SELECT a.id, j.title AS job_title, u.name AS unit_name, c.display_name AS candidate_name, c.email AS candidate_email,
       a.stage_key, a.status, a.applied_at, a.stage_entered_at
FROM job_applications a
JOIN job_openings j ON j.id = a.job_id
JOIN units u ON u.id = a.unit_id
JOIN users c ON c.id = a.candidate_user_id
WHERE a.org_id = @org_id
  AND (@org_wide::bool OR a.unit_id = ANY(@unit_ids::uuid[]) OR a.job_id = ANY(@job_ids::uuid[]))
  AND a.applied_at >= @from_at::timestamptz AND a.applied_at < @to_at::timestamptz
ORDER BY a.applied_at, a.id
LIMIT 100000;

-- name: CreateExport :one
INSERT INTO exports (org_id, requested_by, kind, params) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: GetExport :one
SELECT * FROM exports WHERE id = $1;

-- name: ListMyExports :many
SELECT * FROM exports WHERE requested_by = $1 ORDER BY created_at DESC LIMIT 20;

-- name: MarkExportRunning :execrows
UPDATE exports SET status = 'running' WHERE id = $1 AND status = 'queued';

-- name: FinishExport :exec
UPDATE exports SET status = $2, file_key = $3, row_count = $4, error = $5, finished_at = now(), expires_at = $6 WHERE id = $1;

-- name: ExpiredExports :many
SELECT id, file_key FROM exports WHERE status = 'ready' AND expires_at < now() LIMIT 100;

-- name: MarkExportExpired :exec
UPDATE exports SET status = 'expired', file_key = NULL WHERE id = $1;

-- name: CandidatesInCohort :one
-- Kandidat unik lintas lowongan (tidak bisa dijumlahkan dari baris per lowongan).
SELECT count(DISTINCT a.candidate_user_id)::bigint FROM job_applications a
WHERE a.org_id = @org_id
  AND (@org_wide::bool OR a.unit_id = ANY(@unit_ids::uuid[]) OR a.job_id = ANY(@job_ids::uuid[]))
  AND a.applied_at >= @from_at::timestamptz AND a.applied_at < @to_at::timestamptz
  AND (sqlc.narg(job_id)::uuid IS NULL OR a.job_id = sqlc.narg(job_id));
