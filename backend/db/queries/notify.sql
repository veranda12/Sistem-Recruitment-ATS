-- name: InsertNotification :execrows
INSERT INTO notifications (org_id, user_id, event_id, template_key, subject, body)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (event_id, user_id, template_key) DO NOTHING;

-- name: ListNotifications :many
SELECT * FROM notifications WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT 100;

-- name: MarkNotificationRead :execrows
UPDATE notifications SET read_at = coalesce(read_at, now()) WHERE id = $1 AND user_id = $2;

-- name: BeginDelivery :one
-- Mengembalikan status sebelum percobaan ini. 'pending' dengan attempts > 0 berarti percobaan sebelumnya
-- terhenti di antara kirim dan catat: hasilnya ambigu dan harus direkonsiliasi, bukan dikirim ulang diam-diam.
INSERT INTO email_deliveries (org_id, event_id, template_key, to_user_id, to_address, attempts)
VALUES ($1, $2, $3, $4, $5, 1)
ON CONFLICT (event_id, template_key, to_address)
DO UPDATE SET attempts = email_deliveries.attempts + 1
RETURNING id, status, attempts;

-- name: FinishDelivery :exec
UPDATE email_deliveries SET status = $2, last_error = $3, provider_id = $4,
       sent_at = CASE WHEN $2 = 'sent' THEN now() ELSE sent_at END
WHERE id = $1;

-- name: ListDeliveries :many
SELECT d.*, u.display_name AS to_name FROM email_deliveries d LEFT JOIN users u ON u.id = d.to_user_id
ORDER BY d.created_at DESC LIMIT 200;

-- name: BeginWhatsAppDelivery :one
INSERT INTO email_deliveries (org_id, event_id, template_key, to_user_id, to_address, attempts, channel)
VALUES ($1, $2, $3, $4, $5, 1, 'whatsapp')
ON CONFLICT (event_id, template_key, to_address)
DO UPDATE SET attempts = email_deliveries.attempts + 1
RETURNING id, status, attempts;

-- name: GetWhatsAppPreference :one
SELECT phone, whatsapp_opt_in_at FROM candidate_profiles WHERE user_id = $1;

-- name: SetWhatsAppOptIn :one
UPDATE candidate_profiles SET whatsapp_opt_in_at = $2 WHERE user_id = $1
RETURNING phone, whatsapp_opt_in_at;
