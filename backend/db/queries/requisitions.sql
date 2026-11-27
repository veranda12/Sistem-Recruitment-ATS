-- name: CreateRequisition :one
INSERT INTO requisitions (org_id, unit_id, location_id, title, headcount, employment_type, justification, requested_by, hiring_manager_id,
                          track, line, target_join_date, reason_category)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: ListRequisitions :many
-- Scope unit/organisasi dari requisitions.read, ditambah requisition milik sendiri (pemohon atau hiring manager).
-- unit_code opsional dicocokkan lewat join ke units (ID unit). Filter ini hanya mempersempit hasil di dalam scope.
SELECT r.*, u.name AS unit_name, l.name AS location_name, rq.display_name AS requested_by_name, hm.display_name AS hiring_manager_name
FROM requisitions r
JOIN units u ON u.id = r.unit_id
JOIN locations l ON l.id = r.location_id
JOIN users rq ON rq.id = r.requested_by
LEFT JOIN users hm ON hm.id = r.hiring_manager_id
WHERE r.org_id = @org_id
  AND (@org_wide::bool OR r.unit_id = ANY(@unit_ids::uuid[]) OR r.requested_by = @user_id OR r.hiring_manager_id = @user_id)
  AND (sqlc.narg(unit_code)::text IS NULL OR u.code = sqlc.narg(unit_code))
ORDER BY r.created_at DESC
LIMIT 200;

-- name: LockRequisition :one
SELECT * FROM requisitions WHERE id = $1 FOR UPDATE;

-- name: SetRequisitionStatus :one
UPDATE requisitions SET status = @status, decided_by = CASE WHEN @status IN ('approved', 'rejected') THEN sqlc.narg(decided_by) ELSE decided_by END,
       decided_at = CASE WHEN @status IN ('approved', 'rejected') THEN now() ELSE decided_at END,
       decision_note = @note, version = version + 1, updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;
