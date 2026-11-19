-- name: GetOrganizationBySlug :one
SELECT * FROM organizations WHERE slug = $1;

-- name: GetOrganization :one
SELECT * FROM organizations WHERE id = $1;

-- name: CreateUser :one
INSERT INTO users (org_id, kind, email, display_name, password_hash, email_verified_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE org_id = $1 AND email = $2;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: MarkEmailVerified :exec
UPDATE users SET email_verified_at = coalesce(email_verified_at, now()), updated_at = now() WHERE id = $1;

-- name: UpdatePassword :exec
UPDATE users SET password_hash = $2, password_changed_at = now(), updated_at = now(), version = version + 1
WHERE id = $1;

-- name: CreateSession :one
INSERT INTO sessions (user_id, token_digest, idle_expires_at, absolute_expires_at, ip, user_agent)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetActiveSessionByDigest :one
SELECT s.id AS session_id, s.idle_expires_at, s.absolute_expires_at, s.last_seen_at,
       u.id AS user_id, u.org_id, u.kind, u.email, u.display_name, u.status, u.email_verified_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_digest = $1
  AND s.revoked_at IS NULL
  AND s.idle_expires_at > now()
  AND s.absolute_expires_at > now();

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = now(), idle_expires_at = least(@new_idle::timestamptz, absolute_expires_at)
WHERE id = @id AND revoked_at IS NULL;

-- name: RevokeSession :exec
UPDATE sessions SET revoked_at = now(), revoke_reason = $2 WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeUserSessions :execrows
UPDATE sessions SET revoked_at = now(), revoke_reason = $2 WHERE user_id = $1 AND revoked_at IS NULL;

-- name: CreateEmailToken :one
INSERT INTO email_tokens (user_id, purpose, token_digest, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: ConsumeEmailToken :one
UPDATE email_tokens SET used_at = now()
WHERE token_digest = $1 AND purpose = $2 AND used_at IS NULL AND expires_at > now()
RETURNING user_id;

-- name: InvalidateEmailTokens :exec
UPDATE email_tokens SET used_at = now() WHERE user_id = $1 AND purpose = $2 AND used_at IS NULL;

-- name: ListUserGrants :many
-- Hak akses aktif user: satu baris per (permission, scope). Dibaca tiap request, tanpa cache.
SELECT rp.permission_key, a.unit_id, a.job_id, r.key AS role_key
FROM scoped_role_assignments a
JOIN roles r ON r.id = a.role_id
JOIN role_permissions rp ON rp.role_id = r.id
WHERE a.user_id = $1 AND a.revoked_at IS NULL;
