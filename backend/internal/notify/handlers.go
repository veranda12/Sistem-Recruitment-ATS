package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"ats/db/sqlc"
	"ats/internal/platform"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Template berversi di kode untuk fase awal. Placeholder hanya dari allowlist; nilai disisipkan sebagai teks polos.
type Template struct {
	Key     string
	Version int
	Subject string
	Body    string
}

var Placeholders = []string{"nama", "lowongan", "tahap", "tautan", "organisasi"}

var placeholderRe = regexp.MustCompile(`\{\{\s*([a-z_]+)\s*\}\}`)

var Templates = map[string]Template{
	"verify_email": {Key: "verify_email", Version: 1, Subject: "Verifikasi email Anda",
		Body: "Halo {{nama}},\n\nKlik tautan berikut untuk memverifikasi email Anda di portal karier {{organisasi}}:\n{{tautan}}\n\nTautan berlaku 24 jam. Abaikan email ini bila Anda tidak mendaftar."},
	"password_reset": {Key: "password_reset", Version: 1, Subject: "Atur ulang kata sandi",
		Body: "Halo {{nama}},\n\nGunakan tautan berikut untuk mengatur ulang kata sandi:\n{{tautan}}\n\nTautan berlaku 30 menit dan hanya dapat dipakai sekali. Abaikan email ini bila Anda tidak memintanya."},
	"account_exists": {Key: "account_exists", Version: 1, Subject: "Akun Anda sudah terdaftar",
		Body: "Halo {{nama}},\n\nAda permintaan pendaftaran dengan email ini, padahal akun Anda sudah ada. Bila Anda lupa kata sandi, gunakan:\n{{tautan}}"},
	"application_submitted": {Key: "application_submitted", Version: 1, Subject: "Lamaran diterima: {{lowongan}}",
		Body: "Halo {{nama}},\n\nLamaran Anda untuk posisi {{lowongan}} sudah kami terima. Pantau statusnya di:\n{{tautan}}"},
	"stage_changed": {Key: "stage_changed", Version: 1, Subject: "Pembaruan lamaran: {{lowongan}}",
		Body: "Halo {{nama}},\n\nLamaran Anda untuk posisi {{lowongan}} kini berada pada tahap {{tahap}}. Detail:\n{{tautan}}"},
	"application_withdrawn": {Key: "application_withdrawn", Version: 1, Subject: "Lamaran ditarik: {{lowongan}}",
		Body: "Halo {{nama}},\n\nLamaran Anda untuk posisi {{lowongan}} sudah ditarik sesuai permintaan Anda."},
}

// Render mengganti placeholder allowlist. Placeholder tak dikenal menghasilkan error (dicek juga saat template disimpan).
func Render(t Template, vars map[string]string) (subject, body string, err error) {
	sub := func(s string) (string, error) {
		var bad string
		out := placeholderRe.ReplaceAllStringFunc(s, func(m string) string {
			k := placeholderRe.FindStringSubmatch(m)[1]
			v, ok := vars[k]
			if !ok {
				bad = k
				return m
			}
			return v
		})
		if bad != "" {
			return "", fmt.Errorf("placeholder %q tidak diizinkan atau tidak diisi", bad)
		}
		return out, nil
	}
	if subject, err = sub(t.Subject); err != nil {
		return
	}
	// Header injection: subject satu baris.
	subject = strings.NewReplacer("\r", " ", "\n", " ").Replace(subject)
	body, err = sub(t.Body)
	return
}

type Handlers struct {
	Pool   *pgxpool.Pool
	Q      *sqlc.Queries
	Mailer Mailer
	Origin string
}

// Handle memproses satu event outbox. Mengembalikan error untuk retry; efek bisnis dideduplikasi per consumer + event ID.
func (h *Handlers) Handle(ctx context.Context, ev sqlc.Outbox) error {
	switch ev.Topic {
	case "auth.verify_email":
		return h.tokenEmail(ctx, ev, "verify_email", 24*time.Hour, "/verifikasi-email?token=")
	case "auth.password_reset":
		return h.tokenEmail(ctx, ev, "password_reset", 30*time.Minute, "/atur-ulang-sandi?token=")
	case "auth.account_exists":
		return h.userEmail(ctx, ev, "account_exists", map[string]string{"tautan": h.Origin + "/lupa-sandi"})
	case "application.submitted":
		return h.applicationEvent(ctx, ev, "application_submitted", "")
	case "application.transitioned":
		var p struct {
			From, To, Status, FromStatus string
		}
		_ = json.Unmarshal(ev.Payload, &p)
		switch {
		case p.Status == "withdrawn" && p.FromStatus != "withdrawn":
			return h.applicationEvent(ctx, ev, "application_withdrawn", "")
		case p.Status == "active" && p.From != p.To:
			return h.applicationEvent(ctx, ev, "stage_changed", p.To)
		}
		// Hasil akhir (diterima/ditolak) dikirim recruiter secara eksplisit, bukan otomatis.
		return nil
	}
	return nil
}

type userPayload struct {
	UserID uuid.UUID `json:"userId"`
}

func (h *Handlers) orgName(ctx context.Context, orgID uuid.UUID) string {
	if o, err := h.Q.GetOrganization(ctx, orgID); err == nil {
		return o.Name
	}
	return ""
}

func (h *Handlers) tokenEmail(ctx context.Context, ev sqlc.Outbox, tpl string, ttl time.Duration, path string) error {
	var p userPayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return err
	}
	u, err := h.Q.GetUser(ctx, p.UserID)
	if err != nil {
		return err
	}
	if tpl == "verify_email" && u.EmailVerifiedAt != nil {
		return nil
	}
	// Token dibuat di worker dan hanya ada di email; DB menyimpan digest.
	token, digest := platform.RandomToken()
	if _, err := h.Q.CreateEmailToken(ctx, sqlc.CreateEmailTokenParams{UserID: u.ID, Purpose: map[string]string{"verify_email": "verify_email", "password_reset": "reset_password"}[tpl],
		TokenDigest: digest, ExpiresAt: time.Now().Add(ttl)}); err != nil {
		return err
	}
	return h.send(ctx, ev, u, tpl, map[string]string{"tautan": h.Origin + path + token})
}

func (h *Handlers) userEmail(ctx context.Context, ev sqlc.Outbox, tpl string, vars map[string]string) error {
	var p userPayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return err
	}
	u, err := h.Q.GetUser(ctx, p.UserID)
	if err != nil {
		return err
	}
	return h.send(ctx, ev, u, tpl, vars)
}

func (h *Handlers) applicationEvent(ctx context.Context, ev sqlc.Outbox, tpl, stageKey string) error {
	var p struct {
		ApplicationID uuid.UUID `json:"applicationId"`
	}
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return err
	}
	app, err := h.Q.GetApplication(ctx, p.ApplicationID)
	if err != nil {
		return err
	}
	u, err := h.Q.GetUser(ctx, app.CandidateUserID)
	if err != nil {
		return err
	}
	vars := map[string]string{"lowongan": app.JobTitle, "tahap": app.StageName, "tautan": h.Origin + "/akun/lamaran/" + app.ID.String()}
	_ = stageKey
	// Inbox: idempoten melalui unique(event_id, user_id, template_key).
	t := Templates[tpl]
	vars["nama"], vars["organisasi"] = u.DisplayName, h.orgName(ctx, u.OrgID)
	subject, body, err := Render(t, vars)
	if err != nil {
		return err
	}
	if _, err := h.Q.InsertNotification(ctx, sqlc.InsertNotificationParams{OrgID: u.OrgID, UserID: u.ID, EventID: ev.ID,
		TemplateKey: tpl, Subject: subject, Body: body}); err != nil {
		return err
	}
	return h.send(ctx, ev, u, tpl, vars)
}

// send: catat delivery sebelum kirim, kirim di luar transaksi, lalu catat hasil.
func (h *Handlers) send(ctx context.Context, ev sqlc.Outbox, u sqlc.User, tpl string, vars map[string]string) error {
	t := Templates[tpl]
	if vars["nama"] == "" {
		vars["nama"] = u.DisplayName
	}
	if vars["organisasi"] == "" {
		vars["organisasi"] = h.orgName(ctx, u.OrgID)
	}
	subject, body, err := Render(t, vars)
	if err != nil {
		return err
	}
	var d sqlc.BeginDeliveryRow
	err = platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var err error
		d, err = h.Q.WithTx(tx).BeginDelivery(ctx, sqlc.BeginDeliveryParams{OrgID: u.OrgID, EventID: ev.ID, TemplateKey: tpl, ToUserID: &u.ID, ToAddress: u.Email})
		return err
	})
	if err != nil {
		return err
	}
	switch {
	case d.Status == "sent" || d.Status == "ambiguous":
		return nil
	case d.Status == "pending" && d.Attempts > 1:
		// Percobaan sebelumnya terhenti setelah mulai kirim: tandai untuk rekonsiliasi, jangan kirim ulang otomatis.
		return h.Q.FinishDelivery(ctx, sqlc.FinishDeliveryParams{ID: d.ID, Status: "ambiguous", LastError: strp("percobaan sebelumnya terhenti sebelum hasil tercatat")})
	}
	pid, err := h.Mailer.Send(ctx, Message{To: u.Email, Subject: subject, Body: body, IdempotencyKey: ev.ID.String() + "-" + tpl})
	if err != nil {
		_ = h.Q.FinishDelivery(ctx, sqlc.FinishDeliveryParams{ID: d.ID, Status: "failed", LastError: strp(truncate(err.Error(), 500))})
		return fmt.Errorf("kirim email %s: %w", tpl, err)
	}
	return h.Q.FinishDelivery(ctx, sqlc.FinishDeliveryParams{ID: d.ID, Status: "sent", ProviderID: &pid})
}

func strp(s string) *string { return &s }

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
