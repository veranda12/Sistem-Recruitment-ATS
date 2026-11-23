-- name: ListStaffUsers :many
SELECT u.id, u.email, u.display_name, u.status, u.created_at,
       coalesce((SELECT max(s.last_seen_at) FROM sessions s WHERE s.user_id = u.id), u.created_at)::timestamptz AS last_activity
FROM users u WHERE u.org_id = $1 AND u.kind = 'staff' ORDER BY u.display_name;

-- name: ListAssignments :many
SELECT a.id, a.user_id, a.unit_id, a.job_id, a.created_at, r.key AS role_key, r.name AS role_name,
       un.name AS unit_name, j.title AS job_title
FROM scoped_role_assignments a
JOIN roles r ON r.id = a.role_id
LEFT JOIN units un ON un.id = a.unit_id
LEFT JOIN job_openings j ON j.id = a.job_id
WHERE a.org_id = $1 AND a.revoked_at IS NULL
ORDER BY a.created_at;

-- name: GetRoleByKey :one
SELECT * FROM roles WHERE org_id = $1 AND key = $2;

-- name: ListRolesWithPermissions :many
SELECT r.id, r.key, r.name, r.system, r.version, r.archived_at,
       coalesce(array_agg(rp.permission_key ORDER BY rp.permission_key) FILTER (WHERE rp.permission_key IS NOT NULL), '{}')::text[] AS permissions,
       (SELECT count(*) FROM scoped_role_assignments a WHERE a.role_id = r.id AND a.revoked_at IS NULL)::bigint AS active_assignments
FROM roles r LEFT JOIN role_permissions rp ON rp.role_id = r.id
WHERE r.org_id = $1 GROUP BY r.id ORDER BY r.system DESC, r.name;

-- name: InsertRole :one
INSERT INTO roles (org_id, key, name, system) VALUES ($1, $2, $3, false) RETURNING *;

-- name: LockRole :one
SELECT * FROM roles WHERE org_id = $1 AND key = $2 FOR UPDATE;

-- name: UpdateRole :one
UPDATE roles SET name = $2, version = version + 1, updated_at = now() WHERE id = $1 RETURNING *;

-- name: ArchiveRole :exec
UPDATE roles SET archived_at = now(), version = version + 1, updated_at = now() WHERE id = $1;

-- name: DeleteRolePermissions :exec
DELETE FROM role_permissions WHERE role_id = $1;

-- name: InsertRolePermission :exec
INSERT INTO role_permissions (role_id, permission_key) VALUES ($1, $2);

-- name: CountActiveRoleAssignments :one
SELECT count(*) FROM scoped_role_assignments WHERE role_id = $1 AND revoked_at IS NULL;

-- name: RoleKeyExists :one
SELECT EXISTS (SELECT 1 FROM roles WHERE org_id = $1 AND key = $2);

-- name: InsertAssignment :one
INSERT INTO scoped_role_assignments (org_id, user_id, role_id, unit_id, job_id, granted_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: LockAssignment :one
SELECT a.*, r.key AS role_key FROM scoped_role_assignments a JOIN roles r ON r.id = a.role_id
WHERE a.id = $1 FOR UPDATE OF a;

-- name: RevokeAssignment :exec
UPDATE scoped_role_assignments SET revoked_at = now(), revoked_by = $2 WHERE id = $1 AND revoked_at IS NULL;

-- name: LockActiveSuperAdmins :many
-- Lock baris penugasan super_admin aktif agar dua pencabutan bersamaan tidak sama-sama lolos.
SELECT a.id, a.user_id FROM scoped_role_assignments a
JOIN roles r ON r.id = a.role_id
JOIN users u ON u.id = a.user_id
WHERE a.org_id = $1 AND r.key = 'super_admin' AND a.revoked_at IS NULL AND u.status = 'active'
FOR UPDATE OF a;

-- name: SetUserStatus :exec
UPDATE users SET status = $2, updated_at = now(), version = version + 1 WHERE id = $1;

-- name: ListAudit :many
SELECT l.id, l.actor_user_id, l.actor_kind, l.action, l.target_type, l.target_id, l.outcome, l.reason, l.request_id, l.created_at,
       u.display_name AS actor_name
FROM audit_logs l LEFT JOIN users u ON u.id = l.actor_user_id
WHERE l.org_id = @org_id
  AND (@org_wide::bool OR l.scope_unit_id = ANY(@unit_ids::uuid[]))
  AND (sqlc.narg(action)::text IS NULL OR l.action LIKE sqlc.narg(action) || '%')
  AND (sqlc.narg(target_id)::text IS NULL OR l.target_id = sqlc.narg(target_id))
  AND (sqlc.narg(cursor_id)::bigint IS NULL OR l.id < sqlc.narg(cursor_id))
ORDER BY l.id DESC
LIMIT @page_size::int;

-- name: GetSetting :one
SELECT * FROM system_settings WHERE org_id = $1 AND key = $2;

-- name: ListSettings :many
SELECT * FROM system_settings WHERE org_id = $1 ORDER BY key;

-- name: UpsertSetting :one
INSERT INTO system_settings (org_id, key, value, version, effective_at, updated_by)
VALUES (@org_id, @key, @value, 1, @effective_at, @updated_by)
ON CONFLICT (org_id, key) DO UPDATE SET value = EXCLUDED.value, version = system_settings.version + 1,
       effective_at = EXCLUDED.effective_at, updated_by = EXCLUDED.updated_by, updated_at = now()
WHERE system_settings.version = @expected_version::int
RETURNING *;

-- name: InsertSettingHistory :exec
INSERT INTO system_setting_history (org_id, key, version, value, effective_at, changed_by, reason) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListSettingHistory :many
SELECT h.*, u.display_name AS changed_by_name FROM system_setting_history h LEFT JOIN users u ON u.id = h.changed_by
WHERE h.org_id = $1 AND h.key = $2 ORDER BY h.version DESC;

-- name: UpdateOrganization :one
UPDATE organizations SET name = $3, profile = $4, brand = $5, version = version + 1, updated_at = now()
WHERE id = $1 AND version = $2 RETURNING *;

-- name: SetUnitActive :execrows
UPDATE units SET active = $3 WHERE id = $1 AND org_id = $2;

-- name: SetLocationActive :execrows
UPDATE locations SET active = $3 WHERE id = $1 AND org_id = $2;

-- name: ListDeadOutbox :many
SELECT id, topic, aggregate_type, aggregate_id, attempts, last_error, created_at FROM outbox WHERE org_id = $1 AND status = 'dead' ORDER BY created_at DESC LIMIT 100;
