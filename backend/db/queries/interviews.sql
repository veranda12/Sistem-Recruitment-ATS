-- name: InsertInterview :one
INSERT INTO interviews (org_id, application_id, starts_at, ends_at, timezone, mode, location, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: InsertPanelist :exec
INSERT INTO interview_panelists (interview_id, interviewer_id, period) VALUES ($1, $2, tstzrange(@starts_at::timestamptz, @ends_at::timestamptz, '[)'));

-- name: LockInterview :one
SELECT * FROM interviews WHERE id = $1 FOR UPDATE;

-- name: RescheduleInterview :one
UPDATE interviews SET starts_at = $3, ends_at = $4, timezone = $5, version = version + 1, updated_at = now()
WHERE id = $1 AND version = $2 AND status = 'scheduled'
RETURNING *;

-- name: UpdatePanelPeriod :exec
UPDATE interview_panelists SET period = tstzrange(@starts_at::timestamptz, @ends_at::timestamptz, '[)') WHERE interview_id = @interview_id;

-- name: SetInterviewStatus :one
UPDATE interviews SET status = $3, version = version + 1, updated_at = now()
WHERE id = $1 AND version = $2 AND status = 'scheduled'
RETURNING *;

-- name: DeactivatePanel :exec
UPDATE interview_panelists SET active = false WHERE interview_id = $1;

-- name: InsertInterviewChange :exec
INSERT INTO interview_changes (interview_id, action, old_starts_at, new_starts_at, timezone, reason, actor_user_id)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListInterviewsForApplication :many
SELECT i.*, coalesce((SELECT array_agg(u.display_name ORDER BY u.display_name) FROM interview_panelists p JOIN users u ON u.id = p.interviewer_id WHERE p.interview_id = i.id), '{}')::text[] AS interviewer_names,
       coalesce((SELECT array_agg(p.interviewer_id) FROM interview_panelists p WHERE p.interview_id = i.id), '{}')::uuid[] AS interviewer_ids
FROM interviews i WHERE i.application_id = $1 ORDER BY i.starts_at;

-- name: ListInterviewsForInterviewer :many
-- Kalender: interview aktif yang melibatkan user dalam rentang waktu.
SELECT i.id, i.application_id, i.starts_at, i.ends_at, i.timezone, i.mode, i.location, i.status,
       c.display_name AS candidate_name, j.title AS job_title
FROM interviews i
JOIN interview_panelists p ON p.interview_id = i.id
JOIN job_applications a ON a.id = i.application_id
JOIN users c ON c.id = a.candidate_user_id
JOIN job_openings j ON j.id = a.job_id
WHERE p.interviewer_id = @user_id AND i.starts_at < @to_at::timestamptz AND i.ends_at > @from_at::timestamptz
ORDER BY i.starts_at;

-- name: ListInterviewChanges :many
SELECT ch.*, u.display_name AS actor_name FROM interview_changes ch JOIN users u ON u.id = ch.actor_user_id
WHERE ch.interview_id = $1 ORDER BY ch.id;

-- name: IsPanelist :one
SELECT EXISTS (SELECT 1 FROM interview_panelists WHERE interview_id = $1 AND interviewer_id = $2)::bool;

-- name: UpsertEvaluation :one
INSERT INTO evaluations (interview_id, evaluator_id, rating, recommendation, notes) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (interview_id, evaluator_id) DO UPDATE SET rating = EXCLUDED.rating, recommendation = EXCLUDED.recommendation, notes = EXCLUDED.notes, created_at = now()
RETURNING *;

-- name: ListEvaluationsForApplication :many
SELECT e.*, u.display_name AS evaluator_name, i.starts_at AS interview_at
FROM evaluations e JOIN interviews i ON i.id = e.interview_id JOIN users u ON u.id = e.evaluator_id
WHERE i.application_id = $1 ORDER BY e.created_at;

-- name: GetStaffUser :one
SELECT id, org_id, display_name, status FROM users WHERE id = $1 AND kind = 'staff';

-- name: MarkDecisionNotified :execrows
UPDATE job_applications SET decision_notified_at = now(), decision_notified_by = $2
WHERE id = $1 AND decision_notified_at IS NULL AND status IN ('hired', 'rejected');

-- name: ListActiveStaff :many
SELECT id, display_name FROM users WHERE org_id = $1 AND kind = 'staff' AND status = 'active' ORDER BY display_name;
