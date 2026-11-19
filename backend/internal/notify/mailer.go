// Package notify: inbox kandidat, template email, dan pengiriman dengan deduplikasi per event.
package notify

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mime"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Message struct {
	To      string
	Subject string
	Body    string
	// IdempotencyKey diteruskan sebagai header Message-ID agar provider yang mendukung dapat mendeduplikasi.
	IdempotencyKey string
}

type Mailer interface {
	Send(ctx context.Context, m Message) (providerID string, err error)
}

// FileMailer menulis .eml ke direktori privat. Dipakai di dev/test sebagai pengganti SMTP lokal.
type FileMailer struct {
	Dir  string
	From string
}

func (f FileMailer) Send(_ context.Context, m Message) (string, error) {
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		return "", err
	}
	id := m.IdempotencyKey
	if id == "" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		id = hex.EncodeToString(b)
	}
	name := fmt.Sprintf("%s_%s.eml", time.Now().UTC().Format("20060102T150405.000"), safe(m.To)+"_"+id[:8])
	return id, os.WriteFile(filepath.Join(f.Dir, name), render(f.From, m, id), 0o600)
}

func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-' {
			return r
		}
		return '_'
	}, strings.ToLower(s))
}

func render(from string, m Message, id string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMessage-ID: <%s@ats.local>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n",
		from, m.To, mime.QEncoding.Encode("utf-8", m.Subject), id, m.Body)
	return b.Bytes()
}

// SMTPMailer: pengiriman melalui relay SMTP pelanggan (tanpa autentikasi untuk relay internal; tambahkan bila diperlukan).
type SMTPMailer struct {
	Addr string
	From string
}

func (s SMTPMailer) Send(ctx context.Context, m Message) (string, error) {
	done := make(chan error, 1)
	go func() {
		addr := s.From
		if i := strings.Index(addr, "<"); i >= 0 {
			addr = strings.Trim(addr[i:], "<>")
		}
		done <- smtp.SendMail(s.Addr, nil, addr, []string{m.To}, render(s.From, m, m.IdempotencyKey))
	}()
	select {
	case err := <-done:
		return m.IdempotencyKey, err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
