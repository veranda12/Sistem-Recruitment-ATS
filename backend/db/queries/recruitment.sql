-- name: ListUnits :many
SELECT * FROM units WHERE org_id = $1 ORDER BY name;

-- name: ListLocations :many
SELECT * FROM locations WHERE org_id = $1 ORDER BY name;

-- name: ListPublicJobs :many
-- Keyset pagination stabil (published_at DESC, id DESC). Hanya status published.
SELECT j.id, j.slug, j.title, j.employment_type, j.published_at, j.closes_at,
       u.code AS unit_code, u.name AS unit_name, l.code AS location_code, l.name AS location_name, l.city
FROM job_openings j
JOIN units u ON u.id = j.unit_id
JOIN locations l ON l.id = j.location_id
WHERE j.org_id = @org_id AND j.status = 'published'
  AND (sqlc.narg(unit_code)::text IS NULL OR u.code = sqlc.narg(unit_code))
  AND (sqlc.narg(location_code)::text IS NULL OR l.code = sqlc.narg(location_code))
  AND (sqlc.narg(employment_type)::text IS NULL OR j.employment_type = sqlc.narg(employment_type))
  AND (sqlc.narg(q)::text IS NULL OR j.title ILIKE '%' || sqlc.narg(q) || '%')
  AND (sqlc.narg(cursor_published_at)::timestamptz IS NULL
       OR (j.published_at, j.id) < (sqlc.narg(cursor_published_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY j.published_at DESC, j.id DESC
LIMIT @page_size::int;

-- name: GetJobBySlug :one
SELECT j.*, u.code AS unit_code, u.name AS unit_name, l.code AS location_code, l.name AS location_name, l.city
FROM job_openings j
JOIN units u ON u.id = j.unit_id
JOIN locations l ON l.id = j.location_id
WHERE j.org_id = $1 AND j.slug = $2;

-- name: GetJob :one
SELECT j.*, u.code AS unit_code, u.name AS unit_name, l.code AS location_code, l.name AS location_name, l.city
FROM job_openings j
JOIN units u ON u.id = j.unit_id
JOIN locations l ON l.id = j.location_id
WHERE j.id = $1;

-- name: LockJobForApply :one
-- FOR SHARE: apply dan close berurutan. Close mengambil FOR UPDATE sehingga menunggu apply berjalan, dan sebaliknya.
SELECT id, org_id, unit_id, workflow_id, status, closes_at, now()::timestamptz AS db_now FROM job_openings WHERE id = $1 FOR SHARE;

-- name: ListStaffJobs :many
SELECT j.id, j.slug, j.title, j.employment_type, j.status, j.published_at, j.closes_at, j.unit_id, j.version,
       u.name AS unit_name, l.name AS location_name,
       (SELECT count(*) FROM job_applications a WHERE a.job_id = j.id)::bigint AS application_count
FROM job_openings j
JOIN units u ON u.id = j.unit_id
JOIN locations l ON l.id = j.location_id
WHERE j.org_id = @org_id
  AND (@org_wide::bool OR j.unit_id = ANY(@unit_ids::uuid[]) OR j.id = ANY(@job_ids::uuid[]))
ORDER BY j.created_at DESC, j.id DESC
LIMIT 200;

-- name: CreateJob :one
INSERT INTO job_openings (org_id, requisition_id, unit_id, location_id, workflow_id, slug, title, employment_type, description, requirements, closes_at, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: UpdateJobContent :one
UPDATE job_openings SET title = $3, employment_type = $4, description = $5, requirements = $6,
       location_id = $7, closes_at = $8, version = version + 1, updated_at = now()
WHERE id = $1 AND version = $2
RETURNING *;

-- name: SetJobStatus :one
UPDATE job_openings SET status = @status,
       published_at = CASE WHEN @status = 'published' THEN coalesce(published_at, now()) ELSE published_at END,
       closed_at = CASE WHEN @status = 'closed' THEN now() ELSE NULL END,
       version = version + 1, updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- name: LockJobForUpdate :one
SELECT * FROM job_openings WHERE id = $1 FOR UPDATE;

-- name: InsertApplication :one
INSERT INTO job_applications (org_id, job_id, candidate_user_id, unit_id, workflow_version_id, stage_key)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (job_id, candidate_user_id) DO NOTHING
RETURNING *;

-- name: InsertApplicationSnapshot :exec
INSERT INTO application_snapshots (application_id, profile) VALUES ($1, $2);

-- name: GetApplicationSnapshot :one
SELECT * FROM application_snapshots WHERE application_id = $1;

-- name: GetApplicationByJobCandidate :one
SELECT * FROM job_applications WHERE job_id = $1 AND candidate_user_id = $2;

-- name: ListCandidateApplications :many
SELECT a.id, a.status, a.stage_key, a.applied_at, a.stage_entered_at, a.version,
       j.title AS job_title, j.slug AS job_slug, j.status AS job_status,
       s.name AS stage_name, s.type AS stage_type
FROM job_applications a
JOIN job_openings j ON j.id = a.job_id
JOIN workflow_stages s ON s.workflow_version_id = a.workflow_version_id AND s.key = a.stage_key
WHERE a.candidate_user_id = $1
ORDER BY a.applied_at DESC, a.id DESC;

-- name: GetApplication :one
SELECT a.*, j.title AS job_title, j.slug AS job_slug, u.name AS unit_name,
       s.name AS stage_name, s.type AS stage_type,
       c.email AS candidate_email, c.display_name AS candidate_name,
       r.display_name AS recruiter_name
FROM job_applications a
JOIN job_openings j ON j.id = a.job_id
JOIN units u ON u.id = a.unit_id
JOIN users c ON c.id = a.candidate_user_id
JOIN workflow_stages s ON s.workflow_version_id = a.workflow_version_id AND s.key = a.stage_key
LEFT JOIN users r ON r.id = a.recruiter_user_id
WHERE a.id = $1;

-- name: LockApplication :one
SELECT * FROM job_applications WHERE id = $1 FOR UPDATE;

-- name: ListStaffApplications :many
-- Scope diterapkan di query: deny-by-default bila org_wide=false dan daftar unit/job kosong.
SELECT a.id, a.job_id, a.status, a.stage_key, a.applied_at, a.stage_entered_at, a.unit_id, a.version, a.shortlisted,
       j.title AS job_title, c.display_name AS candidate_name, s.name AS stage_name, s.type AS stage_type,
       a.recruiter_user_id, r.display_name AS recruiter_name
FROM job_applications a
JOIN job_openings j ON j.id = a.job_id
JOIN users c ON c.id = a.candidate_user_id
JOIN workflow_stages s ON s.workflow_version_id = a.workflow_version_id AND s.key = a.stage_key
LEFT JOIN users r ON r.id = a.recruiter_user_id
WHERE a.org_id = @org_id
  AND (@org_wide::bool OR a.unit_id = ANY(@unit_ids::uuid[]) OR a.job_id = ANY(@job_ids::uuid[]))
  AND (sqlc.narg(job_id)::uuid IS NULL OR a.job_id = sqlc.narg(job_id))
  AND (sqlc.narg(status)::text IS NULL OR a.status = sqlc.narg(status))
  AND (sqlc.narg(stage_key)::text IS NULL OR a.stage_key = sqlc.narg(stage_key))
  AND (sqlc.narg(shortlisted)::bool IS NULL OR a.shortlisted = sqlc.narg(shortlisted))
  AND (sqlc.narg(recruiter_user_id)::uuid IS NULL OR a.recruiter_user_id = sqlc.narg(recruiter_user_id))
  AND (sqlc.narg(cursor_applied_at)::timestamptz IS NULL
       OR (a.applied_at, a.id) < (sqlc.narg(cursor_applied_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY a.applied_at DESC, a.id DESC
LIMIT @page_size::int;

-- name: ListOtherApplicationsOfCandidate :many
SELECT a.id, a.status, a.stage_key, a.applied_at, a.unit_id, a.job_id, j.title AS job_title
FROM job_applications a
JOIN job_openings j ON j.id = a.job_id
WHERE a.candidate_user_id = $1 AND a.id <> $2
ORDER BY a.applied_at DESC;

-- name: UpdateApplicationState :one
UPDATE job_applications SET stage_key = $2, status = $3,
       stage_entered_at = CASE WHEN stage_key <> $2 THEN now() ELSE stage_entered_at END,
       version = version + 1, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetApplicationShortlist :one
UPDATE job_applications SET shortlisted = $2, version = version + 1, updated_at = now()
WHERE id = $1 AND version = $3
RETURNING *;

-- name: ListAssignableRecruiters :many
-- Staf aktif yang boleh memindahkan lamaran pada unit atau lowongan ini (peran berizin applications.transition).
SELECT DISTINCT u.id, u.display_name
FROM users u
JOIN scoped_role_assignments sra ON sra.user_id = u.id AND sra.org_id = u.org_id AND sra.revoked_at IS NULL
JOIN role_permissions rp ON rp.role_id = sra.role_id AND rp.permission_key = 'applications.transition'
WHERE u.org_id = @org_id AND u.kind = 'staff' AND u.status = 'active'
  AND ((sra.unit_id IS NULL AND sra.job_id IS NULL) OR sra.unit_id = @unit_id::uuid OR sra.job_id = @job_id::uuid)
ORDER BY u.display_name;

-- name: AssignRecruiter :one
UPDATE job_applications SET recruiter_user_id = $2, version = version + 1, updated_at = now()
WHERE id = $1 AND version = $3
RETURNING *;

-- name: InsertTransition :one
INSERT INTO transition_history (org_id, application_id, execution_id, workflow_version_id, from_stage, to_stage,
       from_status, to_status, trigger, actor_user_id, actor_kind, reason, rule_ref, input_summary, request_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
ON CONFLICT (execution_id) DO NOTHING
RETURNING id;

-- name: GetTransitionByExecution :one
SELECT * FROM transition_history WHERE execution_id = $1;

-- name: ListTransitions :many
SELECT t.*, u.display_name AS actor_name
FROM transition_history t
LEFT JOIN users u ON u.id = t.actor_user_id
WHERE t.application_id = $1
ORDER BY t.id;
