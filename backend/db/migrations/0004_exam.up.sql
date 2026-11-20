-- Ujian: bank soal + versi, paket + versi, jadwal, peserta, attempt, jawaban berevisi, receipt, submission, hasil.

CREATE TABLE question_banks (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    name        text NOT NULL CHECK (length(name) BETWEEN 2 AND 200),
    category    text NOT NULL DEFAULT '' CHECK (length(category) <= 100),
    created_by  uuid REFERENCES users(id) ON DELETE RESTRICT,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, id)
);

CREATE TABLE questions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL,
    bank_id     uuid NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (org_id, bank_id) REFERENCES question_banks (org_id, id) ON DELETE RESTRICT
);

-- options: [{id, text}] ; answer_key: id opsi benar (privat, tidak pernah dikirim ke peserta).
CREATE TABLE question_versions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    question_id  uuid NOT NULL REFERENCES questions(id) ON DELETE RESTRICT,
    version_no   integer NOT NULL CHECK (version_no > 0),
    kind         text NOT NULL CHECK (kind IN ('single', 'multiple', 'essay')),
    stem         text NOT NULL CHECK (length(stem) BETWEEN 1 AND 10000),
    options      jsonb NOT NULL DEFAULT '[]'::jsonb,
    answer_key   jsonb NOT NULL DEFAULT '[]'::jsonb,
    weight       numeric(6,2) NOT NULL DEFAULT 1 CHECK (weight > 0),
    rubric       text NOT NULL DEFAULT '',
    created_by   uuid REFERENCES users(id) ON DELETE RESTRICT,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (question_id, version_no)
);
CREATE TRIGGER question_versions_immutable BEFORE UPDATE OR DELETE ON question_versions
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE TABLE exam_packages (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    key         text NOT NULL CHECK (key ~ '^[a-z0-9-]{2,60}$'),
    name        text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, key),
    UNIQUE (org_id, id)
);

-- question_version_ids: urutan dasar butir; versi paket immutable setelah terbit.
CREATE TABLE exam_package_versions (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id                uuid NOT NULL,
    package_id            uuid NOT NULL,
    version_no            integer NOT NULL CHECK (version_no > 0),
    question_version_ids  uuid[] NOT NULL CHECK (cardinality(question_version_ids) BETWEEN 1 AND 200),
    duration_minutes      integer NOT NULL CHECK (duration_minutes BETWEEN 1 AND 600),
    pass_score            numeric(5,2) NOT NULL CHECK (pass_score BETWEEN 0 AND 100),
    shuffle_items         boolean NOT NULL DEFAULT true,
    shuffle_options       boolean NOT NULL DEFAULT true,
    published_by          uuid REFERENCES users(id) ON DELETE RESTRICT,
    published_at          timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (org_id, package_id) REFERENCES exam_packages (org_id, id) ON DELETE RESTRICT,
    UNIQUE (package_id, version_no),
    UNIQUE (org_id, id)
);
CREATE TRIGGER exam_package_versions_immutable BEFORE UPDATE OR DELETE ON exam_package_versions
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE TABLE exam_schedules (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              uuid NOT NULL,
    package_version_id  uuid NOT NULL,
    name                text NOT NULL CHECK (length(name) BETWEEN 2 AND 200),
    opens_at            timestamptz NOT NULL,
    closes_at           timestamptz NOT NULL,
    max_attempts        integer NOT NULL DEFAULT 1 CHECK (max_attempts BETWEEN 1 AND 5),
    created_by          uuid REFERENCES users(id) ON DELETE RESTRICT,
    created_at          timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (org_id, package_version_id) REFERENCES exam_package_versions (org_id, id) ON DELETE RESTRICT,
    CHECK (closes_at > opens_at),
    UNIQUE (org_id, id)
);

-- Undangan: satu per (jadwal, lamaran). extra_minutes = akomodasi waktu yang diberikan staf (teraudit).
CREATE TABLE exam_participants (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id             uuid NOT NULL,
    schedule_id        uuid NOT NULL,
    application_id     uuid NOT NULL REFERENCES job_applications(id) ON DELETE RESTRICT,
    candidate_user_id  uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    extra_minutes      integer NOT NULL DEFAULT 0 CHECK (extra_minutes BETWEEN 0 AND 240),
    status             text NOT NULL DEFAULT 'invited' CHECK (status IN ('invited', 'started', 'submitted', 'expired', 'cancelled')),
    invited_by         uuid REFERENCES users(id) ON DELETE RESTRICT,
    invited_at         timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (org_id, schedule_id) REFERENCES exam_schedules (org_id, id) ON DELETE RESTRICT,
    UNIQUE (schedule_id, application_id)
);
CREATE INDEX exam_participants_candidate_idx ON exam_participants (candidate_user_id, invited_at DESC);
CREATE INDEX exam_participants_schedule_idx ON exam_participants (schedule_id, status);

CREATE TABLE exam_attempts (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    participant_id      uuid NOT NULL REFERENCES exam_participants(id) ON DELETE RESTRICT,
    attempt_no          integer NOT NULL CHECK (attempt_no > 0),
    candidate_user_id   uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    package_version_id  uuid NOT NULL REFERENCES exam_package_versions(id) ON DELETE RESTRICT,
    status              text NOT NULL DEFAULT 'in_progress' CHECK (status IN ('in_progress', 'submitted', 'expired')),
    started_at          timestamptz NOT NULL,
    deadline_at         timestamptz NOT NULL,
    seed                bytea NOT NULL CHECK (length(seed) = 32),
    algo_version        integer NOT NULL,
    revision            bigint NOT NULL DEFAULT 0,
    last_seen_at        timestamptz,
    finalized_at        timestamptz,
    UNIQUE (participant_id, attempt_no),
    CHECK (deadline_at > started_at)
);
CREATE INDEX exam_attempts_expiry_idx ON exam_attempts (deadline_at) WHERE status = 'in_progress';
CREATE INDEX exam_attempts_candidate_idx ON exam_attempts (candidate_user_id);

-- option_map: {opaqueId: originalOptionId} dan option_order: urutan opaqueId yang ditampilkan.
CREATE TABLE attempt_items (
    attempt_id           uuid NOT NULL REFERENCES exam_attempts(id) ON DELETE RESTRICT,
    item_id              uuid NOT NULL,
    position             integer NOT NULL,
    question_version_id  uuid NOT NULL REFERENCES question_versions(id) ON DELETE RESTRICT,
    option_order         jsonb NOT NULL DEFAULT '[]'::jsonb,
    option_map           jsonb NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (attempt_id, item_id),
    UNIQUE (attempt_id, position)
);

CREATE TABLE exam_answers (
    attempt_id  uuid NOT NULL,
    item_id     uuid NOT NULL,
    value       jsonb,
    revision    integer NOT NULL CHECK (revision > 0),
    op_id       text NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (attempt_id, item_id),
    FOREIGN KEY (attempt_id, item_id) REFERENCES attempt_items (attempt_id, item_id) ON DELETE RESTRICT
);

-- Receipt per operation ID: retry setelah ACK hilang mengembalikan hasil yang sama, bahkan setelah deadline.
CREATE TABLE exam_save_receipts (
    attempt_id      uuid NOT NULL REFERENCES exam_attempts(id) ON DELETE RESTRICT,
    op_id           text NOT NULL CHECK (length(op_id) BETWEEN 8 AND 100),
    payload_digest  bytea NOT NULL,
    revision        bigint NOT NULL,
    response        bytea NOT NULL, -- byte persis respons ACK agar retry identik
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (attempt_id, op_id)
);

CREATE TABLE exam_submissions (
    attempt_id       uuid PRIMARY KEY REFERENCES exam_attempts(id) ON DELETE RESTRICT,
    idempotency_key  text,
    reason           text NOT NULL CHECK (reason IN ('submitted', 'expired')),
    receipt_code     text NOT NULL UNIQUE,
    answers_digest   bytea NOT NULL,
    snapshot         jsonb NOT NULL,
    final_revision   bigint NOT NULL,
    submitted_at     timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER exam_submissions_immutable BEFORE UPDATE OR DELETE ON exam_submissions
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE TABLE exam_results (
    attempt_id       uuid PRIMARY KEY REFERENCES exam_attempts(id) ON DELETE RESTRICT,
    org_id           uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    application_id   uuid NOT NULL REFERENCES job_applications(id) ON DELETE RESTRICT,
    earned           numeric(8,2) NOT NULL,
    max_points       numeric(8,2) NOT NULL CHECK (max_points > 0),
    score            numeric(5,2),
    essay_pending    integer NOT NULL DEFAULT 0,
    passed           boolean,
    status           text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'graded', 'released')),
    released_at      timestamptz,
    released_by      uuid REFERENCES users(id) ON DELETE RESTRICT,
    version          integer NOT NULL DEFAULT 1,
    updated_at       timestamptz NOT NULL DEFAULT now()
);

-- Riwayat penilaian esai (append-only). Nilai yang berlaku = baris terbaru per item.
CREATE TABLE essay_grades (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    attempt_id       uuid NOT NULL REFERENCES exam_attempts(id) ON DELETE RESTRICT,
    item_id          uuid NOT NULL,
    points           numeric(6,2) NOT NULL CHECK (points >= 0),
    rubric_version   integer NOT NULL,
    assessor_id      uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    reason           text NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (attempt_id, item_id) REFERENCES attempt_items (attempt_id, item_id) ON DELETE RESTRICT
);
CREATE INDEX essay_grades_item_idx ON essay_grades (attempt_id, item_id, id DESC);
CREATE TRIGGER essay_grades_append_only BEFORE UPDATE OR DELETE ON essay_grades
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
