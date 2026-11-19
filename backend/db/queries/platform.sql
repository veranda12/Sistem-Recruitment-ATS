-- name: InsertAudit :exec
INSERT INTO audit_logs (org_id, actor_user_id, actor_kind, action, target_type, target_id, scope_unit_id, outcome, reason, request_id, detail)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: ListAuditForTarget :many
SELECT * FROM audit_logs WHERE target_type = $1 AND target_id = $2 ORDER BY id;

-- name: InsertOutbox :one
INSERT INTO outbox (org_id, topic, aggregate_type, aggregate_id, payload)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: ClaimOutbox :many
-- Klaim batch pending; lease melalui locked_until agar worker lain tidak mengambil ulang selama diproses.
UPDATE outbox SET locked_until = now() + make_interval(secs => @lease_seconds::int), attempts = attempts + 1
WHERE id IN (
    SELECT o.id FROM outbox o
    WHERE o.status = 'pending' AND o.available_at <= now()
      AND (o.locked_until IS NULL OR o.locked_until < now())
    ORDER BY o.available_at, o.id
    LIMIT @batch_size::int
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: CompleteOutbox :exec
UPDATE outbox SET status = 'done', processed_at = now(), locked_until = NULL, last_error = NULL WHERE id = $1;

-- name: RetryOutbox :exec
UPDATE outbox SET available_at = $2, locked_until = NULL, last_error = $3 WHERE id = $1;

-- name: DeadOutbox :exec
UPDATE outbox SET status = 'dead', locked_until = NULL, last_error = $2, processed_at = now() WHERE id = $1;

-- name: ReplayDeadOutbox :execrows
UPDATE outbox SET status = 'pending', attempts = 0, available_at = now(), last_error = NULL, processed_at = NULL
WHERE id = $1 AND status = 'dead';

-- name: OutboxStats :one
SELECT
    count(*) FILTER (WHERE status = 'pending')::bigint AS pending,
    count(*) FILTER (WHERE status = 'dead')::bigint AS dead,
    coalesce(extract(epoch FROM now() - min(available_at) FILTER (WHERE status = 'pending')), 0)::float8 AS oldest_pending_seconds
FROM outbox;

-- name: MarkEventProcessed :execrows
INSERT INTO processed_events (consumer, event_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: GetIdempotency :one
SELECT * FROM idempotency_keys WHERE actor_id = $1 AND operation = $2 AND resource = $3 AND key = $4;

-- name: PutIdempotency :exec
INSERT INTO idempotency_keys (actor_id, operation, resource, key, payload_digest, response_status, response_body)
VALUES ($1, $2, $3, $4, $5, $6, $7);
