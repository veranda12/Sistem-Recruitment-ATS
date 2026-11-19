package platform

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"

	"ats/db/sqlc"

	"github.com/google/uuid"
)

// AuditEntry: actor, action, target, scope, outcome, requestId. Detail tidak boleh memuat PII mentah.
type AuditEntry struct {
	OrgID      uuid.UUID
	ActorID    *uuid.UUID
	ActorKind  string
	Action     string
	TargetType string
	TargetID   string
	ScopeUnit  *uuid.UUID
	Outcome    string
	Reason     string
	RequestID  string
	Detail     map[string]any
}

func WriteAudit(ctx context.Context, q *sqlc.Queries, e AuditEntry) error {
	if e.Outcome == "" {
		e.Outcome = "success"
	}
	if e.Detail == nil {
		e.Detail = map[string]any{}
	}
	detail, err := json.Marshal(e.Detail)
	if err != nil {
		return err
	}
	return q.InsertAudit(ctx, sqlc.InsertAuditParams{
		OrgID: e.OrgID, ActorUserID: e.ActorID, ActorKind: e.ActorKind, Action: e.Action,
		TargetType: e.TargetType, TargetID: strPtr(e.TargetID), ScopeUnitID: e.ScopeUnit,
		Outcome: e.Outcome, Reason: strPtr(e.Reason), RequestID: strPtr(e.RequestID), Detail: detail,
	})
}

// Enqueue menulis event outbox di transaksi caller. Worker mengirim setelah commit; email tidak pernah dikirim di dalam transaksi.
func Enqueue(ctx context.Context, q *sqlc.Queries, orgID uuid.UUID, topic, aggType, aggID string, payload any) (uuid.UUID, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return uuid.Nil, err
	}
	return q.InsertOutbox(ctx, sqlc.InsertOutboxParams{OrgID: orgID, Topic: topic, AggregateType: aggType, AggregateID: aggID, Payload: b})
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// RandomToken menghasilkan token 32 byte base64url dan digest SHA-256 untuk disimpan.
func RandomToken() (token string, digest []byte) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, Digest(token)
}

func Digest(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}
