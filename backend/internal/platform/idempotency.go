package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"

	"ats/db/sqlc"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// IdemScope mengidentifikasi satu operasi idempoten: aktor + operasi + resource + key dari header Idempotency-Key.
type IdemScope struct {
	Actor     uuid.UUID
	Operation string
	Resource  string
	Key       string
	Digest    []byte
}

// IdemFromRequest membaca header Idempotency-Key. Kosong = operasi tidak idempoten (diperbolehkan untuk operasi yang secara alami unik).
func IdemFromRequest(c *gin.Context, actor uuid.UUID, op, resource string, payload any) (*IdemScope, error) {
	key := c.GetHeader("Idempotency-Key")
	if key == "" {
		return nil, nil
	}
	if len(key) < 8 || len(key) > 128 {
		return nil, ErrBadRequest("Idempotency-Key harus 8 sampai 128 karakter.")
	}
	b, _ := json.Marshal(payload)
	d := sha256.Sum256(b)
	return &IdemScope{Actor: actor, Operation: op, Resource: resource, Key: key, Digest: d[:]}, nil
}

// IdemLookup dipanggil di dalam transaksi. found=true berarti hasil lama harus dikembalikan apa adanya.
func IdemLookup(ctx context.Context, q *sqlc.Queries, s *IdemScope) (status int, body json.RawMessage, found bool, err error) {
	if s == nil {
		return 0, nil, false, nil
	}
	row, err := q.GetIdempotency(ctx, sqlc.GetIdempotencyParams{ActorID: s.Actor, Operation: s.Operation, Resource: s.Resource, Key: s.Key})
	if IsNoRows(err) {
		return 0, nil, false, nil
	}
	if err != nil {
		return 0, nil, false, err
	}
	if !bytes.Equal(row.PayloadDigest, s.Digest) {
		return 0, nil, false, ErrConflict("idempotency_mismatch", "Kunci permintaan sudah dipakai untuk data berbeda.")
	}
	return int(row.ResponseStatus), row.ResponseBody, true, nil
}

// IdemStore menyimpan hasil di transaksi yang sama dengan efek bisnisnya.
func IdemStore(ctx context.Context, q *sqlc.Queries, s *IdemScope, status int, body any) error {
	if s == nil {
		return nil
	}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return q.PutIdempotency(ctx, sqlc.PutIdempotencyParams{
		ActorID: s.Actor, Operation: s.Operation, Resource: s.Resource, Key: s.Key,
		PayloadDigest: s.Digest, ResponseStatus: int32(status), ResponseBody: b,
	})
}

func WriteRaw(c *gin.Context, status int, body json.RawMessage) {
	if status == 0 {
		status = http.StatusOK
	}
	c.Header("Idempotent-Replay", "true")
	c.Data(status, "application/json; charset=utf-8", body)
}
