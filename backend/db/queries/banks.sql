-- name: ListQuestionBanks :many
SELECT b.*, (SELECT count(*) FROM questions q WHERE q.bank_id = b.id)::bigint AS question_count
FROM question_banks b WHERE b.org_id = $1 ORDER BY b.name;

-- name: GetQuestionBank :one
SELECT * FROM question_banks WHERE id = $1;

-- name: ListBankLatestVersions :many
-- Versi terbaru tiap soal dalam bank. Kunci jawaban ikut karena endpoint ini khusus examiner.
SELECT DISTINCT ON (q.id) q.id AS question_id, v.id AS version_id, v.version_no, v.kind, v.stem, v.options, v.answer_key, v.weight, v.rubric, v.created_at
FROM questions q JOIN question_versions v ON v.question_id = q.id
WHERE q.bank_id = $1
ORDER BY q.id, v.version_no DESC;

-- name: GetQuestion :one
SELECT q.*, b.org_id AS bank_org FROM questions q JOIN question_banks b ON b.id = q.bank_id WHERE q.id = $1;

-- name: LockQuestion :one
SELECT * FROM questions WHERE id = $1 FOR UPDATE;

-- name: NextQuestionVersionNo :one
SELECT coalesce(max(version_no), 0)::int + 1 FROM question_versions WHERE question_id = $1;

-- name: ListPackages :many
SELECT p.*, v.version_no AS latest_version_no, v.duration_minutes, v.pass_score, cardinality(v.question_version_ids)::int AS item_count, v.published_at
FROM exam_packages p
LEFT JOIN exam_package_versions v ON v.package_id = p.id AND v.version_no = (SELECT max(x.version_no) FROM exam_package_versions x WHERE x.package_id = p.id)
WHERE p.org_id = $1 ORDER BY p.name;

-- name: NextPackageVersionNo :one
SELECT coalesce(max(version_no), 0)::int + 1 FROM exam_package_versions WHERE package_id = $1;

-- name: LockPackage :one
SELECT * FROM exam_packages WHERE id = $1 FOR UPDATE;

-- name: CountQuestionVersionsInOrg :one
SELECT count(*)::int FROM question_versions v JOIN questions q ON q.id = v.question_id WHERE v.id = ANY(@ids::uuid[]) AND q.org_id = @org_id;
