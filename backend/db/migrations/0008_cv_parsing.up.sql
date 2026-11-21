-- Pembacaan CV: run berantrean (PostgreSQL = sumber kebenaran; Asynq hanya sinyal), hasil antara,
-- saran per field dengan bukti, dan histori review. Profil kandidat diperluas untuk menampung hasilnya.

-- ---- Profil: field tambahan (semua opsional, tidak mengubah data lama) ----
ALTER TABLE candidate_profiles
    ADD COLUMN contact_email text NOT NULL DEFAULT '' CHECK (contact_email = '' OR (length(contact_email) <= 254 AND contact_email ~ '^[^@\s]+@[^@\s]+\.[^@\s]+$'));

ALTER TABLE experiences
    ADD COLUMN location        text NOT NULL DEFAULT '' CHECK (length(location) <= 100),
    ADD COLUMN start_precision text NOT NULL DEFAULT 'day' CHECK (start_precision IN ('day', 'month', 'year')),
    ADD COLUMN end_precision   text NOT NULL DEFAULT 'day' CHECK (end_precision IN ('day', 'month', 'year')),
    ADD COLUMN is_current      boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT experiences_current_no_end CHECK (NOT is_current OR end_date IS NULL);

ALTER TABLE certifications
    ADD COLUMN issued_precision text NOT NULL DEFAULT 'day' CHECK (issued_precision IN ('day', 'month', 'year'));

CREATE TABLE candidate_skills (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES candidate_profiles(user_id) ON DELETE CASCADE,
    name       text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX candidate_skills_unique ON candidate_skills (user_id, lower(name));

CREATE TABLE candidate_languages (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES candidate_profiles(user_id) ON DELETE CASCADE,
    name       text NOT NULL CHECK (length(name) BETWEEN 1 AND 60),
    level      text NOT NULL DEFAULT '' CHECK (length(level) <= 60),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX candidate_languages_unique ON candidate_languages (user_id, lower(name));

CREATE TABLE candidate_links (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES candidate_profiles(user_id) ON DELETE CASCADE,
    label      text NOT NULL DEFAULT '' CHECK (length(label) <= 200),
    url        text NOT NULL CHECK (length(url) <= 500 AND url ~ '^https?://'),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX candidate_links_unique ON candidate_links (user_id, url);

CREATE TABLE candidate_projects (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES candidate_profiles(user_id) ON DELETE CASCADE,
    name        text NOT NULL CHECK (length(name) BETWEEN 2 AND 200),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 2000),
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX candidate_projects_user_idx ON candidate_projects (user_id);

-- ---- Run pembacaan CV ----
CREATE TABLE cv_parse_runs (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id                uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    candidate_user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    document_id           uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    parser_config_version text NOT NULL,
    status                text NOT NULL DEFAULT 'queued' CHECK (status IN
                          ('waiting_scan', 'queued', 'processing', 'ready_for_review', 'partial', 'failed', 'applied', 'dismissed', 'cancelled')),
    stage                 text NOT NULL DEFAULT 'queued' CHECK (length(stage) <= 40),
    pages_total           integer,
    pages_done            integer NOT NULL DEFAULT 0,
    attempt               integer NOT NULL DEFAULT 0,
    max_attempts          integer NOT NULL DEFAULT 3 CHECK (max_attempts BETWEEN 1 AND 10),
    -- fence naik setiap klaim; worker lama yang kehilangan lease tidak dapat menulis hasil.
    fence                 bigint NOT NULL DEFAULT 0,
    lease_owner           text,
    lease_until           timestamptz,
    heartbeat_at          timestamptz,
    next_attempt_at       timestamptz NOT NULL DEFAULT now(),
    dispatched_at         timestamptz,
    failure_reason        text CHECK (failure_reason IS NULL OR length(failure_reason) <= 60),
    failure_transient     boolean,
    dead_lettered_at      timestamptz,
    manifest              jsonb,
    review_version        integer NOT NULL DEFAULT 1,
    queue_ms              integer,
    process_ms            integer,
    queued_at             timestamptz NOT NULL DEFAULT now(),
    started_at            timestamptz,
    finished_at           timestamptz,
    applied_at            timestamptz,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    UNIQUE (candidate_user_id, document_id, parser_config_version)
);
CREATE INDEX cv_parse_runs_due_idx ON cv_parse_runs (next_attempt_at) WHERE status IN ('queued', 'processing');
CREATE INDEX cv_parse_runs_candidate_idx ON cv_parse_runs (candidate_user_id, created_at DESC);
CREATE INDEX cv_parse_runs_status_idx ON cv_parse_runs (status, created_at);
CREATE INDEX cv_parse_runs_document_idx ON cv_parse_runs (document_id);

CREATE TABLE cv_parse_pages (
    run_id         uuid NOT NULL REFERENCES cv_parse_runs(id) ON DELETE CASCADE,
    page_no        integer NOT NULL CHECK (page_no >= 1),
    method         text NOT NULL,
    reason         text NOT NULL DEFAULT '',
    status         text NOT NULL CHECK (status IN ('ok', 'failed')),
    failure_reason text,
    width          real,
    height         real,
    scale          real,
    image_name     text,
    PRIMARY KEY (run_id, page_no)
);

-- Representasi perantara (page/line/word) dengan koordinat; aksesnya sama ketatnya dengan dokumen asli.
CREATE TABLE cv_parse_artifacts (
    run_id     uuid PRIMARY KEY REFERENCES cv_parse_runs(id) ON DELETE CASCADE,
    ir         jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE cv_evidence_refs (
    id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id   uuid NOT NULL REFERENCES cv_parse_runs(id) ON DELETE CASCADE,
    line_id  text NOT NULL,
    page_no  integer NOT NULL,
    bbox     real[] CHECK (bbox IS NULL OR array_length(bbox, 1) = 4),
    snippet  text NOT NULL CHECK (length(snippet) <= 2000),
    source   text NOT NULL CHECK (source IN ('native', 'ocr', 'docx')),
    locator  text NOT NULL DEFAULT '',
    UNIQUE (run_id, line_id)
);

CREATE TABLE cv_field_suggestions (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id        uuid NOT NULL REFERENCES cv_parse_runs(id) ON DELETE CASCADE,
    path          text NOT NULL,
    section       text NOT NULL,
    entry_key     text NOT NULL DEFAULT '',
    field         text NOT NULL,
    raw_value     text NOT NULL DEFAULT '',
    normalized    jsonb,
    read_doubts   text[] NOT NULL DEFAULT '{}',
    assoc_doubts  text[] NOT NULL DEFAULT '{}',
    status        text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'edited', 'rejected')),
    edited_value  jsonb,
    sort          integer NOT NULL,
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (run_id, path)
);
CREATE INDEX cv_field_suggestions_run_idx ON cv_field_suggestions (run_id, sort);

CREATE TABLE cv_suggestion_evidence (
    suggestion_id uuid NOT NULL REFERENCES cv_field_suggestions(id) ON DELETE CASCADE,
    evidence_id   uuid NOT NULL REFERENCES cv_evidence_refs(id) ON DELETE CASCADE,
    PRIMARY KEY (suggestion_id, evidence_id)
);

CREATE TABLE cv_review_events (
    id            bigserial PRIMARY KEY,
    run_id        uuid NOT NULL REFERENCES cv_parse_runs(id) ON DELETE CASCADE,
    suggestion_id uuid REFERENCES cv_field_suggestions(id) ON DELETE SET NULL,
    actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    action        text NOT NULL CHECK (action IN ('accept', 'edit', 'reject', 'reset', 'apply', 'dismiss', 'replay', 'cancel')),
    detail        jsonb NOT NULL DEFAULT '{}',
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX cv_review_events_run_idx ON cv_review_events (run_id, id);

INSERT INTO permissions (key, description) VALUES ('cv_parses.replay', 'Menjalankan ulang pembacaan CV yang gagal permanen (dead-letter)')
ON CONFLICT DO NOTHING;
INSERT INTO role_permissions (role_id, permission_key)
SELECT id, 'cv_parses.replay' FROM roles WHERE key = 'super_admin'
ON CONFLICT DO NOTHING;
