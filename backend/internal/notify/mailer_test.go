package notify

import (
	"bufio"
	"context"
	"encoding/base64"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeSMTP adalah server SMTP minimal di loopback: mengiklankan AUTH PLAIN (tanpa STARTTLS; net/smtp mengizinkan
// PlainAuth ke localhost) dan mencatat kredensial serta isi DATA yang diterima.
type fakeSMTP struct {
	addr string
	auth chan string
	data chan string
}

func startFakeSMTP(t *testing.T, requireAuth bool) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	f := &fakeSMTP{addr: ln.Addr().String(), auth: make(chan string, 1), data: make(chan string, 1)}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		w := func(s string) { conn.Write([]byte(s + "\r\n")) }
		w("220 fake ESMTP")
		authed := !requireAuth
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.TrimRight(line, "\r\n")
			switch {
			case strings.HasPrefix(cmd, "EHLO"):
				w("250-fake")
				w("250 AUTH PLAIN")
			case strings.HasPrefix(cmd, "AUTH PLAIN "):
				raw, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(cmd, "AUTH PLAIN "))
				f.auth <- string(raw)
				authed = true
				w("235 ok")
			case strings.HasPrefix(cmd, "MAIL FROM"):
				if !authed {
					w("530 authentication required")
					continue
				}
				w("250 ok")
			case strings.HasPrefix(cmd, "RCPT TO"):
				w("250 ok")
			case cmd == "DATA":
				w("354 go")
				var b strings.Builder
				for {
					l, err := r.ReadString('\n')
					if err != nil || l == ".\r\n" {
						break
					}
					b.WriteString(l)
				}
				f.data <- b.String()
				w("250 queued")
			case cmd == "QUIT":
				w("221 bye")
				return
			default:
				w("250 ok")
			}
		}
	}()
	return f
}

func TestSMTPMailerAuthPlain(t *testing.T) {
	f := startFakeSMTP(t, true)
	m := SMTPMailer{Addr: f.addr, From: "Rekrutmen <no-reply@contoh.test>", Username: "akun@contoh.test", Password: "sandi-aplikasi"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := m.Send(ctx, Message{To: "kandidat@contoh.test", Subject: "Verifikasi email Anda", Body: "Halo", IdempotencyKey: "abc12345"}); err != nil {
		t.Fatalf("kirim: %v", err)
	}
	if got := <-f.auth; got != "\x00akun@contoh.test\x00sandi-aplikasi" {
		t.Fatalf("kredensial AUTH PLAIN salah: %q", got)
	}
	data := <-f.data
	for _, want := range []string{"To: kandidat@contoh.test", "Date: ", "Message-ID: <abc12345@ats.local>", "Halo"} {
		if !strings.Contains(data, want) {
			t.Errorf("pesan tidak memuat %q:\n%s", want, data)
		}
	}
}

func TestSMTPMailerRelayTanpaLogin(t *testing.T) {
	f := startFakeSMTP(t, false)
	m := SMTPMailer{Addr: f.addr, From: "no-reply@contoh.test"}
	if _, err := m.Send(context.Background(), Message{To: "kandidat@contoh.test", Subject: "Uji", Body: "Isi"}); err != nil {
		t.Fatalf("kirim relay: %v", err)
	}
	select {
	case a := <-f.auth:
		t.Fatalf("relay tanpa login tidak boleh mengirim AUTH, didapat %q", a)
	default:
	}
	if data := <-f.data; !strings.Contains(data, "Message-ID: <") {
		t.Fatalf("Message-ID wajib ada walau IdempotencyKey kosong:\n%s", data)
	}
}
