-- Hak subjek data dan retensi: anonimisasi kandidat (atas permintaan atau otomatis setelah masa simpan) dan legal hold.
ALTER TABLE users ADD COLUMN anonymized_at timestamptz;
ALTER TABLE users ADD COLUMN legal_hold_at timestamptz;
ALTER TABLE users ADD COLUMN legal_hold_reason text NOT NULL DEFAULT '' CHECK (length(legal_hold_reason) <= 500);

-- Snapshot lamaran dan snapshot pengiriman ujian adalah bukti proses yang tidak boleh diubah, kecuali isinya dianonimkan
-- dalam transaksi penghapusan privasi yang menandai dirinya dengan SET LOCAL ats.privacy_erasure = 'on'.
-- Penghapusan baris tetap dilarang; tabel append-only lain (audit, histori transisi) tidak terpengaruh.
CREATE FUNCTION forbid_mutation_unless_erasure() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND current_setting('ats.privacy_erasure', true) = 'on' THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION '% pada % tidak diizinkan (append-only)', TG_OP, TG_TABLE_NAME
        USING ERRCODE = 'restrict_violation';
END;
$$;
DROP TRIGGER application_snapshots_immutable ON application_snapshots;
CREATE TRIGGER application_snapshots_immutable BEFORE UPDATE OR DELETE ON application_snapshots
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation_unless_erasure();
DROP TRIGGER exam_submissions_immutable ON exam_submissions;
CREATE TRIGGER exam_submissions_immutable BEFORE UPDATE OR DELETE ON exam_submissions
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation_unless_erasure();

-- Izin baru untuk data pribadi lintas unit. Bawaan hanya Super Admin (berlaku se-organisasi); organisasi dapat
-- memberikannya lewat peran kustom dengan scope seluruh organisasi. Endpoint menolak pemegang berscope unit/lowongan.
INSERT INTO permissions (key, description) VALUES ('privacy.manage', 'Mengekspor dan menganonimkan data kandidat, legal hold, retensi')
ON CONFLICT (key) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_key)
SELECT id, 'privacy.manage' FROM roles WHERE system AND key = 'super_admin'
ON CONFLICT DO NOTHING;
