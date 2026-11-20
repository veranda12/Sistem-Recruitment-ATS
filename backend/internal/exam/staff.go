package exam

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"ats/db/sqlc"
	"ats/internal/platform"

	"github.com/google/uuid"
)

// GradeEssay menambah baris penilaian (append-only) lalu menghitung ulang nilai. Regrade setelah rilis
// memperbarui nilai dan audit, tetapi tidak memicu event workflow kedua.
func GradeEssay(ctx context.Context, q *sqlc.Queries, attemptID, itemID, assessor uuid.UUID, points float64, reason, requestID string) (sqlc.ExamResult, error) {
	res, err := q.LockResult(ctx, attemptID)
	if platform.IsNoRows(err) {
		return res, platform.ErrConflict("not_finalized", "Ujian belum difinalisasi.")
	}
	if err != nil {
		return res, err
	}
	items, err := q.ListAttemptItems(ctx, attemptID)
	if err != nil {
		return res, err
	}
	var item *sqlc.ListAttemptItemsRow
	for i := range items {
		if items[i].ItemID == itemID {
			item = &items[i]
		}
	}
	if item == nil || item.Kind != "essay" {
		return res, platform.ErrValidation(map[string]string{"itemId": "Butir bukan soal esai pada ujian ini."})
	}
	if points < 0 || points > item.Weight || math.IsNaN(points) {
		return res, platform.ErrValidation(map[string]string{"points": fmt.Sprintf("Nilai 0 sampai %.2f.", item.Weight)})
	}
	prev, err := q.LatestEssayGrades(ctx, attemptID)
	if err != nil {
		return res, err
	}
	regrade := false
	for _, g := range prev {
		regrade = regrade || g.ItemID == itemID
	}
	if regrade && len(reason) < 5 {
		return res, platform.ErrValidation(map[string]string{"reason": "Alasan regrade wajib diisi."})
	}
	if err := q.InsertEssayGrade(ctx, sqlc.InsertEssayGradeParams{AttemptID: attemptID, ItemID: itemID, Points: points,
		RubricVersion: item.QuestionVersionNo, AssessorID: assessor, Reason: reason}); err != nil {
		return res, err
	}
	ans, err := q.ListAnswers(ctx, attemptID)
	if err != nil {
		return res, err
	}
	byItem := map[uuid.UUID]sqlc.ListAnswersRow{}
	for _, x := range ans {
		byItem[x.ItemID] = x
	}
	sc, err := scoreAttempt(ctx, q, attemptID, items, byItem)
	if err != nil {
		return res, err
	}
	a, err := q.GetAttempt(ctx, attemptID)
	if err != nil {
		return res, err
	}
	pv, err := q.GetPackageVersion(ctx, a.PackageVersionID)
	if err != nil {
		return res, err
	}
	var passed *bool
	if sc.Score != nil {
		ok := *sc.Score >= pv.PassScore
		passed = &ok
	}
	if err := q.UpdateResultScore(ctx, sqlc.UpdateResultScoreParams{AttemptID: attemptID, Earned: sc.Earned, Score: sc.Score, EssayPending: sc.Pending, Passed: passed}); err != nil {
		return res, err
	}
	action := "exam.essay_grade"
	if regrade {
		action = "exam.essay_regrade"
	}
	if err := platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: a.OrgID, ActorID: &assessor, ActorKind: "staff", Action: action,
		TargetType: "exam_attempt", TargetID: attemptID.String(), Reason: reason, RequestID: requestID,
		Detail: map[string]any{"item_id": itemID, "points": points, "released_already": res.Status == "released"}}); err != nil {
		return res, err
	}
	return q.GetResult(ctx, attemptID)
}

// ReleaseSchedule merilis semua hasil yang belum dirilis pada satu jadwal dan menulis event outbox per lamaran.
// Event membawa fakta untuk evaluator workflow; ID event menjadi execution ID sehingga replay tidak menggandakan transisi.
func ReleaseSchedule(ctx context.Context, q *sqlc.Queries, scheduleID, actor uuid.UUID, allowed func(unit uuid.UUID, job uuid.UUID) bool, requestID string) (int, error) {
	s, err := q.GetSchedule(ctx, scheduleID)
	if err != nil {
		return 0, err
	}
	rows, err := q.ResultsForSchedule(ctx, scheduleID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		if r.Status == "released" {
			continue
		}
		app, err := q.GetApplication(ctx, r.ApplicationID)
		if err != nil {
			return n, err
		}
		if !allowed(app.UnitID, app.JobID) {
			continue // scope diperiksa per item
		}
		changed, err := q.ReleaseResult(ctx, sqlc.ReleaseResultParams{AttemptID: r.AttemptID, ReleasedBy: &actor})
		if err != nil {
			return n, err
		}
		if changed == 0 {
			continue
		}
		payload := map[string]any{"applicationId": r.ApplicationID, "attemptId": r.AttemptID, "assessmentKey": s.PackageKey,
			"score": r.Score, "passed": r.Passed, "pendingManual": r.EssayPending > 0, "candidateId": app.CandidateUserID}
		if _, err := platform.Enqueue(ctx, q, r.OrgID, TopicReleased, "exam_attempt", r.AttemptID.String(), payload); err != nil {
			return n, err
		}
		if err := platform.WriteAudit(ctx, q, platform.AuditEntry{OrgID: r.OrgID, ActorID: &actor, ActorKind: "staff", Action: "exam.result_release",
			TargetType: "exam_attempt", TargetID: r.AttemptID.String(), ScopeUnit: &app.UnitID, RequestID: requestID,
			Detail: map[string]any{"score": r.Score, "essay_pending": r.EssayPending}}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ReleasedFacts mengubah payload event rilis menjadi fakta workflow.
func ReleasedFacts(payload []byte) (app uuid.UUID, key string, facts map[string]any, err error) {
	var p struct {
		ApplicationID uuid.UUID `json:"applicationId"`
		AssessmentKey string    `json:"assessmentKey"`
		Score         *float64  `json:"score"`
		Passed        *bool     `json:"passed"`
		PendingManual bool      `json:"pendingManual"`
	}
	if err = json.Unmarshal(payload, &p); err != nil {
		return
	}
	facts = map[string]any{"assessment.status": "released", "assessment.pending_manual": p.PendingManual}
	if p.Score != nil {
		facts["assessment.score"] = *p.Score
	}
	if p.Passed != nil {
		facts["assessment.passed"] = *p.Passed
	}
	return p.ApplicationID, p.AssessmentKey, facts, nil
}
