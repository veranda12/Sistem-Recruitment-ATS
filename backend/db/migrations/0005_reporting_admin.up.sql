-- Ekspor laporan (pekerjaan asinkron) dan riwayat versi pengaturan.

CREATE TABLE exports (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    requested_by  uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    kind          text NOT NULL CHECK (kind IN ('funnel', 'applications')),
    params        jsonb NOT NULL DEFAULT '{}'::jsonb,
    status        text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'ready', 'failed', 'expired')),
    file_key      text,
    row_count     integer,
    error         text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    finished_at   timestamptz,
    expires_at    timestamptz
);
CREATE INDEX exports_user_idx ON exports (requested_by, created_at DESC);
CREATE INDEX exports_expiry_idx ON exports (expires_at) WHERE status = 'ready';

-- Riwayat pengaturan: setiap perubahan menyimpan versi lama beserta waktu efektif dan pelaku.
CREATE TABLE system_setting_history (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    org_id        uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    key           text NOT NULL,
    version       integer NOT NULL,
    value         jsonb NOT NULL,
    effective_at  timestamptz NOT NULL,
    changed_by    uuid REFERENCES users(id) ON DELETE RESTRICT,
    reason        text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, key, version)
);
CREATE TRIGGER system_setting_history_append_only BEFORE UPDATE OR DELETE ON system_setting_history
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE INDEX transition_history_app_stage_idx ON transition_history (application_id, to_stage);
