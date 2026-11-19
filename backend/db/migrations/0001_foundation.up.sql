-- Foundation: organisasi, user, sesi, RBAC, audit, outbox, idempotensi, pengaturan.

CREATE TABLE organizations (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9-]{2,64}$'),
    name        text NOT NULL CHECK (length(name) BETWEEN 2 AND 200),
    timezone    text NOT NULL DEFAULT 'Asia/Jakarta',
    profile     jsonb NOT NULL DEFAULT '{}'::jsonb,
    brand       jsonb NOT NULL DEFAULT '{}'::jsonb,
    version     integer NOT NULL DEFAULT 1,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE units (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    code        text NOT NULL CHECK (code ~ '^[A-Z0-9-]{2,20}$'),
    name        text NOT NULL CHECK (length(name) BETWEEN 2 AND 200),
    active      boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, code),
    UNIQUE (org_id, id)
);

CREATE TABLE locations (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    code        text NOT NULL CHECK (code ~ '^[A-Z0-9-]{2,20}$'),
    name        text NOT NULL CHECK (length(name) BETWEEN 2 AND 200),
    city        text NOT NULL,
    active      boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, code),
    UNIQUE (org_id, id)
);

CREATE TABLE users (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id             uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    kind               text NOT NULL CHECK (kind IN ('staff', 'candidate')),
    email              text NOT NULL CHECK (email = lower(email) AND length(email) BETWEEN 3 AND 254),
    display_name       text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 200),
    password_hash      text NOT NULL,
    email_verified_at  timestamptz,
    status             text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    mfa_secret         bytea,
    mfa_enabled_at     timestamptz,
    password_changed_at timestamptz NOT NULL DEFAULT now(),
    version            integer NOT NULL DEFAULT 1,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, email),
    UNIQUE (org_id, id)
);

CREATE TABLE sessions (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_digest         bytea NOT NULL UNIQUE,
    created_at           timestamptz NOT NULL DEFAULT now(),
    last_seen_at         timestamptz NOT NULL DEFAULT now(),
    idle_expires_at      timestamptz NOT NULL,
    absolute_expires_at  timestamptz NOT NULL,
    revoked_at           timestamptz,
    revoke_reason        text,
    ip                   text,
    user_agent           text
);
CREATE INDEX sessions_user_active_idx ON sessions (user_id) WHERE revoked_at IS NULL;

CREATE TABLE email_tokens (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose       text NOT NULL CHECK (purpose IN ('verify_email', 'reset_password')),
    token_digest  bytea NOT NULL UNIQUE,
    expires_at    timestamptz NOT NULL,
    used_at       timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX email_tokens_user_idx ON email_tokens (user_id, purpose) WHERE used_at IS NULL;

CREATE TABLE permissions (
    key          text PRIMARY KEY CHECK (key ~ '^[a-z_]+\.[a-z_]+$'),
    description  text NOT NULL
);

CREATE TABLE roles (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    key         text NOT NULL CHECK (key ~ '^[a-z_]{2,40}$'),
    name        text NOT NULL,
    system      boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, key),
    UNIQUE (org_id, id)
);

CREATE TABLE role_permissions (
    role_id         uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_key  text NOT NULL REFERENCES permissions(key) ON DELETE RESTRICT,
    PRIMARY KEY (role_id, permission_key)
);

-- job_id FK ditambahkan di migrasi rekrutmen setelah job_openings ada.
CREATE TABLE scoped_role_assignments (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    user_id     uuid NOT NULL,
    role_id     uuid NOT NULL,
    unit_id     uuid,
    job_id      uuid,
    granted_by  uuid REFERENCES users(id) ON DELETE RESTRICT,
    created_at  timestamptz NOT NULL DEFAULT now(),
    revoked_at  timestamptz,
    revoked_by  uuid REFERENCES users(id) ON DELETE RESTRICT,
    FOREIGN KEY (org_id, user_id) REFERENCES users (org_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (org_id, role_id) REFERENCES roles (org_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (org_id, unit_id) REFERENCES units (org_id, id) ON DELETE RESTRICT
);
CREATE INDEX sra_user_active_idx ON scoped_role_assignments (user_id) WHERE revoked_at IS NULL;
CREATE UNIQUE INDEX sra_unique_active_idx ON scoped_role_assignments
    (user_id, role_id, coalesce(unit_id, '00000000-0000-0000-0000-000000000000'::uuid),
     coalesce(job_id, '00000000-0000-0000-0000-000000000000'::uuid))
    WHERE revoked_at IS NULL;

CREATE TABLE audit_logs (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    org_id         uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    actor_user_id  uuid REFERENCES users(id) ON DELETE RESTRICT,
    actor_kind     text NOT NULL CHECK (actor_kind IN ('staff', 'candidate', 'system', 'anonymous')),
    action         text NOT NULL,
    target_type    text NOT NULL,
    target_id      text,
    scope_unit_id  uuid,
    outcome        text NOT NULL CHECK (outcome IN ('success', 'denied', 'failed')),
    reason         text,
    request_id     text,
    detail         jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_org_time_idx ON audit_logs (org_id, created_at DESC, id DESC);
CREATE INDEX audit_logs_target_idx ON audit_logs (target_type, target_id, created_at DESC);

-- Audit append-only: tolak UPDATE/DELETE di level database.
CREATE FUNCTION forbid_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% pada % tidak diizinkan (append-only)', TG_OP, TG_TABLE_NAME
        USING ERRCODE = 'restrict_violation';
END;
$$;
CREATE TRIGGER audit_logs_append_only BEFORE UPDATE OR DELETE ON audit_logs
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE TABLE outbox (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    topic           text NOT NULL,
    aggregate_type  text NOT NULL,
    aggregate_id    text NOT NULL,
    payload         jsonb NOT NULL,
    status          text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'done', 'dead')),
    attempts        integer NOT NULL DEFAULT 0,
    max_attempts    integer NOT NULL DEFAULT 8,
    available_at    timestamptz NOT NULL DEFAULT now(),
    locked_until    timestamptz,
    last_error      text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    processed_at    timestamptz
);
CREATE INDEX outbox_pending_idx ON outbox (available_at, id) WHERE status = 'pending';
CREATE INDEX outbox_dead_idx ON outbox (created_at) WHERE status = 'dead';

CREATE TABLE processed_events (
    consumer      text NOT NULL,
    event_id      uuid NOT NULL,
    processed_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
);

CREATE TABLE idempotency_keys (
    actor_id        uuid NOT NULL,
    operation       text NOT NULL,
    resource        text NOT NULL,
    key             text NOT NULL CHECK (length(key) BETWEEN 8 AND 128),
    payload_digest  bytea NOT NULL,
    response_status integer NOT NULL,
    response_body   jsonb NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (actor_id, operation, resource, key)
);

CREATE TABLE system_settings (
    org_id        uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    key           text NOT NULL,
    value         jsonb NOT NULL,
    version       integer NOT NULL DEFAULT 1,
    effective_at  timestamptz NOT NULL DEFAULT now(),
    updated_by    uuid REFERENCES users(id) ON DELETE RESTRICT,
    updated_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, key)
);
