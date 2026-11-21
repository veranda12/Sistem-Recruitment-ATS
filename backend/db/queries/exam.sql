-- name: CreateQuestionBank :one
INSERT INTO question_banks (org_id, name, category, created_by) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: CreateQuestion :one
INSERT INTO questions (org_id, bank_id) VALUES ($1, $2) RETURNING *;

-- name: InsertQuestionVersion :one
INSERT INTO question_versions (question_id, version_no, kind, stem, options, answer_key, weight, rubric, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: CreateExamPackage :one
INSERT INTO exam_packages (org_id, key, name) VALUES ($1, $2, $3) RETURNING *;

-- name: GetExamPackageByKey :one
SELECT * FROM exam_packages WHERE org_id = $1 AND key = $2;

-- name: InsertPackageVersion :one
INSERT INTO exam_package_versions (org_id, package_id, version_no, question_version_ids, duration_minutes, pass_score, shuffle_items, shuffle_options, published_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: LatestPackageVersion :one
SELECT v.* FROM exam_package_versions v WHERE v.package_id = $1 ORDER BY v.version_no DESC LIMIT 1;

-- name: GetPackageVersion :one
SELECT v.*, p.key AS package_key, p.name AS package_name
FROM exam_package_versions v JOIN exam_packages p ON p.id = v.package_id WHERE v.id = $1;

-- name: ListQuestionVersions :many
SELECT * FROM question_versions WHERE id = ANY(@ids::uuid[]);

-- name: CreateSchedule :one
INSERT INTO exam_schedules (org_id, package_version_id, name, opens_at, closes_at, max_attempts, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetSchedule :one
SELECT s.*, v.duration_minutes, v.pass_score, p.key AS package_key, p.name AS package_name
FROM exam_schedules s
JOIN exam_package_versions v ON v.id = s.package_version_id
JOIN exam_packages p ON p.id = v.package_id
WHERE s.id = $1;

-- name: ListSchedules :many
SELECT s.id, s.name, s.opens_at, s.closes_at, p.key AS package_key, p.name AS package_name,
       (SELECT count(*) FROM exam_participants ep WHERE ep.schedule_id = s.id)::bigint AS participant_count
FROM exam_schedules s
JOIN exam_package_versions v ON v.id = s.package_version_id
JOIN exam_packages p ON p.id = v.package_id
WHERE s.org_id = $1
ORDER BY s.opens_at DESC
LIMIT 100;

-- name: InsertParticipant :one
INSERT INTO exam_participants (org_id, schedule_id, application_id, candidate_user_id, extra_minutes, invited_by)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (schedule_id, application_id) DO NOTHING
RETURNING *;

-- name: LockParticipant :one
SELECT * FROM exam_participants WHERE id = $1 FOR UPDATE;

-- name: SetParticipantStatus :exec
UPDATE exam_participants SET status = $2 WHERE id = $1;

-- name: ListCandidateInvitations :many
SELECT ep.id, ep.status, ep.extra_minutes, ep.invited_at, s.name AS schedule_name, s.opens_at, s.closes_at,
       v.duration_minutes, p.name AS package_name, j.title AS job_title, la.id AS attempt_id, la.status AS attempt_status
FROM exam_participants ep
LEFT JOIN exam_attempts la ON la.participant_id = ep.id
     AND la.attempt_no = (SELECT max(x.attempt_no) FROM exam_attempts x WHERE x.participant_id = ep.id)
JOIN exam_schedules s ON s.id = ep.schedule_id
JOIN exam_package_versions v ON v.id = s.package_version_id
JOIN exam_packages p ON p.id = v.package_id
JOIN job_applications ja ON ja.id = ep.application_id
JOIN job_openings j ON j.id = ja.job_id
WHERE ep.candidate_user_id = $1
ORDER BY s.opens_at DESC;

-- name: CountAttempts :one
SELECT count(*)::int FROM exam_attempts WHERE participant_id = $1;

-- name: LatestAttemptForParticipant :one
SELECT * FROM exam_attempts WHERE participant_id = $1 ORDER BY attempt_no DESC LIMIT 1;

-- name: InsertAttempt :one
-- started_at dan deadline_at ditetapkan dari jam database, bukan dari klien.
INSERT INTO exam_attempts (org_id, participant_id, attempt_no, candidate_user_id, package_version_id, started_at, deadline_at, seed, algo_version, last_seen_at)
VALUES (@org_id, @participant_id, @attempt_no, @candidate_user_id, @package_version_id, now(),
        least(now() + make_interval(mins => @minutes::int), @hard_deadline::timestamptz), @seed, @algo_version, now())
RETURNING *;

-- name: InsertAttemptItem :exec
INSERT INTO attempt_items (attempt_id, item_id, position, question_version_id, option_order, option_map)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetAttempt :one
SELECT * FROM exam_attempts WHERE id = $1;

-- name: LockAttempt :one
-- Urutan lock yang sama untuk save, submit dan expiry: attempt dulu, baru baris lain.
SELECT *, clock_timestamp()::timestamptz AS db_now FROM exam_attempts WHERE id = $1 FOR UPDATE;

-- name: ListAttemptItems :many
SELECT ai.item_id, ai.position, ai.option_order, ai.option_map, ai.question_version_id,
       qv.kind, qv.stem, qv.options, qv.answer_key, qv.weight, qv.rubric, qv.version_no AS question_version_no
FROM attempt_items ai JOIN question_versions qv ON qv.id = ai.question_version_id
WHERE ai.attempt_id = $1
ORDER BY ai.position;

-- name: ListAnswers :many
SELECT item_id, value, revision, updated_at FROM exam_answers WHERE attempt_id = $1;

-- name: UpsertAnswer :exec
INSERT INTO exam_answers (attempt_id, item_id, value, revision, op_id, updated_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (attempt_id, item_id) DO UPDATE SET value = EXCLUDED.value, revision = EXCLUDED.revision, op_id = EXCLUDED.op_id, updated_at = now();

-- name: BumpAttemptRevision :one
UPDATE exam_attempts SET revision = revision + 1, last_seen_at = now() WHERE id = $1 RETURNING revision;

-- name: GetSaveReceipt :one
SELECT * FROM exam_save_receipts WHERE attempt_id = $1 AND op_id = $2;

-- name: InsertSaveReceipt :exec
INSERT INTO exam_save_receipts (attempt_id, op_id, payload_digest, revision, response) VALUES ($1, $2, $3, $4, $5);

-- name: Heartbeat :execrows
UPDATE exam_attempts SET last_seen_at = now() WHERE id = $1 AND candidate_user_id = $2 AND status = 'in_progress';

-- name: FinalizeAttempt :exec
UPDATE exam_attempts SET status = $2, finalized_at = now() WHERE id = $1;

-- name: GetSubmission :one
SELECT * FROM exam_submissions WHERE attempt_id = $1;

-- name: InsertSubmission :one
-- Digest dihitung PostgreSQL dari teks kanonis jsonb agar dapat diverifikasi ulang secara independen (rekonsiliasi, drill restore).
INSERT INTO exam_submissions (attempt_id, idempotency_key, reason, receipt_code, answers_digest, snapshot, final_revision)
SELECT sqlc.arg(attempt_id)::uuid, sqlc.narg(idempotency_key)::text, sqlc.arg(reason)::text, sqlc.arg(receipt_code)::text,
       sha256(convert_to(s.j::text, 'UTF8')), s.j, sqlc.arg(final_revision)::bigint
FROM (SELECT sqlc.arg(snapshot)::jsonb AS j) s
RETURNING *;

-- name: InsertResult :exec
INSERT INTO exam_results (attempt_id, org_id, application_id, earned, max_points, score, essay_pending, passed, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: GetResult :one
SELECT * FROM exam_results WHERE attempt_id = $1;

-- name: LockResult :one
SELECT * FROM exam_results WHERE attempt_id = $1 FOR UPDATE;

-- name: UpdateResultScore :exec
UPDATE exam_results SET earned = $2, score = $3, essay_pending = $4, passed = $5,
       status = CASE WHEN status = 'released' THEN 'released' WHEN $4 = 0 THEN 'graded' ELSE 'pending' END,
       version = version + 1, updated_at = now()
WHERE attempt_id = $1;

-- name: ReleaseResult :execrows
UPDATE exam_results SET status = 'released', released_at = now(), released_by = $2, version = version + 1, updated_at = now()
WHERE attempt_id = $1 AND status <> 'released';

-- name: ExpiredAttemptIDs :many
SELECT id FROM exam_attempts WHERE status = 'in_progress' AND deadline_at < now() ORDER BY deadline_at LIMIT $1;

-- name: InsertEssayGrade :exec
INSERT INTO essay_grades (attempt_id, item_id, points, rubric_version, assessor_id, reason) VALUES ($1, $2, $3, $4, $5, $6);

-- name: LatestEssayGrades :many
SELECT DISTINCT ON (item_id) item_id, points, rubric_version, assessor_id, reason, created_at
FROM essay_grades WHERE attempt_id = $1 ORDER BY item_id, id DESC;

-- name: ListEssayGradeHistory :many
SELECT g.*, u.display_name AS assessor_name FROM essay_grades g JOIN users u ON u.id = g.assessor_id
WHERE g.attempt_id = $1 ORDER BY g.id;

-- name: MonitorSchedule :many
-- Satu query agregat berindeks untuk dashboard monitoring (bukan satu query per peserta).
SELECT ep.id AS participant_id, ep.status AS participant_status, ep.extra_minutes, c.display_name AS candidate_name,
       a.id AS attempt_id, a.status AS attempt_status, a.started_at, a.deadline_at, a.last_seen_at, a.revision,
       (SELECT count(*) FROM exam_answers ea WHERE ea.attempt_id = a.id AND ea.value IS NOT NULL)::bigint AS answered,
       (SELECT count(*) FROM attempt_items ai WHERE ai.attempt_id = a.id)::bigint AS total_items,
       r.score, r.essay_pending, r.status AS result_status, r.passed
FROM exam_participants ep
JOIN users c ON c.id = ep.candidate_user_id
LEFT JOIN exam_attempts a ON a.participant_id = ep.id
     AND a.attempt_no = (SELECT max(x.attempt_no) FROM exam_attempts x WHERE x.participant_id = ep.id)
LEFT JOIN exam_results r ON r.attempt_id = a.id
WHERE ep.schedule_id = $1
ORDER BY c.display_name;

-- name: ResultsForSchedule :many
SELECT r.*, a.participant_id FROM exam_results r
JOIN exam_attempts a ON a.id = r.attempt_id
JOIN exam_participants ep ON ep.id = a.participant_id
WHERE ep.schedule_id = $1;

-- name: GetParticipant :one
SELECT * FROM exam_participants WHERE id = $1;
