// Package notify: inbox kandidat, template email, dan pengiriman dengan deduplikasi per event.
package notify

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"mime"
	"net"
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
	if id == "" {
		r := make([]byte, 8)
		_, _ = rand.Read(r)
		id = hex.EncodeToString(r)
	}
	// Date wajib menurut RFC 5322; tanpanya penyedia seperti Gmail cenderung menandai pesan sebagai spam.
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%s@ats.local>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n",
		from, m.To, mime.QEncoding.Encode("utf-8", m.Subject), time.Now().Format(time.RFC1123Z), id, m.Body)
	return b.Bytes()
}

// SMTPMailer: pengiriman melalui server SMTP. Tanpa Username dipakai sebagai relay internal tanpa login; dengan
// Username/Password memakai AUTH PLAIN (mis. Gmail dengan App Password, Brevo, Mailgun). AUTH hanya dikirim lewat TLS:
// STARTTLS (port 587) bila server mendukung, atau TLS langsung bila ImplicitTLS (port 465).
type SMTPMailer struct {
	Addr        string
	From        string
	Username    string
	Password    string
	ImplicitTLS bool
}

func (s SMTPMailer) Send(ctx context.Context, m Message) (string, error) {
	done := make(chan error, 1)
	go func() { done <- s.send(m) }()
	select {
	case err := <-done:
		return m.IdempotencyKey, err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s SMTPMailer) send(m Message) error {
	from := s.From
	if i := strings.Index(from, "<"); i >= 0 {
		from = strings.Trim(from[i:], "<>")
	}
	host, _, err := net.SplitHostPort(s.Addr)
	if err != nil {
		return fmt.Errorf("SMTP_ADDR harus host:port: %w", err)
	}
	var auth smtp.Auth
	if s.Username != "" {
		// PlainAuth menolak mengirim kredensial lewat koneksi tanpa TLS (kecuali localhost).
		auth = smtp.PlainAuth("", s.Username, s.Password, host)
	}
	msg := render(s.From, m, m.IdempotencyKey)
	if !s.ImplicitTLS {
		return smtp.SendMail(s.Addr, auth, from, []string{m.To}, msg)
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", s.Addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(m.To); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
