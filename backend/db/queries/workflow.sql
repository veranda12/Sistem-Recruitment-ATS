-- name: CreateWorkflow :one
INSERT INTO workflows (org_id, key, name, draft_definition, updated_by) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetWorkflow :one
SELECT * FROM workflows WHERE id = $1;

-- name: GetWorkflowByKey :one
SELECT * FROM workflows WHERE org_id = $1 AND key = $2;

-- name: ListWorkflows :many
SELECT w.*, v.version_no AS active_version_no
FROM workflows w LEFT JOIN workflow_versions v ON v.id = w.active_version_id
WHERE w.org_id = $1 ORDER BY w.name;

-- name: LockWorkflow :one
SELECT * FROM workflows WHERE id = $1 FOR UPDATE;

-- name: UpdateWorkflowDraft :one
UPDATE workflows SET draft_definition = $3, name = $4, draft_revision = draft_revision + 1, updated_at = now(), updated_by = $5
WHERE id = $1 AND draft_revision = $2
RETURNING *;

-- name: NextWorkflowVersionNo :one
SELECT coalesce(max(version_no), 0)::int + 1 FROM workflow_versions WHERE workflow_id = $1;

-- name: InsertWorkflowVersion :one
INSERT INTO workflow_versions (org_id, workflow_id, version_no, definition, checksum, notes, published_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: InsertWorkflowStage :exec
INSERT INTO workflow_stages (workflow_version_id, key, name, type, terminal_outcome, position)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: SetActiveWorkflowVersion :exec
UPDATE workflows SET active_version_id = $2, updated_at = now() WHERE id = $1;

-- name: InsertWorkflowActivation :exec
INSERT INTO workflow_activations (workflow_id, version_id, action, actor_user_id, reason) VALUES ($1, $2, $3, $4, $5);

-- name: ListWorkflowVersions :many
SELECT v.id, v.version_no, v.published_at, v.notes, v.published_by, u.display_name AS published_by_name,
       (w.active_version_id = v.id)::bool AS active
FROM workflow_versions v
JOIN workflows w ON w.id = v.workflow_id
LEFT JOIN users u ON u.id = v.published_by
WHERE v.workflow_id = $1 ORDER BY v.version_no DESC;

-- name: GetWorkflowVersion :one
SELECT * FROM workflow_versions WHERE id = $1;

-- name: ListWorkflowActivations :many
SELECT a.*, v.version_no FROM workflow_activations a JOIN workflow_versions v ON v.id = a.version_id
WHERE a.workflow_id = $1 ORDER BY a.id DESC;

-- name: CountApprovals :one
SELECT count(*) FILTER (WHERE decision = 'approve')::int AS approves,
       count(*) FILTER (WHERE decision = 'reject')::int AS rejects
FROM approvals
WHERE application_id = $1 AND stage_key = $2 AND stage_entry_seq = $3 AND invalidated_at IS NULL;

-- name: InsertApproval :one
INSERT INTO approvals (org_id, application_id, stage_key, stage_entry_seq, approver_user_id, decision, comment)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT DO NOTHING
RETURNING id;

-- name: InvalidateApprovals :exec
UPDATE approvals SET invalidated_at = now() WHERE application_id = $1 AND invalidated_at IS NULL;

-- name: ListApprovals :many
SELECT a.*, u.display_name AS approver_name FROM approvals a JOIN users u ON u.id = a.approver_user_id
WHERE a.application_id = $1 ORDER BY a.created_at;

-- name: LastStageEntrySeq :one
SELECT coalesce(max(id), 0)::bigint FROM transition_history WHERE application_id = $1 AND to_stage = $2;

-- name: ActiveApplicationsByVersionStage :many
-- Lamaran aktif per versi dan tahap untuk satu workflow (bahan pratinjau pemindahan versi).
SELECT a.workflow_version_id, v.version_no, a.stage_key, count(*)::bigint AS n
FROM job_applications a JOIN workflow_versions v ON v.id = a.workflow_version_id
WHERE v.workflow_id = $1 AND a.status = 'active'
GROUP BY a.workflow_version_id, v.version_no, a.stage_key
ORDER BY v.version_no, a.stage_key;

-- name: LockActiveApplicationsOnVersion :many
SELECT id, org_id, stage_key, status FROM job_applications
WHERE workflow_version_id = $1 AND status = 'active' ORDER BY id FOR UPDATE;

-- name: MigrateApplicationVersion :exec
UPDATE job_applications SET workflow_version_id = $2, stage_key = $3,
       stage_entered_at = CASE WHEN stage_key <> $3 THEN now() ELSE stage_entered_at END,
       version = version + 1, updated_at = now()
WHERE id = $1;
