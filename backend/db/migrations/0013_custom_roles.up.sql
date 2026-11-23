-- Peran kustom: versi untuk perubahan bersamaan dan arsip (peran yang pernah dipakai tidak dihapus agar riwayat
-- penugasan dan audit tetap utuh). Peran sistem (system = true) tidak dapat diubah lewat aplikasi.
ALTER TABLE roles ADD COLUMN version integer NOT NULL DEFAULT 1;
ALTER TABLE roles ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE roles ADD COLUMN archived_at timestamptz;
