-- Waktu proses dihitung dari klaim percobaan terakhir (bukan heartbeat terakhir).
ALTER TABLE cv_parse_runs ADD COLUMN claimed_at timestamptz;
