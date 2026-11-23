-- Template pesan yang diubah admin, berversi. Template bawaan tetap di kode (notify.Templates); baris di sini
-- menggantikannya per organisasi, kunci, dan kanal. Versi terbaru = yang dipakai. Riwayat tidak pernah dihapus,
-- sehingga mengembalikan ke versi lama atau ke bawaan menambah versi baru.
CREATE TABLE message_templates (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    key         text NOT NULL CHECK (key ~ '^[a-z_]{2,64}$'),
    channel     text NOT NULL CHECK (channel IN ('email', 'whatsapp')),
    version     integer NOT NULL CHECK (version > 0),
    subject     text NOT NULL DEFAULT '' CHECK (length(subject) <= 200),
    body        text NOT NULL CHECK (length(body) BETWEEN 1 AND 5000),
    is_default  boolean NOT NULL DEFAULT false, -- true = salinan template bawaan (hasil "kembalikan ke bawaan")
    note        text NOT NULL DEFAULT '' CHECK (length(note) <= 500),
    created_by  uuid REFERENCES users(id) ON DELETE RESTRICT,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, key, channel, version)
);
CREATE INDEX message_templates_latest_idx ON message_templates (org_id, key, channel, version DESC);
