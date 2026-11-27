-- Track rekrutmen (OPERATOR, STAFF, MT_S1_D4), field PTK tambahan, dan kuota lowongan.
-- Baris lama: track = 'STAFF'. Kuota = headcount requisition hanya bila lowongan itu satu-satunya lowongan draft/published
-- dari requisition-nya; selain itu 1. Aturan total kuota <= headcount diperiksa di aplikasi saat create/update, bukan constraint.

ALTER TABLE requisitions
    ADD COLUMN track text NOT NULL DEFAULT 'STAFF' CHECK (track IN ('OPERATOR', 'STAFF', 'MT_S1_D4')),
    ADD COLUMN line text CHECK (line IS NULL OR char_length(line) <= 100),
    ADD COLUMN target_join_date date,
    ADD COLUMN reason_category text CHECK (reason_category IN ('penambahan', 'penggantian', 'proyek_baru'));
ALTER TABLE requisitions ALTER COLUMN track DROP DEFAULT;
ALTER TABLE requisitions ADD CONSTRAINT requisitions_org_id_id_track_key UNIQUE (org_id, id, track);

ALTER TABLE job_openings
    ADD COLUMN track text NOT NULL DEFAULT 'STAFF' CHECK (track IN ('OPERATOR', 'STAFF', 'MT_S1_D4')),
    ADD COLUMN quota integer;
ALTER TABLE job_openings ALTER COLUMN track DROP DEFAULT;

UPDATE job_openings j SET quota = CASE
    WHEN j.requisition_id IS NOT NULL AND j.status IN ('draft', 'published')
         AND (SELECT count(*) FROM job_openings o
              WHERE o.org_id = j.org_id AND o.requisition_id = j.requisition_id AND o.status IN ('draft', 'published')) = 1
    THEN (SELECT r.headcount FROM requisitions r WHERE r.org_id = j.org_id AND r.id = j.requisition_id)
    ELSE 1
END;

ALTER TABLE job_openings ALTER COLUMN quota SET NOT NULL;
ALTER TABLE job_openings ADD CONSTRAINT job_openings_quota_check CHECK (quota >= 1);
-- Lowongan yang terhubung ke requisition wajib memakai track yang sama. MATCH SIMPLE: tanpa requisition, FK tidak diperiksa.
ALTER TABLE job_openings ADD CONSTRAINT job_openings_requisition_track_fk
    FOREIGN KEY (org_id, requisition_id, track) REFERENCES requisitions (org_id, id, track) ON DELETE RESTRICT;
