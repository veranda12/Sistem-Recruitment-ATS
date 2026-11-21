package exam

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"ats/db/sqlc"
	"ats/internal/auth"
	"ats/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const MaxImportRows = 500

// ImportRow adalah satu baris CSV impor soal yang sudah divalidasi.
// Format (pemisah ';'): jenis;pertanyaan;opsi (dipisah |);kunci (huruf, dipisah koma);bobot;rubrik
type ImportRow struct {
	Line    int      `json:"line"`
	Kind    string   `json:"kind"`
	Stem    string   `json:"stem"`
	Options []string `json:"options"`
	Key     []string `json:"key"`
	Weight  float64  `json:"weight"`
	Rubric  string   `json:"rubric"`
	Errors  []string `json:"errors"`
}

var kindAlias = map[string]string{"tunggal": "single", "single": "single", "jamak": "multiple", "multiple": "multiple", "esai": "essay", "essay": "essay"}

// ParseImport memvalidasi seluruh baris tanpa menulis apa pun. Digest dipakai untuk memastikan
// impor yang dikonfirmasi sama persis dengan yang dipratinjau.
func ParseImport(content string) ([]ImportRow, string, error) {
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(content, string(rune(0xFEFF)))))
	r.Comma = ';'
	r.FieldsPerRecord = -1
	r.LazyQuotes = false
	var rows []ImportRow
	line := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		line++
		if err != nil {
			return nil, "", platform.ErrValidation(map[string]string{"content": fmt.Sprintf("Baris %d tidak dapat dibaca: %v", line, err)})
		}
		if line == 1 && len(rec) > 0 && strings.EqualFold(strings.TrimSpace(rec[0]), "jenis") {
			continue // baris judul
		}
		if len(rec) == 1 && strings.TrimSpace(rec[0]) == "" {
			continue
		}
		if len(rows) >= MaxImportRows {
			return nil, "", platform.ErrValidation(map[string]string{"content": fmt.Sprintf("Maksimal %d soal per impor.", MaxImportRows)})
		}
		row := ImportRow{Line: line, Errors: []string{}}
		get := func(i int) string {
			if i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		k, ok := kindAlias[strings.ToLower(get(0))]
		if !ok {
			row.Errors = append(row.Errors, "Jenis harus tunggal, jamak, atau esai.")
		}
		row.Kind = k
		row.Stem = get(1)
		if row.Stem == "" || len(row.Stem) > 10000 {
			row.Errors = append(row.Errors, "Pertanyaan wajib diisi (maksimal 10000 karakter).")
		}
		if get(2) != "" {
			for _, o := range strings.Split(get(2), "|") {
				row.Options = append(row.Options, strings.TrimSpace(o))
			}
		}
		if get(3) != "" {
			for _, kk := range strings.Split(get(3), ",") {
				row.Key = append(row.Key, strings.ToUpper(strings.TrimSpace(kk)))
			}
		}
		row.Weight = 1
		if w := get(4); w != "" {
			f, err := strconv.ParseFloat(strings.ReplaceAll(w, ",", "."), 64)
			if err != nil || f <= 0 || f > 100 {
				row.Errors = append(row.Errors, "Bobot harus angka lebih dari 0 sampai 100.")
			}
			row.Weight = f
		}
		row.Rubric = get(5)
		switch row.Kind {
		case "single", "multiple":
			if len(row.Options) < 2 || len(row.Options) > 10 || slices.Contains(row.Options, "") {
				row.Errors = append(row.Errors, "Pilihan jawaban 2 sampai 10, tidak boleh kosong.")
			}
			if len(row.Key) == 0 || (row.Kind == "single" && len(row.Key) != 1) {
				row.Errors = append(row.Errors, "Kunci: tepat satu huruf untuk pilihan tunggal, minimal satu untuk pilihan jamak.")
			}
			seen := map[string]bool{}
			for _, kk := range row.Key {
				idx := int(kk[0] - 'A')
				if len(kk) != 1 || idx < 0 || idx >= len(row.Options) || seen[kk] {
					row.Errors = append(row.Errors, "Kunci "+kk+" tidak sesuai daftar pilihan.")
				}
				seen[kk] = true
			}
		case "essay":
			if len(row.Options) > 0 || len(row.Key) > 0 {
				row.Errors = append(row.Errors, "Soal esai tidak memiliki pilihan atau kunci.")
			}
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, "", platform.ErrValidation(map[string]string{"content": "Tidak ada soal pada berkas."})
	}
	b, _ := json.Marshal(rows)
	sum := sha256.Sum256(b)
	return rows, hex.EncodeToString(sum[:]), nil
}

func optionPayload(row ImportRow) ([]byte, []byte) {
	opts := []map[string]string{}
	for i, o := range row.Options {
		opts = append(opts, map[string]string{"id": string(rune('a' + i)), "text": o})
	}
	key := []string{}
	for _, k := range row.Key {
		key = append(key, strings.ToLower(k))
	}
	ob, _ := json.Marshal(opts)
	kb, _ := json.Marshal(key)
	return ob, kb
}

// GET /exam-banks
func (h *Handler) ListBanks(c *gin.Context) {
	ctx := c.Request.Context()
	banks, err := h.Q.ListQuestionBanks(ctx, h.OrgID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	pkgs, err := h.Q.ListPackages(ctx, h.OrgID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"banks": banks, "packages": pkgs})
}

// POST /exam-banks {name, category}
func (h *Handler) CreateBank(c *gin.Context) {
	p := auth.Current(c)
	var req struct {
		Name     string `json:"name"`
		Category string `json:"category"`
	}
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	if l := len(strings.TrimSpace(req.Name)); l < 2 || l > 200 {
		platform.Fail(c, platform.ErrValidation(map[string]string{"name": "Nama bank 2 sampai 200 karakter."}))
		return
	}
	b, err := h.Q.CreateQuestionBank(c.Request.Context(), sqlc.CreateQuestionBankParams{OrgID: h.OrgID, Name: strings.TrimSpace(req.Name), Category: strings.TrimSpace(req.Category), CreatedBy: &p.UserID})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, b)
}

func (h *Handler) bank(c *gin.Context) (sqlc.QuestionBank, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		platform.Fail(c, platform.ErrNotFound(""))
		return sqlc.QuestionBank{}, false
	}
	b, err := h.Q.GetQuestionBank(c.Request.Context(), id)
	if platform.IsNoRows(err) || (err == nil && b.OrgID != h.OrgID) {
		platform.Fail(c, platform.ErrNotFound("Bank soal tidak ditemukan."))
		return b, false
	}
	if err != nil {
		platform.Fail(c, err)
		return b, false
	}
	return b, true
}

// GET /exam-banks/:id/questions: tampilan examiner (termasuk kunci). Tidak dipakai endpoint peserta.
func (h *Handler) BankQuestions(c *gin.Context) {
	b, ok := h.bank(c)
	if !ok {
		return
	}
	rows, err := h.Q.ListBankLatestVersions(c.Request.Context(), b.ID)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	type q struct {
		sqlc.ListBankLatestVersionsRow
		Key json.RawMessage `json:"answerKey"`
	}
	out := []q{}
	for _, r := range rows {
		out = append(out, q{r, json.RawMessage(r.AnswerKey)})
	}
	noStore(c)
	c.JSON(http.StatusOK, gin.H{"bank": b, "questions": out})
}

type importReq struct {
	Content string `json:"content"`
	Digest  string `json:"digest"`
}

// POST /exam-banks/:id/import/preview: validasi tanpa menulis.
func (h *Handler) ImportPreview(c *gin.Context) {
	if _, ok := h.bank(c); !ok {
		return
	}
	var req importReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	rows, digest, err := ParseImport(req.Content)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	bad := 0
	for _, r := range rows {
		if len(r.Errors) > 0 {
			bad++
		}
	}
	c.JSON(http.StatusOK, gin.H{"rows": rows, "digest": digest, "valid": bad == 0, "invalidCount": bad})
}

// POST /exam-banks/:id/import: hanya bila digest sama dengan preview dan semua baris valid; atomik (semua atau tidak sama sekali).
func (h *Handler) Import(c *gin.Context) {
	b, ok := h.bank(c)
	if !ok {
		return
	}
	p := auth.Current(c)
	var req importReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	rows, digest, err := ParseImport(req.Content)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if digest != req.Digest {
		platform.Fail(c, platform.ErrConflict("preview_mismatch", "Isi impor berbeda dari yang dipratinjau. Ulangi pratinjau."))
		return
	}
	for _, r := range rows {
		if len(r.Errors) > 0 {
			platform.Fail(c, platform.ErrConflict("import_invalid", "Masih ada baris yang tidak valid."))
			return
		}
	}
	ctx := c.Request.Context()
	err = platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		for _, r := range rows {
			qu, err := q.CreateQuestion(ctx, sqlc.CreateQuestionParams{OrgID: h.OrgID, BankID: b.ID})
			if err != nil {
				return err
			}
			ob, kb := optionPayload(r)
			if _, err := q.InsertQuestionVersion(ctx, sqlc.InsertQuestionVersionParams{QuestionID: qu.ID, VersionNo: 1, Kind: r.Kind, Stem: r.Stem,
				Options: ob, AnswerKey: kb, Weight: r.Weight, Rubric: r.Rubric, CreatedBy: &p.UserID}); err != nil {
				return err
			}
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "exam.bank_import",
			TargetType: "question_bank", TargetID: b.ID.String(), RequestID: platform.RequestID(c), Detail: map[string]any{"rows": len(rows), "digest": digest[:16]}})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"imported": len(rows)})
}

// POST /questions/:id/versions: perubahan soal selalu membuat versi baru; attempt berjalan tetap memakai versi lamanya.
func (h *Handler) NewQuestionVersion(c *gin.Context) {
	p := auth.Current(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		platform.Fail(c, platform.ErrNotFound(""))
		return
	}
	var req struct {
		Line string `json:"row"` // satu baris CSV format impor
	}
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	rows, _, err := ParseImport(req.Line)
	if err != nil {
		platform.Fail(c, err)
		return
	}
	if len(rows) != 1 || len(rows[0].Errors) > 0 {
		e := platform.ErrValidation(map[string]string{"row": "Baris soal tidak valid."})
		e.Details = rows
		platform.Fail(c, e)
		return
	}
	ctx := c.Request.Context()
	var v sqlc.QuestionVersion
	err = platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		qu, err := q.LockQuestion(ctx, id)
		if platform.IsNoRows(err) || (err == nil && qu.OrgID != h.OrgID) {
			return platform.ErrNotFound("Soal tidak ditemukan.")
		}
		if err != nil {
			return err
		}
		no, err := q.NextQuestionVersionNo(ctx, id)
		if err != nil {
			return err
		}
		ob, kb := optionPayload(rows[0])
		v, err = q.InsertQuestionVersion(ctx, sqlc.InsertQuestionVersionParams{QuestionID: id, VersionNo: no, Kind: rows[0].Kind, Stem: rows[0].Stem,
			Options: ob, AnswerKey: kb, Weight: rows[0].Weight, Rubric: rows[0].Rubric, CreatedBy: &p.UserID})
		if err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "exam.question_version",
			TargetType: "question", TargetID: id.String(), RequestID: platform.RequestID(c), Detail: map[string]any{"version": no}})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, v)
}

type packageVersionReq struct {
	Name               string      `json:"name"`
	QuestionVersionIDs []uuid.UUID `json:"questionVersionIds"`
	DurationMinutes    int32       `json:"durationMinutes"`
	PassScore          float64     `json:"passScore"`
	ShuffleItems       bool        `json:"shuffleItems"`
	ShuffleOptions     bool        `json:"shuffleOptions"`
}

// POST /exam-packages/:key/versions: menerbitkan versi paket immutable (membuat paket bila belum ada).
func (h *Handler) PublishPackageVersion(c *gin.Context) {
	p := auth.Current(c)
	key := c.Param("key")
	var req packageVersionReq
	if err := platform.BindJSON(c, &req); err != nil {
		platform.Fail(c, err)
		return
	}
	f := map[string]string{}
	if len(req.QuestionVersionIDs) == 0 || len(req.QuestionVersionIDs) > 200 {
		f["questionVersionIds"] = "Pilih 1 sampai 200 soal."
	}
	if req.DurationMinutes < 1 || req.DurationMinutes > 600 {
		f["durationMinutes"] = "Durasi 1 sampai 600 menit."
	}
	if req.PassScore < 0 || req.PassScore > 100 {
		f["passScore"] = "Ambang lulus 0 sampai 100."
	}
	if len(f) > 0 {
		platform.Fail(c, platform.ErrValidation(f))
		return
	}
	ctx := c.Request.Context()
	var v sqlc.ExamPackageVersion
	err := platform.WithTx(ctx, h.Pool, func(tx pgx.Tx) error {
		q := h.Q.WithTx(tx)
		n, err := q.CountQuestionVersionsInOrg(ctx, sqlc.CountQuestionVersionsInOrgParams{Ids: req.QuestionVersionIDs, OrgID: h.OrgID})
		if err != nil {
			return err
		}
		if int(n) != len(req.QuestionVersionIDs) {
			return platform.ErrValidation(map[string]string{"questionVersionIds": "Ada versi soal yang tidak dikenal atau dipilih ganda."})
		}
		pkg, err := q.GetExamPackageByKey(ctx, sqlc.GetExamPackageByKeyParams{OrgID: h.OrgID, Key: key})
		if platform.IsNoRows(err) {
			if len(strings.TrimSpace(req.Name)) < 2 {
				return platform.ErrValidation(map[string]string{"name": "Nama paket wajib untuk paket baru."})
			}
			pkg, err = q.CreateExamPackage(ctx, sqlc.CreateExamPackageParams{OrgID: h.OrgID, Key: key, Name: strings.TrimSpace(req.Name)})
			if err != nil {
				return platform.ErrValidation(map[string]string{"key": "Kunci paket hanya huruf kecil, angka, dan tanda hubung."})
			}
		} else if err != nil {
			return err
		}
		if _, err := q.LockPackage(ctx, pkg.ID); err != nil {
			return err
		}
		no, err := q.NextPackageVersionNo(ctx, pkg.ID)
		if err != nil {
			return err
		}
		v, err = q.InsertPackageVersion(ctx, sqlc.InsertPackageVersionParams{OrgID: h.OrgID, PackageID: pkg.ID, VersionNo: no, QuestionVersionIds: req.QuestionVersionIDs,
			DurationMinutes: req.DurationMinutes, PassScore: req.PassScore, ShuffleItems: req.ShuffleItems, ShuffleOptions: req.ShuffleOptions, PublishedBy: &p.UserID})
		if err != nil {
			return err
		}
		return platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: h.OrgID, ActorID: &p.UserID, ActorKind: "staff", Action: "exam.package_publish",
			TargetType: "exam_package", TargetID: pkg.ID.String(), RequestID: platform.RequestID(c), Detail: map[string]any{"version": no, "items": len(req.QuestionVersionIDs)}})
	})
	if err != nil {
		platform.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": v.ID, "versionNo": v.VersionNo})
}
