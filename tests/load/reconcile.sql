-- Rekonsiliasi read-only pascauji untuk satu run k6. Pakai: psql -v run=r20261121a -v seed=20261121 -f tests/load/reconcile.sql
-- Oracle independen dari respons aplikasi: membaca snapshot final, receipt, revisi committed, dan hasil langsung dari DB.
\set prefix 'k6-' :run '-%'
WITH p AS (
    SELECT ep.id AS participant_id, u.email, substring(u.email FROM '-(\d{4})@')::int AS idx
    FROM exam_participants ep JOIN users u ON u.id = ep.candidate_user_id
    WHERE u.email LIKE :'prefix'
), a AS (
    SELECT x.*, p.idx FROM exam_attempts x JOIN p ON p.participant_id = x.participant_id
)
SELECT 'peserta' AS cek, (SELECT count(*) FROM p)::text AS nilai, '' AS harus
UNION ALL SELECT 'attempt', (SELECT count(*) FROM a)::text, '<= peserta'
UNION ALL SELECT 'peserta dengan >1 attempt', (SELECT count(*) FROM (SELECT participant_id FROM a GROUP BY 1 HAVING count(*) > 1) z)::text, '0'
UNION ALL SELECT 'submission', (SELECT count(*) FROM exam_submissions s JOIN a ON a.id = s.attempt_id)::text, '= submits_ok k6 (+ expired)'
UNION ALL SELECT 'submission reason=expired', (SELECT count(*) FROM exam_submissions s JOIN a ON a.id = s.attempt_id WHERE s.reason = 'expired')::text, ''
UNION ALL SELECT 'attempt final tanpa submission', (SELECT count(*) FROM a WHERE a.status <> 'in_progress' AND NOT EXISTS (SELECT 1 FROM exam_submissions s WHERE s.attempt_id = a.id))::text, '0'
UNION ALL SELECT 'hasil != submission', ((SELECT count(*) FROM exam_results r JOIN a ON a.id = r.attempt_id) - (SELECT count(*) FROM exam_submissions s JOIN a ON a.id = s.attempt_id))::text, '0'
UNION ALL SELECT 'digest receipt tidak cocok', (SELECT count(*) FROM exam_submissions s JOIN a ON a.id = s.attempt_id WHERE s.answers_digest <> sha256(convert_to(s.snapshot::text, 'UTF8')))::text, '0'
UNION ALL SELECT 'snapshot beda dari revisi committed', (SELECT count(*) FROM exam_submissions s JOIN a ON a.id = s.attempt_id
        JOIN LATERAL jsonb_array_elements(s.snapshot->'items') si ON true
        LEFT JOIN exam_answers ea ON ea.attempt_id = s.attempt_id AND ea.item_id = (si->>'itemId')::uuid
        WHERE coalesce(ea.revision, 0) <> coalesce((si->>'revision')::int, 0))::text, '0'
UNION ALL SELECT 'receipt save (ACK persist)', (SELECT count(*) FROM exam_save_receipts r JOIN a ON a.id = r.attempt_id)::text, '= acked_ops k6'
UNION ALL SELECT 'tulis lintas peserta', (SELECT count(*) FROM exam_save_receipts r JOIN a ON a.id = r.attempt_id
        WHERE r.op_id NOT LIKE 'k6-' || a.idx || '-%')::text, '0'
UNION ALL SELECT 'revisi attempt != jumlah receipt', (SELECT count(*) FROM a WHERE a.revision <> (SELECT count(*) FROM exam_save_receipts r WHERE r.attempt_id = a.id))::text, '0';
