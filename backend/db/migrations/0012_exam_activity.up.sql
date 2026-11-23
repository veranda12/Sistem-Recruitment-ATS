-- Log perilaku peserta selama ujian (tanpa kamera): pindah tab, keluar jendela, salin, tempel, koneksi.
-- Waktu kejadian berasal dari perangkat (perkiraan); client_seq mencegah duplikasi saat dikirim ulang.
-- Peserta diberi tahu di ruang ujian bahwa aktivitas ini dicatat.
CREATE TABLE exam_attempt_events (
    id           bigserial PRIMARY KEY,
    attempt_id   uuid NOT NULL REFERENCES exam_attempts(id) ON DELETE RESTRICT,
    client_seq   integer NOT NULL CHECK (client_seq > 0),
    kind         text NOT NULL CHECK (kind IN ('tab_hidden', 'tab_visible', 'window_blur', 'copy', 'paste', 'offline', 'online')),
    occurred_at  timestamptz NOT NULL,
    duration_ms  integer CHECK (duration_ms IS NULL OR duration_ms >= 0),
    received_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (attempt_id, client_seq)
);
CREATE INDEX exam_attempt_events_attempt_idx ON exam_attempt_events (attempt_id, occurred_at);
