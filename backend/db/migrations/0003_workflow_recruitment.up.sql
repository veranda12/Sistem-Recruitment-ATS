-- Workflow berversi, requisition, lowongan, lamaran, snapshot, histori transisi, approval.

CREATE TABLE workflows (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id             uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    key                text NOT NULL CHECK (key ~ '^[a-z0-9-]{2,60}$'),
    name               text NOT NULL CHECK (length(name) BETWEEN 2 AND 200),
    draft_definition   jsonb NOT NULL,
    draft_revision     integer NOT NULL DEFAULT 1,
    active_version_id  uuid,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    updated_by         uuid REFERENCES users(id) ON DELETE RESTRICT,
    UNIQUE (org_id, key),
    UNIQUE (org_id, id)
);

CREATE TABLE workflow_versions (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        uuid NOT NULL,
    workflow_id   uuid NOT NULL,
    version_no    integer NOT NULL CHECK (version_no > 0),
    definition    jsonb NOT NULL,
    checksum      bytea NOT NULL,
    notes         text NOT NULL DEFAULT '',
    published_by  uuid REFERENCES users(id) ON DELETE RESTRICT,
    published_at  timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (org_id, workflow_id) REFERENCES workflows (org_id, id) ON DELETE RESTRICT,
    UNIQUE (workflow_id, version_no)
);
CREATE TRIGGER workflow_versions_immutable BEFORE UPDATE OR DELETE ON workflow_versions
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

ALTER TABLE workflows ADD CONSTRAINT workflows_active_version_fk
    FOREIGN KEY (active_version_id) REFERENCES workflow_versions(id) ON DELETE RESTRICT;

CREATE TABLE workflow_stages (
    workflow_version_id  uuid NOT NULL REFERENCES workflow_versions(id) ON DELETE RESTRICT,
    key                  text NOT NULL,
    name                 text NOT NULL,
    type                 text NOT NULL CHECK (type IN ('screening', 'interview', 'assessment', 'approval', 'decision', 'terminal')),
    terminal_outcome     text CHECK (terminal_outcome IN ('hired', 'rejected')),
    position             integer NOT NULL,
    PRIMARY KEY (workflow_version_id, key),
    CHECK ((type = 'terminal') = (terminal_outcome IS NOT NULL))
);

-- Riwayat aktivasi versi (publish dan rollback) untuk lamaran baru.
CREATE TABLE workflow_activations (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workflow_id   uuid NOT NULL REFERENCES workflows(id) ON DELETE RESTRICT,
    version_id    uuid NOT NULL REFERENCES workflow_versions(id) ON DELETE RESTRICT,
    action        text NOT NULL CHECK (action IN ('publish', 'rollback')),
    actor_user_id uuid REFERENCES users(id) ON DELETE RESTRICT,
    reason        text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE requisitions (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id             uuid NOT NULL,
    unit_id            uuid NOT NULL,
    location_id        uuid NOT NULL,
    title              text NOT NULL CHECK (length(title) BETWEEN 3 AND 200),
    headcount          integer NOT NULL CHECK (headcount BETWEEN 1 AND 1000),
    employment_type    text NOT NULL CHECK (employment_type IN ('full_time', 'contract', 'internship', 'part_time')),
    justification      text NOT NULL DEFAULT '' CHECK (length(justification) <= 4000),
    status             text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'submitted', 'approved', 'rejected', 'cancelled')),
    requested_by       uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    hiring_manager_id  uuid REFERENCES users(id) ON DELETE RESTRICT,
    decided_by         uuid REFERENCES users(id) ON DELETE RESTRICT,
    decided_at         timestamptz,
    decision_note      text NOT NULL DEFAULT '',
    version            integer NOT NULL DEFAULT 1,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (org_id, unit_id) REFERENCES units (org_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (org_id, location_id) REFERENCES locations (org_id, id) ON DELETE RESTRICT,
    UNIQUE (org_id, id)
);
CREATE INDEX requisitions_unit_idx ON requisitions (org_id, unit_id, created_at DESC);

CREATE TABLE job_openings (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           uuid NOT NULL,
    requisition_id   uuid,
    unit_id          uuid NOT NULL,
    location_id      uuid NOT NULL,
    workflow_id      uuid NOT NULL,
    slug             text NOT NULL CHECK (slug ~ '^[a-z0-9-]{3,120}$'),
    title            text NOT NULL CHECK (length(title) BETWEEN 3 AND 200),
    employment_type  text NOT NULL CHECK (employment_type IN ('full_time', 'contract', 'internship', 'part_time')),
    description      text NOT NULL DEFAULT '' CHECK (length(description) <= 20000),
    requirements     text NOT NULL DEFAULT '' CHECK (length(requirements) <= 20000),
    status           text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'closed')),
    published_at     timestamptz,
    closes_at        timestamptz,
    closed_at        timestamptz,
    created_by       uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    version          integer NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (org_id, requisition_id) REFERENCES requisitions (org_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (org_id, unit_id) REFERENCES units (org_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (org_id, location_id) REFERENCES locations (org_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (org_id, workflow_id) REFERENCES workflows (org_id, id) ON DELETE RESTRICT,
    UNIQUE (org_id, slug),
    UNIQUE (org_id, id),
    CHECK (status <> 'published' OR published_at IS NOT NULL)
);
CREATE INDEX job_openings_public_idx ON job_openings (org_id, published_at DESC, id DESC) WHERE status = 'published';
CREATE INDEX job_openings_unit_idx ON job_openings (org_id, unit_id, created_at DESC);

ALTER TABLE scoped_role_assignments ADD CONSTRAINT sra_job_fk
    FOREIGN KEY (org_id, job_id) REFERENCES job_openings (org_id, id) ON DELETE RESTRICT;

CREATE TABLE job_applications (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id               uuid NOT NULL,
    job_id               uuid NOT NULL,
    candidate_user_id    uuid NOT NULL,
    unit_id              uuid NOT NULL,
    workflow_version_id  uuid NOT NULL,
    stage_key            text NOT NULL,
    status               text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'withdrawn', 'rejected', 'hired')),
    applied_at           timestamptz NOT NULL DEFAULT now(),
    stage_entered_at     timestamptz NOT NULL DEFAULT now(),
    recruiter_user_id    uuid REFERENCES users(id) ON DELETE RESTRICT,
    shortlisted          boolean NOT NULL DEFAULT false,
    version              integer NOT NULL DEFAULT 1,
    updated_at           timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (org_id, job_id) REFERENCES job_openings (org_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (org_id, candidate_user_id) REFERENCES users (org_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (org_id, unit_id) REFERENCES units (org_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workflow_version_id, stage_key) REFERENCES workflow_stages (workflow_version_id, key) ON DELETE RESTRICT,
    UNIQUE (job_id, candidate_user_id),
    UNIQUE (org_id, id)
);
CREATE INDEX job_applications_job_stage_idx ON job_applications (job_id, stage_key, applied_at, id);
CREATE INDEX job_applications_candidate_idx ON job_applications (candidate_user_id, applied_at, id);
CREATE INDEX job_applications_unit_idx ON job_applications (org_id, unit_id, applied_at DESC, id DESC);

CREATE TABLE application_snapshots (
    application_id  uuid PRIMARY KEY REFERENCES job_applications(id) ON DELETE RESTRICT,
    profile         jsonb NOT NULL,
    captured_at     timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER application_snapshots_immutable BEFORE UPDATE OR DELETE ON application_snapshots
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE TABLE transition_history (
    id                   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    org_id               uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    application_id       uuid NOT NULL REFERENCES job_applications(id) ON DELETE RESTRICT,
    execution_id         uuid NOT NULL UNIQUE,
    workflow_version_id  uuid NOT NULL REFERENCES workflow_versions(id) ON DELETE RESTRICT,
    from_stage           text,
    to_stage             text NOT NULL,
    from_status          text,
    to_status            text NOT NULL,
    trigger              text NOT NULL,
    actor_user_id        uuid REFERENCES users(id) ON DELETE RESTRICT,
    actor_kind           text NOT NULL CHECK (actor_kind IN ('staff', 'candidate', 'system')),
    reason               text NOT NULL DEFAULT '',
    rule_ref             jsonb,
    input_summary        jsonb NOT NULL DEFAULT '{}'::jsonb,
    request_id           text,
    created_at           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX transition_history_app_idx ON transition_history (application_id, id);
CREATE INDEX transition_history_org_time_idx ON transition_history (org_id, created_at, id);
CREATE TRIGGER transition_history_append_only BEFORE UPDATE OR DELETE ON transition_history
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE TABLE approvals (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id            uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    application_id    uuid NOT NULL REFERENCES job_applications(id) ON DELETE RESTRICT,
    stage_key         text NOT NULL,
    stage_entry_seq   bigint NOT NULL,
    approver_user_id  uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    decision          text NOT NULL CHECK (decision IN ('approve', 'reject')),
    comment           text NOT NULL DEFAULT '' CHECK (length(comment) <= 2000),
    created_at        timestamptz NOT NULL DEFAULT now(),
    invalidated_at    timestamptz,
    UNIQUE (application_id, stage_key, stage_entry_seq, approver_user_id)
);

CREATE TABLE notifications (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    user_id       uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    event_id      uuid NOT NULL,
    template_key  text NOT NULL,
    subject       text NOT NULL,
    body          text NOT NULL,
    read_at       timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (event_id, user_id, template_key)
);
CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC, id DESC);

CREATE TABLE email_deliveries (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    event_id      uuid NOT NULL,
    template_key  text NOT NULL,
    to_user_id    uuid REFERENCES users(id) ON DELETE RESTRICT,
    to_address    text NOT NULL,
    status        text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed', 'ambiguous')),
    attempts      integer NOT NULL DEFAULT 0,
    last_error    text,
    provider_id   text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    sent_at       timestamptz,
    UNIQUE (event_id, template_key, to_address)
);
