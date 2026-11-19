package platform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// APIError adalah bentuk error stabil untuk klien: code mesin, pesan UI berbahasa Indonesia, fieldErrors.
type APIError struct {
	Status      int               `json:"-"`
	Code        string            `json:"code"`
	Message     string            `json:"message"`
	FieldErrors map[string]string `json:"fieldErrors,omitempty"`
	RetryAfter  int               `json:"-"`
	Details     any               `json:"details,omitempty"`
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

func ErrValidation(fields map[string]string) *APIError {
	return &APIError{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: "Periksa kembali isian yang ditandai.", FieldErrors: fields}
}
func ErrBadRequest(msg string) *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: "bad_request", Message: msg}
}
func ErrUnauthenticated() *APIError {
	return &APIError{Status: http.StatusUnauthorized, Code: "unauthenticated", Message: "Silakan masuk terlebih dahulu."}
}
func ErrForbidden() *APIError {
	return &APIError{Status: http.StatusForbidden, Code: "forbidden", Message: "Anda tidak memiliki akses untuk tindakan ini."}
}
func ErrNotFound(msg string) *APIError {
	if msg == "" {
		msg = "Data tidak ditemukan."
	}
	return &APIError{Status: http.StatusNotFound, Code: "not_found", Message: msg}
}
func ErrConflict(code, msg string) *APIError {
	return &APIError{Status: http.StatusConflict, Code: code, Message: msg}
}
func ErrRateLimited(retryAfter int) *APIError {
	return &APIError{Status: http.StatusTooManyRequests, Code: "rate_limited", Message: "Terlalu banyak permintaan. Coba lagi sebentar lagi.", RetryAfter: retryAfter}
}
func ErrUnavailable(msg string, retryAfter int) *APIError {
	return &APIError{Status: http.StatusServiceUnavailable, Code: "unavailable", Message: msg, RetryAfter: retryAfter}
}

const requestIDKey = "request_id"

func RequestID(c *gin.Context) string { return c.GetString(requestIDKey) }

// Fail menulis error terstruktur. Error non-APIError dicatat lengkap dan dikembalikan sebagai 500 generik.
func Fail(c *gin.Context, err error) {
	var ae *APIError
	if !errors.As(err, &ae) {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			ae = ErrUnavailable("Server sedang sibuk. Coba lagi.", 2)
		} else {
			slog.Error("internal error", "request_id", RequestID(c), "path", c.FullPath(), "err", err.Error())
			ae = &APIError{Status: http.StatusInternalServerError, Code: "internal", Message: "Terjadi kesalahan pada server. Coba lagi."}
		}
	}
	if ae.RetryAfter > 0 {
		c.Header("Retry-After", strconv.Itoa(ae.RetryAfter))
	}
	c.AbortWithStatusJSON(ae.Status, gin.H{"error": ae, "requestId": RequestID(c)})
}


// BindJSON membaca body JSON dengan pesan error yang aman.
func BindJSON(c *gin.Context, dst any) error {
	if err := c.ShouldBindJSON(dst); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return &APIError{Status: http.StatusRequestEntityTooLarge, Code: "payload_too_large", Message: "Data yang dikirim terlalu besar."}
		}
		return ErrBadRequest("Format data tidak valid.")
	}
	return nil
}

// Middleware

func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if len(id) < 8 || len(id) > 64 {
			b := make([]byte, 12)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
		}
		c.Set(requestIDKey, id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

func RecoverMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic", "request_id", RequestID(c), "panic", r, "stack", string(debug.Stack()))
				Fail(c, errors.New("panic"))
			}
		}()
		c.Next()
	}
}

// AccessLog mencatat route template (bukan URL mentah) agar token di query string tidak ikut terekam.
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		slog.Info("http", "request_id", RequestID(c), "method", c.Request.Method, "route", route,
			"status", c.Writer.Status(), "duration_ms", time.Since(start).Milliseconds())
	}
}

func BodyLimit(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > n {
			Fail(c, &APIError{Status: http.StatusRequestEntityTooLarge, Code: "payload_too_large", Message: "Data yang dikirim terlalu besar."})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		c.Next()
	}
}

// Deadline memasang batas waktu per kelas operasi pada context request.
func Deadline(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func SecurityHeaders(production bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		if production {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}
