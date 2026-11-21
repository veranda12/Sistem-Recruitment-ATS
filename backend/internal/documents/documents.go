// Package documents: unggah dokumen kandidat ke karantina privat, pemindaian, dan unduh bertanda tangan berumur pendek.
package documents

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// MaxBytes = batas terbesar (CV); jenis lain dibatasi MaxBytesOther.
	MaxBytes      = 15 << 20
	MaxBytesOther = 5 << 20
	MaxPerUser    = 10
	TopicScan     = "document.uploaded"
	LinkTTL       = 60 * time.Second
	quarantineDir = "quarantine"
	cleanDir      = "clean"
)

var Kinds = []string{"cv", "ijazah", "sertifikat", "lainnya"}

// sniff mengenali tipe dari signature byte, bukan dari nama berkas atau header klien.
func sniff(head []byte, name string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(name))
	switch {
	case bytes.HasPrefix(head, []byte("%PDF-")) && ext == ".pdf":
		return "application/pdf", true
	case bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")) && ext == ".png":
		return "image/png", true
	case bytes.HasPrefix(head, []byte("\xff\xd8\xff")) && (ext == ".jpg" || ext == ".jpeg"):
		return "image/jpeg", true
	case bytes.HasPrefix(head, []byte("PK\x03\x04")) && ext == ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document", true
	case len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP" && ext == ".webp":
		return "image/webp", true
	case bytes.HasPrefix(head, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}) && ext == ".doc":
		return "application/msword", true
	}
	return "", false
}

// safeName membuang path dan karakter kontrol; nama asli hanya untuk tampilan, objek disimpan dengan nama acak.
func safeName(n string) string {
	n = filepath.Base(strings.ReplaceAll(n, "\\", "/"))
	n = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, n)
	if len(n) > 150 {
		n = n[len(n)-150:]
	}
	return n
}

type Handler struct {
	Pool       *pgxpool.Pool
	Q          *sqlc.Queries
	OrgID      uuid.UUID
	ObjectDir  string
	SigningKey []byte
}

// POST /me/documents (multipart: kind, file)
func (h *Handler) Upload(c *gin.Context) {
	p := auth.Current(c)
	kind := c.PostForm("kind")
	if !slices.Contains(Kinds, kind) {
		platform.Fail(c, platform.ErrValidation(map[string]string{"kind": "Pilih jenis dokumen."}))
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			platform.Fail(c, &platform.APIError{Status: http.StatusRequestEntityTooLarge, Code: "payload_too_large", Message: "Ukuran berkas maksimal 15 MB untuk CV dan 5 MB untuk dokumen lain."})
			return
		}
		platform.Fail(c, platform.ErrValidation(map[string]string{"file": "Pilih berkas."}))
		return
	}
	limit := int64(MaxBytesOther)
	if kind == "cv" {
		limit = MaxBytes
	}
	tooLarge := &platform.APIError{Status: http.StatusRequestEntityTooLarge, Code: "payload_too_large", Message: fmt.Sprintf("Ukuran berkas maksimal %d MB.", limit>>20)}
	if fh.Size <= 0 || fh.Size > limit {
		platform.Fail(c, tooLarge)
		return
	}
	f, err := fh.Open()
	if err != nil {
		platform.Fail(c, err)
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		platform.Fail(c, tooLarge)
		return
	}
	name := safeName(fh.Filename)
	ctype, ok := sniff(data, name)
	if !ok {
		platform.Fail(c, platform.ErrValidation(map[string]string{"file": "Format tidak didukung. Gunakan PDF, DOCX, DOC, JPG, PNG, atau WebP yang sesuai isinya."}))
		return
	}
	ctx := c.Request.Context()
	n, err := h.Q.CountActiveDocuments(ctx, p.UserID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if n >= MaxPerUser {
		platform.Fail(c, platform.ErrConflict("document_limit", "Maksimal 10 dokumen. Hapus dokumen lama terlebih dahulu."))
		return
	}
	rnd := make([]byte, 16)
	_, _ = rand.Read(rnd)
	key := filepath.ToSlash(filepath.Join(quarantineDir, hex.EncodeToString(rnd)))
	path := filepath.Join(h.ObjectDir, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		platform.Fail(c, err)
		return
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		platform.Fail(c, err)
		return
	}
	sum := sha256.Sum256(data)
	var doc sqlc.Document
	err = platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		var err error
		doc, err = q.InsertDocument(ctx, sqlc.InsertDocumentParams{OrgID: h.OrgID, OwnerUserID: p.UserID, Kind: kind, FileName: name,
			ContentType: ctype, SizeBytes: int64(len(data)), ObjectKey: key, Sha256: sum[:]})
		if err != nil {
			return err
		}
		if _, err := platform.Enqueue(ctx, q, h.OrgID, TopicScan, "document", doc.ID.String(), gin.H{"documentId": doc.ID}); err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "candidate", Action: "document.upload",
			TargetType: "document", TargetID: doc.ID.String(), RequestID: platform.RequestID(c), Detail: map[string]any{"kind": kind, "size": len(data)}})
	})
	if err != nil {
		_ = os.Remove(path)
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": doc.ID, "fileName": doc.FileName, "scanStatus": doc.ScanStatus})
}

// GET /me/documents
func (h *Handler) Mine(c *gin.Context) {
	rows, err := h.Q.ListOwnerDocuments(c.Request.Context(), auth.Current(c).UserID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

// DELETE /me/documents/:id: penghapusan lunak; berkas fisik dibersihkan sesuai kebijakan retensi.
// Pembacaan CV terkait dibatalkan (fence naik), turunannya (teks, saran, bukti, preview) dihapus.
func (h *Handler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		platform.Fail(c, platform.ErrNotFound(""))
		return
	}
	ctx := c.Request.Context()
	var n int64
	var runs []uuid.UUID
	err = platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		var err error
		if n, err = q.SoftDeleteDocument(ctx, sqlc.SoftDeleteDocumentParams{ID: id, OwnerUserID: auth.Current(c).UserID}); err != nil || n == 0 {
			return err
		}
		if runs, err = q.CancelCVRunsForDocument(ctx, id); err != nil {
			return err
		}
		for _, r := range runs {
			if err := q.DeleteCVSuggestionsForRun(ctx, r); err != nil {
				return err
			}
			if err := q.DeleteCVEvidenceForRun(ctx, r); err != nil {
				return err
			}
			if err := q.DeleteCVArtifactsForRun(ctx, r); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if n == 0 {
		platform.Fail(c, platform.ErrNotFound("Dokumen tidak ditemukan."))
		return
	}
	for _, r := range runs {
		_ = os.RemoveAll(filepath.Join(h.ObjectDir, "cvparse", r.String()))
	}
	c.Status(http.StatusNoContent)
}

// ListForCandidate dipakai detail lamaran staf.
func (h *Handler) ListForCandidate(ctx context.Context, candidate uuid.UUID) ([]sqlc.ListOwnerDocumentsRow, error) {
	return h.Q.ListOwnerDocuments(ctx, candidate)
}

// canAccess: pemilik, atau staf dengan applications.read atas salah satu lamaran pemilik.
func (h *Handler) canAccess(ctx context.Context, p *auth.Principal, d sqlc.Document) (bool, error) {
	if p.UserID == d.OwnerUserID {
		return true, nil
	}
	if !p.IsStaff() {
		return false, nil
	}
	scopes, err := h.Q.ApplicationScopesOfCandidate(ctx, d.OwnerUserID)
	if err != nil {
		return false, err
	}
	for _, s := range scopes {
		job := s.JobID
		if p.Can(auth.PermApplicationsRead, auth.Resource{Unit: s.UnitID, Job: &job}) {
			return true, nil
		}
	}
	return false, nil
}

func (h *Handler) sign(id, user uuid.UUID, exp int64) string {
	m := hmac.New(sha256.New, h.SigningKey)
	fmt.Fprintf(m, "document:%s:%s:%d", id, user, exp)
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (h *Handler) load(c *gin.Context, id uuid.UUID) (sqlc.Document, bool) {
	p := auth.Current(c)
	d, err := h.Q.GetDocument(c.Request.Context(), id)
	if platform.IsNoRows(err) || (err == nil && d.DeletedAt != nil) {
		platform.Fail(c, platform.ErrNotFound("Dokumen tidak ditemukan."))
		return d, false
	}
	if err != nil {
		platform.Fail(c, err)
		return d, false
	}
	ok, err := h.canAccess(c.Request.Context(), p, d)
	if err != nil {
		platform.Fail(c, err)
		return d, false
	}
	if !ok {
		platform.Fail(c, platform.ErrNotFound("Dokumen tidak ditemukan."))
		return d, false
	}
	return d, true
}

// POST /documents/:id/link: hanya dokumen yang sudah lolos pindai. Status pending/infected ditolak.
func (h *Handler) Link(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		platform.Fail(c, platform.ErrNotFound(""))
		return
	}
	d, ok := h.load(c, id)
	if !ok {
		return
	}
	if d.ScanStatus != "clean" {
		msg := map[string]string{"pending": "Dokumen masih menunggu pemindaian keamanan.", "infected": "Dokumen ditolak oleh pemindai keamanan.", "error": "Pemindaian gagal; hubungi admin."}[d.ScanStatus]
		platform.Fail(c, platform.ErrConflict("scan_"+d.ScanStatus, msg))
		return
	}
	p := auth.Current(c)
	exp := time.Now().Add(LinkTTL).Unix()
	c.JSON(http.StatusOK, gin.H{"url": fmt.Sprintf("/api/v1/documents/download?d=%s&exp=%d&sig=%s", id, exp, h.sign(id, p.UserID, exp))})
}

// GET /documents/download: verifikasi tanda tangan, umur, dan izin saat ini; akses staf dicatat di audit.
func (h *Handler) Download(c *gin.Context) {
	p := auth.Current(c)
	id, err := uuid.Parse(c.Query("d"))
	exp, err2 := strconv.ParseInt(c.Query("exp"), 10, 64)
	if err != nil || err2 != nil || time.Now().Unix() > exp || !hmac.Equal([]byte(c.Query("sig")), []byte(h.sign(id, p.UserID, exp))) {
		platform.Fail(c, &platform.APIError{Status: http.StatusForbidden, Code: "link_invalid", Message: "Tautan unduh tidak valid atau sudah kedaluwarsa."})
		return
	}
	d, ok := h.load(c, id)
	if !ok {
		return
	}
	if d.ScanStatus != "clean" {
		platform.Fail(c, platform.ErrConflict("scan_"+d.ScanStatus, "Dokumen belum dapat diunduh."))
		return
	}
	if p.IsStaff() {
		_ = platform.WriteAudit(c.Request.Context(), h.Q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "document.download",
			TargetType: "document", TargetID: id.String(), RequestID: platform.RequestID(c)})
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, strings.ReplaceAll(d.FileName, `"`, "")))
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Type", d.ContentType)
	c.File(filepath.Join(h.ObjectDir, filepath.FromSlash(d.ObjectKey)))
}

// ---- Worker: pemindaian ----

// Scanner mengembalikan "clean", "infected", atau error bila pemindai tidak tersedia/gagal.
type Scanner func(ctx context.Context, path string) (string, error)

// CommandScanner menjalankan perintah eksternal (mis. clamdscan --no-summary). Exit 0 = bersih, 1 = terinfeksi.
func CommandScanner(command string) Scanner {
	if strings.TrimSpace(command) == "" {
		return nil
	}
	parts := strings.Fields(command)
	return func(ctx context.Context, path string) (string, error) {
		cmd := exec.CommandContext(ctx, parts[0], append(parts[1:], path)...)
		err := cmd.Run()
		var ee *exec.ExitError
		switch {
		case err == nil:
			return "clean", nil
		case errors.As(err, &ee) && ee.ExitCode() == 1:
			return "infected", nil
		default:
			return "", fmt.Errorf("pemindai gagal: %w", err)
		}
	}
}

// Scan memproses event unggah. Tanpa pemindai, status tetap pending (dokumen tidak dapat diunduh).
func Scan(ctx context.Context, pool *pgxpool.Pool, objectDir string, scanner Scanner, payload []byte) error {
	if scanner == nil {
		return nil
	}
	var p struct {
		DocumentID uuid.UUID `json:"documentId"`
	}
	if err := jsonUnmarshal(payload, &p); err != nil {
		return err
	}
	q := sqlc.New(pool)
	d, err := q.GetDocument(ctx, p.DocumentID)
	if err != nil {
		return err
	}
	if d.ScanStatus != "pending" {
		return nil
	}
	src := filepath.Join(objectDir, filepath.FromSlash(d.ObjectKey))
	verdict, err := scanner(ctx, src)
	if err != nil {
		return err // dicoba ulang dengan backoff; setelah batas -> dead-letter, status tetap pending
	}
	key := d.ObjectKey
	if verdict == "clean" {
		key = filepath.ToSlash(filepath.Join(cleanDir, filepath.Base(d.ObjectKey)))
		dst := filepath.Join(objectDir, filepath.FromSlash(key))
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return err
		}
		if err := os.Rename(src, dst); err != nil {
			return err
		}
	}
	if err := q.SetDocumentScan(ctx, sqlc.SetDocumentScanParams{ID: d.ID, ScanStatus: verdict, ObjectKey: key}); err != nil {
		return err
	}
	// Pembacaan CV yang menunggu pemindaian dilepas ke antrean, atau gagal bila berkas terinfeksi.
	if verdict == "clean" {
		_, err = q.ReleaseWaitingCVRuns(ctx, d.ID)
	} else {
		_, err = q.FailWaitingCVRuns(ctx, sqlc.FailWaitingCVRunsParams{DocumentID: d.ID, FailureReason: ptrStr("scan_" + verdict)})
	}
	return err
}

func ptrStr(s string) *string { return &s }

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
