-- name: LatestMessageTemplate :one
SELECT * FROM message_templates WHERE org_id = $1 AND key = $2 AND channel = $3 ORDER BY version DESC LIMIT 1;

-- name: LatestMessageTemplates :many
SELECT DISTINCT ON (key, channel) * FROM message_templates WHERE org_id = $1 ORDER BY key, channel, version DESC;

-- name: ListMessageTemplateVersions :many
SELECT t.*, u.display_name AS author_name
FROM message_templates t LEFT JOIN users u ON u.id = t.created_by
WHERE t.org_id = $1 AND t.key = $2 AND t.channel = $3 ORDER BY t.version DESC LIMIT 50;

-- name: GetMessageTemplateVersion :one
SELECT * FROM message_templates WHERE org_id = $1 AND key = $2 AND channel = $3 AND version = $4;

-- name: InsertMessageTemplate :one
-- Versi berikutnya; unique (org, key, channel, version) menolak dua penyimpanan bersamaan dari versi yang sama.
INSERT INTO message_templates (org_id, key, channel, version, subject, body, is_default, note, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;
