-- Interview, panel interviewer dengan pencegahan bentrok jadwal di level database, scorecard, dan pemberitahuan keputusan.
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE interviews (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    application_id  uuid NOT NULL REFERENCES job_applications(id) ON DELETE RESTRICT,
    starts_at       timestamptz NOT NULL,
    ends_at         timestamptz NOT NULL,
    timezone        text NOT NULL,
    mode            text NOT NULL CHECK (mode IN ('onsite', 'video', 'phone')),
    location        text NOT NULL DEFAULT '' CHECK (length(location) <= 300),
    status          text NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'cancelled', 'completed')),
    created_by      uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    version         integer NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at AND ends_at - starts_at <= interval '8 hours')
);
CREATE INDEX interviews_application_idx ON interviews (application_id, starts_at);

-- Satu baris per interviewer. Exclusion constraint menolak dua interview aktif yang tumpang tindih untuk interviewer sama,
-- termasuk saat dua penjadwalan terjadi bersamaan.
CREATE TABLE interview_panelists (
    interview_id    uuid NOT NULL REFERENCES interviews(id) ON DELETE RESTRICT,
    interviewer_id  uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    period          tstzrange NOT NULL,
    active          boolean NOT NULL DEFAULT true,
    PRIMARY KEY (interview_id, interviewer_id),
    CONSTRAINT interview_panelists_no_overlap EXCLUDE USING gist (interviewer_id WITH =, period WITH &&) WHERE (active)
);

-- Riwayat jadwal (reschedule mencatat waktu lama/baru beserta zona waktu).
CREATE TABLE interview_changes (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    interview_id  uuid NOT NULL REFERENCES interviews(id) ON DELETE RESTRICT,
    action        text NOT NULL CHECK (action IN ('schedule', 'reschedule', 'cancel', 'complete')),
    old_starts_at timestamptz,
    new_starts_at timestamptz,
    timezone      text NOT NULL,
    reason        text NOT NULL DEFAULT '',
    actor_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER interview_changes_append_only BEFORE UPDATE OR DELETE ON interview_changes
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE TABLE evaluations (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    interview_id    uuid NOT NULL REFERENCES interviews(id) ON DELETE RESTRICT,
    evaluator_id    uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    rating          integer NOT NULL CHECK (rating BETWEEN 1 AND 5),
    recommendation  text NOT NULL CHECK (recommendation IN ('lanjut', 'pertimbangkan', 'tidak_lanjut')),
    notes           text NOT NULL DEFAULT '' CHECK (length(notes) <= 4000),
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (interview_id, evaluator_id)
);

ALTER TABLE job_applications ADD COLUMN decision_notified_at timestamptz;
ALTER TABLE job_applications ADD COLUMN decision_notified_by uuid REFERENCES users(id) ON DELETE RESTRICT;
