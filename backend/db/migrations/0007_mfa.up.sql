-- MFA TOTP untuk staf: rahasia terenkripsi (users.mfa_secret), langkah terakhir dipakai (anti-replay), recovery code sekali pakai.
ALTER TABLE users ADD COLUMN mfa_last_step bigint;

CREATE TABLE mfa_recovery_codes (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_digest  bytea NOT NULL,
    used_at      timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, code_digest)
);
CREATE INDEX mfa_recovery_codes_user_idx ON mfa_recovery_codes (user_id) WHERE used_at IS NULL;
