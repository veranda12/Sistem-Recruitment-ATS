-- name: InsertDocument :one
INSERT INTO documents (org_id, owner_user_id, kind, file_name, content_type, size_bytes, object_key, sha256)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: CountActiveDocuments :one
SELECT count(*)::int FROM documents WHERE owner_user_id = $1 AND deleted_at IS NULL;

-- name: ListOwnerDocuments :many
SELECT id, kind, file_name, content_type, size_bytes, scan_status, scanned_at, created_at
FROM documents WHERE owner_user_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC;

-- name: GetDocument :one
SELECT * FROM documents WHERE id = $1;

-- name: SoftDeleteDocument :execrows
UPDATE documents SET deleted_at = now() WHERE id = $1 AND owner_user_id = $2 AND deleted_at IS NULL;

-- name: SetDocumentScan :exec
UPDATE documents SET scan_status = $2, scanned_at = now(), object_key = $3 WHERE id = $1;

-- name: ApplicationScopesOfCandidate :many
-- Unit dan lowongan dari lamaran kandidat, untuk memeriksa scope staf sebelum membuka dokumen.
SELECT unit_id, job_id FROM job_applications WHERE candidate_user_id = $1;
