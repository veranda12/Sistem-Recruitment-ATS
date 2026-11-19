-- Profil kandidat, identitas (NIK terenkripsi), riwayat pendidikan/pengalaman/sertifikasi, dokumen.

CREATE TABLE candidate_profiles (
    user_id     uuid PRIMARY KEY,
    org_id      uuid NOT NULL,
    full_name   text NOT NULL DEFAULT '' CHECK (length(full_name) <= 200),
    phone       text NOT NULL DEFAULT '' CHECK (phone = '' OR phone ~ '^\+?[0-9]{8,15}$'),
    birth_date  date,
    city        text NOT NULL DEFAULT '' CHECK (length(city) <= 100),
    address     text NOT NULL DEFAULT '' CHECK (length(address) <= 500),
    summary     text NOT NULL DEFAULT '' CHECK (length(summary) <= 2000),
    version     integer NOT NULL DEFAULT 1,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (org_id, user_id) REFERENCES users (org_id, id) ON DELETE RESTRICT
);

CREATE TABLE candidate_identities (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id             uuid NOT NULL,
    user_id            uuid NOT NULL UNIQUE,
    nik_ciphertext     bytea NOT NULL,
    nik_nonce          bytea NOT NULL CHECK (length(nik_nonce) = 12),
    key_version        integer NOT NULL,
    nik_blind_index    bytea NOT NULL CHECK (length(nik_blind_index) = 32),
    blind_key_version  integer NOT NULL,
    nik_last4          text NOT NULL CHECK (nik_last4 ~ '^[0-9]{4}$'),
    verification       text NOT NULL DEFAULT 'self_declared'
                       CHECK (verification IN ('self_declared', 'verified', 'disputed')),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (org_id, user_id) REFERENCES users (org_id, id) ON DELETE RESTRICT,
    UNIQUE (org_id, nik_blind_index)
);

-- Klaim NIK yang bentrok dengan akun lain: jalur verifikasi manual, tidak ada merge otomatis.
CREATE TABLE identity_claims (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    claimant_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    nik_blind_index  bytea NOT NULL,
    status           text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved', 'rejected')),
    note             text NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now(),
    resolved_by      uuid REFERENCES users(id) ON DELETE RESTRICT,
    resolved_at      timestamptz
);
CREATE UNIQUE INDEX identity_claims_open_idx ON identity_claims (claimant_user_id) WHERE status = 'open';

CREATE TABLE education (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES candidate_profiles(user_id) ON DELETE CASCADE,
    level        text NOT NULL CHECK (level IN ('SMA', 'SMK', 'D3', 'D4', 'S1', 'S2', 'S3', 'Lainnya')),
    institution  text NOT NULL CHECK (length(institution) BETWEEN 2 AND 200),
    major        text NOT NULL DEFAULT '' CHECK (length(major) <= 200),
    start_year   integer CHECK (start_year BETWEEN 1950 AND 2100),
    end_year     integer CHECK (end_year BETWEEN 1950 AND 2100),
    gpa          numeric(4,2) CHECK (gpa BETWEEN 0 AND 4),
    created_at   timestamptz NOT NULL DEFAULT now(),
    CHECK (end_year IS NULL OR start_year IS NULL OR end_year >= start_year)
);
CREATE INDEX education_user_idx ON education (user_id);

CREATE TABLE experiences (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES candidate_profiles(user_id) ON DELETE CASCADE,
    company      text NOT NULL CHECK (length(company) BETWEEN 2 AND 200),
    title        text NOT NULL CHECK (length(title) BETWEEN 2 AND 200),
    start_date   date NOT NULL,
    end_date     date,
    description  text NOT NULL DEFAULT '' CHECK (length(description) <= 2000),
    created_at   timestamptz NOT NULL DEFAULT now(),
    CHECK (end_date IS NULL OR end_date >= start_date)
);
CREATE INDEX experiences_user_idx ON experiences (user_id);

CREATE TABLE certifications (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES candidate_profiles(user_id) ON DELETE CASCADE,
    name        text NOT NULL CHECK (length(name) BETWEEN 2 AND 200),
    issuer      text NOT NULL DEFAULT '' CHECK (length(issuer) <= 200),
    issued_on   date,
    expires_on  date,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX certifications_user_idx ON certifications (user_id);

CREATE TABLE documents (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    owner_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    kind          text NOT NULL CHECK (kind IN ('cv', 'ijazah', 'sertifikat', 'lainnya')),
    file_name     text NOT NULL CHECK (length(file_name) BETWEEN 1 AND 200),
    content_type  text NOT NULL,
    size_bytes    bigint NOT NULL CHECK (size_bytes > 0),
    object_key    text NOT NULL UNIQUE,
    sha256        bytea NOT NULL,
    scan_status   text NOT NULL DEFAULT 'pending' CHECK (scan_status IN ('pending', 'clean', 'infected', 'error')),
    scanned_at    timestamptz,
    deleted_at    timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX documents_owner_idx ON documents (owner_user_id, created_at DESC) WHERE deleted_at IS NULL;
