package exam

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"slices"
	"time"

	"ats/db/sqlc"
	"ats/internal/platform"

	"github.com/google/uuid"
)

const (
	MaxBatchItems = 50
	MaxEssayChars = 10000
	TopicReleased = "exam.result_released"
	TopicInvited  = "exam.invited"
)

type option struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Value adalah jawaban peserta. Pilihan memakai ID opsi opaque per attempt; esai memakai Text.
type Value struct {
	Selected []string `json:"selected,omitempty"`
	Text     *string  `json:"text,omitempty"`
}

func (v *Value) empty() bool {
	return v == nil || (len(v.Selected) == 0 && (v.Text == nil || *v.Text == ""))
}

// ---- Start ----

// Start mengunci undangan, memeriksa kepemilikan, jendela jadwal, dan kuota attempt.
// Retry start mengembalikan attempt yang sama selama masih berjalan.
func Start(ctx context.Context, q *sqlc.Queries, participantID, candidate uuid.UUID) (sqlc.ExamAttempt, bool, error) {
	p, err := q.LockParticipant(ctx, participantID)
	if platform.IsNoRows(err) || (err == nil && p.CandidateUserID != candidate) {
		return sqlc.ExamAttempt{}, false, platform.ErrNotFound("Undangan ujian tidak ditemukan.")
	}
	if err != nil {
		return sqlc.ExamAttempt{}, false, err
	}
	if p.Status == "cancelled" {
		return sqlc.ExamAttempt{}, false, platform.ErrConflict("invitation_cancelled", "Undangan ujian ini dibatalkan.")
	}
	if last, err := q.LatestAttemptForParticipant(ctx, p.ID); err == nil {
		if last.Status == "in_progress" {
			return last, false, nil
		}
	} else if !platform.IsNoRows(err) {
		return sqlc.ExamAttempt{}, false, err
	}
	s, err := q.GetSchedule(ctx, p.ScheduleID)
	if err != nil {
		return sqlc.ExamAttempt{}, false, err
	}
	n, err := q.CountAttempts(ctx, p.ID)
	if err != nil {
		return sqlc.ExamAttempt{}, false, err
	}
	if n >= s.MaxAttempts {
		return sqlc.ExamAttempt{}, false, platform.ErrConflict("attempts_exhausted", "Kesempatan ujian Anda sudah habis.")
	}
	now := time.Now()
	if now.Before(s.OpensAt) {
		return sqlc.ExamAttempt{}, false, platform.ErrConflict("schedule_not_open", "Ujian belum dibuka.")
	}
	if !now.Before(s.ClosesAt) {
		return sqlc.ExamAttempt{}, false, platform.ErrConflict("schedule_closed", "Jadwal ujian sudah berakhir.")
	}
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return sqlc.ExamAttempt{}, false, err
	}
	// Batas sesi: durasi + akomodasi, tetapi tidak melewati penutupan jadwal (+ akomodasi).
	hard := s.ClosesAt.Add(time.Duration(p.ExtraMinutes) * time.Minute)
	a, err := q.InsertAttempt(ctx, sqlc.InsertAttemptParams{OrgID: p.OrgID, ParticipantID: p.ID, AttemptNo: n + 1, CandidateUserID: candidate,
		PackageVersionID: s.PackageVersionID, Minutes: s.DurationMinutes + p.ExtraMinutes, HardDeadline: hard, Seed: seed, AlgoVersion: AlgoVersion})
	if err != nil {
		return a, false, err
	}
	pv, err := q.GetPackageVersion(ctx, s.PackageVersionID)
	if err != nil {
		return a, false, err
	}
	qvs, err := q.ListQuestionVersions(ctx, pv.QuestionVersionIds)
	if err != nil {
		return a, false, err
	}
	byID := map[uuid.UUID]sqlc.QuestionVersion{}
	for _, v := range qvs {
		byID[v.ID] = v
	}
	order := make([]int, len(pv.QuestionVersionIds))
	for i := range order {
		order[i] = i
	}
	if pv.ShuffleItems {
		order = permutation(seed, "items", len(order))
	}
	for pos, idx := range order {
		qv, ok := byID[pv.QuestionVersionIds[idx]]
		if !ok {
			return a, false, fmt.Errorf("versi soal %s tidak ditemukan", pv.QuestionVersionIds[idx])
		}
		var opts []option
		_ = json.Unmarshal(qv.Options, &opts)
		perm := make([]int, len(opts))
		for i := range perm {
			perm[i] = i
		}
		if pv.ShuffleOptions && qv.Kind != "essay" {
			perm = permutation(seed, "opts:"+qv.ID.String(), len(opts))
		}
		optOrder := make([]string, 0, len(opts))
		optMap := map[string]string{}
		for _, oi := range perm {
			opaque := uuid.NewString()
			optOrder = append(optOrder, opaque)
			optMap[opaque] = opts[oi].ID
		}
		ob, _ := json.Marshal(optOrder)
		mb, _ := json.Marshal(optMap)
		if err := q.InsertAttemptItem(ctx, sqlc.InsertAttemptItemParams{AttemptID: a.ID, ItemID: uuid.New(), Position: int32(pos + 1),
			QuestionVersionID: qv.ID, OptionOrder: ob, OptionMap: mb}); err != nil {
			return a, false, err
		}
	}
	if err := q.SetParticipantStatus(ctx, sqlc.SetParticipantStatusParams{ID: p.ID, Status: "started"}); err != nil {
		return a, false, err
	}
	return a, true, nil
}

// ---- Resume (DTO peserta) ----

type ItemDTO struct {
	ItemID   uuid.UUID `json:"itemId"`
	Number   int32     `json:"number"`
	Kind     string    `json:"kind"`
	Stem     string    `json:"stem"`
	Options  []option  `json:"options"`
	MaxChars int       `json:"maxChars,omitempty"`
}

type AnswerDTO struct {
	ItemID    uuid.UUID `json:"itemId"`
	Value     *Value    `json:"value"`
	Revision  int32     `json:"revision"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type AttemptDTO struct {
	ID         uuid.UUID   `json:"id"`
	Status     string      `json:"status"`
	ServerTime time.Time   `json:"serverTime"`
	StartedAt  time.Time   `json:"startedAt"`
	DeadlineAt time.Time   `json:"deadlineAt"`
	Revision   int64       `json:"revision"`
	Title      string      `json:"title"`
	Items      []ItemDTO   `json:"items"`
	Answers    []AnswerDTO `json:"answers"`
	Receipt    *ReceiptDTO `json:"receipt"`
}

// Resume tidak memuat kunci jawaban, bobot, rubric, seed, atau ID opsi asli.
func Resume(ctx context.Context, q *sqlc.Queries, a sqlc.ExamAttempt) (AttemptDTO, error) {
	items, err := q.ListAttemptItems(ctx, a.ID)
	if err != nil {
		return AttemptDTO{}, err
	}
	pv, err := q.GetPackageVersion(ctx, a.PackageVersionID)
	if err != nil {
		return AttemptDTO{}, err
	}
	d := AttemptDTO{ID: a.ID, Status: a.Status, ServerTime: time.Now().UTC(), StartedAt: a.StartedAt, DeadlineAt: a.DeadlineAt,
		Revision: a.Revision, Title: pv.PackageName, Items: []ItemDTO{}, Answers: []AnswerDTO{}}
	for _, it := range items {
		var opts []option
		_ = json.Unmarshal(it.Options, &opts)
		text := map[string]string{}
		for _, o := range opts {
			text[o.ID] = o.Text
		}
		var order []string
		var omap map[string]string
		_ = json.Unmarshal(it.OptionOrder, &order)
		_ = json.Unmarshal(it.OptionMap, &omap)
		dto := ItemDTO{ItemID: it.ItemID, Number: it.Position, Kind: it.Kind, Stem: it.Stem, Options: []option{}}
		for _, opaque := range order {
			dto.Options = append(dto.Options, option{ID: opaque, Text: text[omap[opaque]]})
		}
		if it.Kind == "essay" {
			dto.MaxChars = MaxEssayChars
		}
		d.Items = append(d.Items, dto)
	}
	ans, err := q.ListAnswers(ctx, a.ID)
	if err != nil {
		return d, err
	}
	for _, x := range ans {
		var v *Value
		if len(x.Value) > 0 && string(x.Value) != "null" {
			v = &Value{}
			_ = json.Unmarshal(x.Value, v)
		}
		d.Answers = append(d.Answers, AnswerDTO{ItemID: x.ItemID, Value: v, Revision: x.Revision, UpdatedAt: x.UpdatedAt})
	}
	if a.Status != "in_progress" {
		if s, err := q.GetSubmission(ctx, a.ID); err == nil {
			r := receiptOf(s)
			d.Receipt = &r
		}
	}
	return d, nil
}

// ---- Save ----

type AnswerInput struct {
	ItemID       uuid.UUID `json:"itemId"`
	Value        *Value    `json:"value"`
	BaseRevision int32     `json:"baseRevision"`
}

type SaveRequest struct {
	OpID    string        `json:"opId"`
	Answers []AnswerInput `json:"answers"`
}

type SaveAck struct {
	Revision int64     `json:"revision"`
	SavedAt  time.Time `json:"savedAt"`
	Items    []struct {
		ItemID   uuid.UUID `json:"itemId"`
		Revision int32     `json:"revision"`
	} `json:"items"`
}

type ConflictItem struct {
	ItemID         uuid.UUID `json:"itemId"`
	ServerValue    *Value    `json:"serverValue"`
	ServerRevision int32     `json:"serverRevision"`
}

func digestOf(v any) []byte {
	b, _ := json.Marshal(v)
	d := sha256.Sum256(b)
	return d[:]
}

// Save menerapkan satu batch secara atomik di transaksi pendek. ACK hanya dikirim setelah commit.
// Receipt per opId memungkinkan retry setelah ACK hilang, bahkan setelah deadline.
func Save(ctx context.Context, q *sqlc.Queries, attemptID, candidate uuid.UUID, req SaveRequest) (json.RawMessage, error) {
	if len(req.OpID) < 8 || len(req.OpID) > 100 {
		return nil, platform.ErrValidation(map[string]string{"opId": "ID operasi tidak valid."})
	}
	if len(req.Answers) == 0 || len(req.Answers) > MaxBatchItems {
		return nil, platform.ErrValidation(map[string]string{"answers": fmt.Sprintf("Kirim 1 sampai %d jawaban per batch.", MaxBatchItems)})
	}
	a, err := q.LockAttempt(ctx, attemptID)
	if platform.IsNoRows(err) || (err == nil && a.CandidateUserID != candidate) {
		return nil, platform.ErrNotFound("Ujian tidak ditemukan.")
	}
	if err != nil {
		return nil, err
	}
	dig := digestOf(req)
	if r, err := q.GetSaveReceipt(ctx, sqlc.GetSaveReceiptParams{AttemptID: a.ID, OpID: req.OpID}); err == nil {
		if !bytes.Equal(r.PayloadDigest, dig) {
			return nil, platform.ErrConflict("idempotency_mismatch", "ID operasi sudah dipakai untuk data berbeda.")
		}
		return r.Response, nil
	} else if !platform.IsNoRows(err) {
		return nil, err
	}
	if a.Status != "in_progress" {
		return nil, platform.ErrConflict("attempt_finalized", "Ujian sudah selesai. Jawaban baru tidak diterima.")
	}
	// Deadline diperiksa dengan jam database setelah lock diperoleh.
	if !a.DbNow.Before(a.DeadlineAt) {
		return nil, platform.ErrConflict("deadline_passed", "Waktu ujian sudah habis. Jawaban terakhir yang tersimpan di server yang dinilai.")
	}
	ack, err := applyBatch(ctx, q, a.ID, req)
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(ack)
	if err := q.InsertSaveReceipt(ctx, sqlc.InsertSaveReceiptParams{AttemptID: a.ID, OpID: req.OpID, PayloadDigest: dig, Revision: ack.Revision, Response: body}); err != nil {
		return nil, err
	}
	return body, nil
}

func applyBatch(ctx context.Context, q *sqlc.Queries, attemptID uuid.UUID, req SaveRequest) (SaveAck, error) {
	items, err := q.ListAttemptItems(ctx, attemptID)
	if err != nil {
		return SaveAck{}, err
	}
	byItem := map[uuid.UUID]sqlc.ListAttemptItemsRow{}
	for _, it := range items {
		byItem[it.ItemID] = it
	}
	current, err := q.ListAnswers(ctx, attemptID)
	if err != nil {
		return SaveAck{}, err
	}
	cur := map[uuid.UUID]sqlc.ListAnswersRow{}
	for _, c := range current {
		cur[c.ItemID] = c
	}
	seen := map[uuid.UUID]bool{}
	var conflicts []ConflictItem
	for i, in := range req.Answers {
		it, ok := byItem[in.ItemID]
		if !ok || seen[in.ItemID] {
			return SaveAck{}, platform.ErrValidation(map[string]string{fmt.Sprintf("answers.%d.itemId", i): "Butir soal tidak dikenal atau dikirim ganda."})
		}
		seen[in.ItemID] = true
		if msg := validateValue(it, in.Value); msg != "" {
			return SaveAck{}, platform.ErrValidation(map[string]string{fmt.Sprintf("answers.%d.value", i): msg})
		}
		if c, has := cur[in.ItemID]; (has && c.Revision != in.BaseRevision) || (!has && in.BaseRevision != 0) {
			ci := ConflictItem{ItemID: in.ItemID}
			if has {
				ci.ServerRevision = c.Revision
				if len(c.Value) > 0 && string(c.Value) != "null" {
					ci.ServerValue = &Value{}
					_ = json.Unmarshal(c.Value, ci.ServerValue)
				}
			}
			conflicts = append(conflicts, ci)
		}
	}
	if len(conflicts) > 0 {
		// Revisi basi: tidak ada yang diterapkan; klien menyelesaikan konflik secara eksplisit.
		e := platform.ErrConflict("answer_conflict", "Jawaban sudah diubah dari tab atau perangkat lain. Pilih versi yang dipakai.")
		e.Details = map[string]any{"items": conflicts}
		return SaveAck{}, e
	}
	var ack SaveAck
	for _, in := range req.Answers {
		var vb []byte
		if !in.Value.empty() {
			vb, _ = json.Marshal(in.Value)
		}
		rev := in.BaseRevision + 1
		if err := q.UpsertAnswer(ctx, sqlc.UpsertAnswerParams{AttemptID: attemptID, ItemID: in.ItemID, Value: vb, Revision: rev, OpID: req.OpID}); err != nil {
			return ack, err
		}
		ack.Items = append(ack.Items, struct {
			ItemID   uuid.UUID `json:"itemId"`
			Revision int32     `json:"revision"`
		}{in.ItemID, rev})
	}
	ack.Revision, err = q.BumpAttemptRevision(ctx, attemptID)
	ack.SavedAt = time.Now().UTC()
	return ack, err
}

func validateValue(it sqlc.ListAttemptItemsRow, v *Value) string {
	if v.empty() {
		return ""
	}
	if it.Kind == "essay" {
		if len(v.Selected) > 0 || v.Text == nil {
			return "Jawaban esai harus berupa teks."
		}
		if len([]rune(*v.Text)) > MaxEssayChars {
			return fmt.Sprintf("Jawaban maksimal %d karakter.", MaxEssayChars)
		}
		return ""
	}
	if v.Text != nil {
		return "Jawaban pilihan tidak boleh berupa teks."
	}
	var order []string
	_ = json.Unmarshal(it.OptionOrder, &order)
	if it.Kind == "single" && len(v.Selected) > 1 {
		return "Pilih satu jawaban."
	}
	seen := map[string]bool{}
	for _, s := range v.Selected {
		if !slices.Contains(order, s) || seen[s] {
			return "Pilihan jawaban tidak dikenal."
		}
		seen[s] = true
	}
	return ""
}

// ---- Submit dan finalisasi ----

type ReceiptDTO struct {
	Code          string    `json:"code"`
	Reason        string    `json:"reason"`
	SubmittedAt   time.Time `json:"submittedAt"`
	FinalRevision int64     `json:"finalRevision"`
	AnswerDigest  string    `json:"answerDigest"`
	Answered      int       `json:"answered"`
	Total         int       `json:"total"`
}

func receiptOf(s sqlc.ExamSubmission) ReceiptDTO {
	var snap struct {
		Items []struct {
			Value json.RawMessage `json:"value"`
		} `json:"items"`
	}
	_ = json.Unmarshal(s.Snapshot, &snap)
	answered := 0
	for _, it := range snap.Items {
		if len(it.Value) > 0 && string(it.Value) != "null" {
			answered++
		}
	}
	return ReceiptDTO{Code: s.ReceiptCode, Reason: s.Reason, SubmittedAt: s.SubmittedAt, FinalRevision: s.FinalRevision,
		AnswerDigest: fmt.Sprintf("%x", s.AnswersDigest[:8]), Answered: answered, Total: len(snap.Items)}
}

type SubmitRequest struct {
	ExpectedRevision int64         `json:"expectedRevision"`
	OpID             string        `json:"opId"`
	Answers          []AnswerInput `json:"answers"`
}

// Submit: satu attempt maksimal satu submission. Retry identik mengembalikan receipt yang sama.
// Batch terakhir (opsional) diterapkan, dibekukan, dan receipt dibuat dalam satu transaksi.
func Submit(ctx context.Context, q *sqlc.Queries, attemptID, candidate uuid.UUID, key string, req SubmitRequest) (ReceiptDTO, error) {
	if len(key) < 8 || len(key) > 128 {
		return ReceiptDTO{}, platform.ErrBadRequest("Idempotency-Key wajib 8 sampai 128 karakter.")
	}
	a, err := q.LockAttempt(ctx, attemptID)
	if platform.IsNoRows(err) || (err == nil && a.CandidateUserID != candidate) {
		return ReceiptDTO{}, platform.ErrNotFound("Ujian tidak ditemukan.")
	}
	if err != nil {
		return ReceiptDTO{}, err
	}
	if s, err := q.GetSubmission(ctx, a.ID); err == nil {
		r := receiptOf(s)
		if s.IdempotencyKey != nil && *s.IdempotencyKey == key {
			return r, nil
		}
		e := platform.ErrConflict("attempt_finalized", "Ujian sudah difinalisasi sebelumnya.")
		e.Details = map[string]any{"receipt": r}
		return ReceiptDTO{}, e
	} else if !platform.IsNoRows(err) {
		return ReceiptDTO{}, err
	}
	if !a.DbNow.Before(a.DeadlineAt) {
		// Lewat deadline tanpa grace period: finalisasi dengan jawaban terakhir yang sudah committed.
		// Receipt ber-reason "expired" harus di-commit; handler menjawab 409 deadline_passed setelah commit.
		return finalize(ctx, q, a.ID, "expired", nil)
	}
	if req.ExpectedRevision != a.Revision {
		e := platform.ErrConflict("stale_revision", "Ada jawaban yang belum sinkron. Muat ulang ujian lalu kirim kembali.")
		e.Details = map[string]any{"revision": a.Revision}
		return ReceiptDTO{}, e
	}
	if len(req.Answers) > 0 {
		if _, err := Save(ctx, q, a.ID, candidate, SaveRequest{OpID: req.OpID, Answers: req.Answers}); err != nil {
			return ReceiptDTO{}, err
		}
	}
	return finalize(ctx, q, a.ID, "submitted", &key)
}

// finalize membekukan snapshot jawaban committed, membuat satu receipt, dan menghitung nilai objektif.
// Caller sudah memegang lock attempt.
func finalize(ctx context.Context, q *sqlc.Queries, attemptID uuid.UUID, reason string, key *string) (ReceiptDTO, error) {
	a, err := q.GetAttempt(ctx, attemptID)
	if err != nil {
		return ReceiptDTO{}, err
	}
	items, err := q.ListAttemptItems(ctx, a.ID)
	if err != nil {
		return ReceiptDTO{}, err
	}
	ans, err := q.ListAnswers(ctx, a.ID)
	if err != nil {
		return ReceiptDTO{}, err
	}
	byItem := map[uuid.UUID]sqlc.ListAnswersRow{}
	for _, x := range ans {
		byItem[x.ItemID] = x
	}
	type snapItem struct {
		ItemID            uuid.UUID       `json:"itemId"`
		Number            int32           `json:"number"`
		QuestionVersionID uuid.UUID       `json:"questionVersionId"`
		Value             json.RawMessage `json:"value"`
		Revision          int32           `json:"revision"`
	}
	snap := struct {
		AttemptID     uuid.UUID  `json:"attemptId"`
		PackageVer    uuid.UUID  `json:"packageVersionId"`
		AlgoVersion   int32      `json:"algoVersion"`
		FinalRevision int64      `json:"finalRevision"`
		Items         []snapItem `json:"items"`
	}{AttemptID: a.ID, PackageVer: a.PackageVersionID, AlgoVersion: a.AlgoVersion, FinalRevision: a.Revision}
	for _, it := range items {
		si := snapItem{ItemID: it.ItemID, Number: it.Position, QuestionVersionID: it.QuestionVersionID, Value: json.RawMessage("null")}
		if x, ok := byItem[it.ItemID]; ok && len(x.Value) > 0 {
			si.Value, si.Revision = json.RawMessage(x.Value), x.Revision
		}
		snap.Items = append(snap.Items, si)
	}
	sb, _ := json.Marshal(snap)
	dig := sha256.Sum256(sb)
	code := make([]byte, 8)
	_, _ = rand.Read(code)
	s, err := q.InsertSubmission(ctx, sqlc.InsertSubmissionParams{AttemptID: a.ID, IdempotencyKey: key, Reason: reason,
		ReceiptCode: "UJ-" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(code), AnswersDigest: dig[:], Snapshot: sb, FinalRevision: a.Revision})
	if err != nil {
		return ReceiptDTO{}, err
	}
	status := "submitted"
	if reason == "expired" {
		status = "expired"
	}
	if err := q.FinalizeAttempt(ctx, sqlc.FinalizeAttemptParams{ID: a.ID, Status: status}); err != nil {
		return ReceiptDTO{}, err
	}
	if err := q.SetParticipantStatus(ctx, sqlc.SetParticipantStatusParams{ID: a.ParticipantID, Status: status}); err != nil {
		return ReceiptDTO{}, err
	}
	p, err := q.GetParticipant(ctx, a.ParticipantID)
	if err != nil {
		return ReceiptDTO{}, err
	}
	sc, err := scoreAttempt(ctx, q, a.ID, items, byItem)
	if err != nil {
		return ReceiptDTO{}, err
	}
	pv, err := q.GetPackageVersion(ctx, a.PackageVersionID)
	if err != nil {
		return ReceiptDTO{}, err
	}
	resStatus := "pending"
	var passed *bool
	if sc.Score != nil {
		resStatus = "graded"
		ok := *sc.Score >= pv.PassScore
		passed = &ok
	}
	if err := q.InsertResult(ctx, sqlc.InsertResultParams{AttemptID: a.ID, OrgID: a.OrgID, ApplicationID: p.ApplicationID, Earned: sc.Earned,
		MaxPoints: sc.Max, Score: sc.Score, EssayPending: sc.Pending, Passed: passed, Status: resStatus}); err != nil {
		return ReceiptDTO{}, err
	}
	actor := &a.CandidateUserID
	kind := "candidate"
	if reason == "expired" {
		actor, kind = nil, "system"
	}
	if err := platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: a.OrgID, ActorID: actor, ActorKind: kind, Action: "exam." + reason,
		TargetType: "exam_attempt", TargetID: a.ID.String(), Detail: map[string]any{"receipt": s.ReceiptCode, "final_revision": a.Revision}}); err != nil {
		return ReceiptDTO{}, err
	}
	return receiptOf(s), nil
}

type scoreResult struct {
	Earned  float64
	Max     float64
	Score   *float64
	Pending int32
}

// scoreAttempt: pilihan tunggal/jamak benar hanya bila himpunan pilihan sama persis dengan kunci;
// kosong = 0; esai memakai nilai penilai terbaru, tanpa nilai = pending.
func scoreAttempt(ctx context.Context, q *sqlc.Queries, attemptID uuid.UUID, items []sqlc.ListAttemptItemsRow, answers map[uuid.UUID]sqlc.ListAnswersRow) (scoreResult, error) {
	grades, err := q.LatestEssayGrades(ctx, attemptID)
	if err != nil {
		return scoreResult{}, err
	}
	essay := map[uuid.UUID]float64{}
	for _, g := range grades {
		essay[g.ItemID] = g.Points
	}
	var r scoreResult
	for _, it := range items {
		r.Max += it.Weight
		x, has := answers[it.ItemID]
		var v Value
		if has && len(x.Value) > 0 {
			_ = json.Unmarshal(x.Value, &v)
		}
		if it.Kind == "essay" {
			if pts, ok := essay[it.ItemID]; ok {
				r.Earned += pts
			} else if !v.empty() {
				r.Pending++
			}
			continue
		}
		if v.empty() {
			continue
		}
		var omap map[string]string
		_ = json.Unmarshal(it.OptionMap, &omap)
		var key []string
		_ = json.Unmarshal(it.AnswerKey, &key)
		got := make([]string, 0, len(v.Selected))
		for _, s := range v.Selected {
			got = append(got, omap[s])
		}
		slices.Sort(got)
		slices.Sort(key)
		if slices.Equal(got, key) {
			r.Earned += it.Weight
		}
	}
	if r.Pending == 0 {
		s := math.Round(r.Earned/r.Max*10000) / 100
		r.Score = &s
	}
	return r, nil
}

// ExpireOne memfinalisasi satu attempt yang lewat deadline. Aman dipanggil bersamaan dengan submit:
// lock attempt yang sama memastikan hanya satu finalisasi.
func ExpireOne(ctx context.Context, q *sqlc.Queries, attemptID uuid.UUID) (bool, error) {
	a, err := q.LockAttempt(ctx, attemptID)
	if err != nil {
		return false, err
	}
	if a.Status != "in_progress" || a.DbNow.Before(a.DeadlineAt) {
		return false, nil
	}
	_, err = finalize(ctx, q, a.ID, "expired", nil)
	return err == nil, err
}

var errForbidden = &platform.APIError{Status: http.StatusForbidden, Code: "forbidden", Message: "Anda tidak memiliki akses untuk tindakan ini."}
