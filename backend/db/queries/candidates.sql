-- name: CreateCandidateProfile :exec
INSERT INTO candidate_profiles (user_id, org_id, full_name) VALUES ($1, $2, $3);

-- name: GetCandidateProfile :one
SELECT * FROM candidate_profiles WHERE user_id = $1;

-- name: UpdateCandidateProfile :one
UPDATE candidate_profiles SET full_name = $3, phone = $4, birth_date = $5, city = $6, address = $7, summary = $8,
       version = version + 1, updated_at = now()
WHERE user_id = $1 AND version = $2
RETURNING *;

-- name: InsertCandidateIdentity :one
INSERT INTO candidate_identities (org_id, user_id, nik_ciphertext, nik_nonce, key_version, nik_blind_index, blind_key_version, nik_last4)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT DO NOTHING
RETURNING id;

-- name: GetCandidateIdentity :one
SELECT * FROM candidate_identities WHERE user_id = $1;

-- name: OpenIdentityClaim :exec
INSERT INTO identity_claims (org_id, claimant_user_id, nik_blind_index, note)
VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING;

-- name: ListEducation :many
SELECT * FROM education WHERE user_id = $1 ORDER BY coalesce(end_year, 9999) DESC, id;

-- name: ListExperiences :many
SELECT * FROM experiences WHERE user_id = $1 ORDER BY start_date DESC, id;

-- name: ListCertifications :many
SELECT * FROM certifications WHERE user_id = $1 ORDER BY issued_on DESC NULLS LAST, id;

-- name: DeleteEducationForUser :exec
DELETE FROM education WHERE user_id = $1;

-- name: InsertEducation :exec
INSERT INTO education (user_id, level, institution, major, start_year, end_year, gpa) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: DeleteExperiencesForUser :exec
DELETE FROM experiences WHERE user_id = $1;

-- name: InsertExperience :exec
INSERT INTO experiences (user_id, company, title, start_date, end_date, description) VALUES ($1, $2, $3, $4, $5, $6);

-- name: DeleteCertificationsForUser :exec
DELETE FROM certifications WHERE user_id = $1;

-- name: InsertCertification :exec
INSERT INTO certifications (user_id, name, issuer, issued_on, expires_on) VALUES ($1, $2, $3, $4, $5);
