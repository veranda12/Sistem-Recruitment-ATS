-- Pemberitahuan WhatsApp: persetujuan eksplisit kandidat (waktu persetujuan disimpan; null = tidak setuju) dan
-- kanal pada catatan pengiriman. Tautan sekali pakai (verifikasi, atur ulang sandi) tetap hanya lewat email.
ALTER TABLE candidate_profiles ADD COLUMN whatsapp_opt_in_at timestamptz;
ALTER TABLE email_deliveries ADD COLUMN channel text NOT NULL DEFAULT 'email' CHECK (channel IN ('email', 'whatsapp'));
