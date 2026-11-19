package app

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"ats/db/sqlc"
	"ats/internal/platform"
	"ats/internal/workflow"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (e *env) exec(cmd workflow.Command) (workflow.Result, error) {
	var res workflow.Result
	err := platform.WithTx(context.Background(), e.pool, func(tx pgx.Tx) error {
		var err error
		res, err = workflow.Execute(context.Background(), sqlc.New(tx), cmd)
		return err
	})
	return res, err
}

func apiCode(err error) string {
	var ae *platform.APIError
	if errors.As(err, &ae) {
		return ae.Code
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

func (e *env) userID(email string) uuid.UUID {
	return e.scalar(`SELECT id FROM users WHERE email = $1`, email).([16]byte)
}

// appAt membuat lamaran baru (via API) dan memindahkannya ke tahap ujian.
func (e *env) appAtExam(email, nik string) uuid.UUID {
	e.t.Helper()
	c := e.registerVerified(email, "Peserta Uji", nik)
	r := c.do("POST", "/jobs/operator-produksi-karawang/apply", nil)
	expect(e.t, r, 201, "apply")
	id := uuid.MustParse(idOf(r, "applicationId"))
	rec := e.userID("rekruter.ops@arunika.example")
	if _, err := e.exec(workflow.Command{ExecutionID: uuid.New(), ApplicationID: id, Kind: "manual", ToStage: "ujian",
		Actor: workflow.Actor{UserID: &rec, Kind: "staff"}}); err != nil {
		e.t.Fatal(err)
	}
	return id
}

func TestWorkflowEventBranchesOnRealDB(t *testing.T) {
	e := newEnv(t)
	cases := []struct {
		name  string
		facts workflow.Facts
		key   string
		want  string
		stat  string
	}{
		{"lulus", workflow.Facts{"assessment.status": "released", "assessment.score": 82.5}, "tes-dasar", "interview", "active"},
		{"di_bawah_ambang", workflow.Facts{"assessment.status": "released", "assessment.score": 60.0}, "tes-dasar", "ditolak", "rejected"},
		{"penilaian_manual", workflow.Facts{"assessment.status": "released", "assessment.pending_manual": true}, "tes-dasar", "review", "active"},
		{"paket_lain_diabaikan", workflow.Facts{"assessment.status": "released", "assessment.score": 99.0}, "paket-lain", "ujian", "active"},
	}
	for i, c := range cases {
		id := e.appAtExam("cabang"+c.name+"@contoh.test", "320101450799"+[]string{"0101", "0102", "0103", "0104"}[i])
		exe := uuid.New()
		res, err := e.exec(workflow.Command{ExecutionID: exe, ApplicationID: id, Kind: "event", Event: "assessment.released",
			AssessmentKey: c.key, Facts: c.facts, Actor: workflow.Actor{Kind: "system"}})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if res.ToStage != c.want || res.Status != c.stat {
			t.Fatalf("%s: tahap %s/%s, ingin %s/%s", c.name, res.ToStage, res.Status, c.want, c.stat)
		}
		// Event duplikat (execution ID sama) tidak membuat transisi kedua.
		dup, err := e.exec(workflow.Command{ExecutionID: exe, ApplicationID: id, Kind: "event", Event: "assessment.released",
			AssessmentKey: c.key, Facts: c.facts, Actor: workflow.Actor{Kind: "system"}})
		if err != nil || (res.Changed && !dup.Duplicate) {
			t.Fatalf("%s: event duplikat harus no-op: %+v %v", c.name, dup, err)
		}
		if res.Changed {
			var rr json.RawMessage
			if err := e.pool.QueryRow(context.Background(), `SELECT rule_ref FROM transition_history WHERE execution_id = $1`, exe).Scan(&rr); err != nil {
				t.Fatal(err)
			}
			var m map[string]any
			_ = json.Unmarshal(rr, &m)
			if m["edgeId"] == nil || m["workflowVersionId"] == nil {
				t.Fatalf("%s: histori harus menjelaskan rule dan versi: %s", c.name, rr)
			}
		}
	}
}

func TestApprovalGuardsOnRealDB(t *testing.T) {
	e := newEnv(t)
	hm := e.userID("hm.ops@arunika.example")
	rec := e.userID("rekruter.ops@arunika.example")
	toApproval := func(id uuid.UUID, actor uuid.UUID) {
		for _, to := range []string{"interview", "persetujuan"} {
			if to == "interview" {
				if _, err := e.exec(workflow.Command{ExecutionID: uuid.New(), ApplicationID: id, Kind: "event", Event: "assessment.released",
					AssessmentKey: "tes-dasar", Facts: workflow.Facts{"assessment.status": "released", "assessment.score": 90.0}, Actor: workflow.Actor{Kind: "system"}}); err != nil {
					t.Fatal(err)
				}
				continue
			}
			if _, err := e.exec(workflow.Command{ExecutionID: uuid.New(), ApplicationID: id, Kind: "manual", ToStage: to, Actor: workflow.Actor{UserID: &actor, Kind: "staff"}}); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Self-approval ditolak: penyetuju adalah orang yang memindahkan ke tahap persetujuan.
	a := e.appAtExam("self@contoh.test", "3201014507990201")
	toApproval(a, hm)
	_, err := e.exec(workflow.Command{ExecutionID: uuid.New(), ApplicationID: a, Kind: "approval", Decision: "approve",
		Actor: workflow.Actor{UserID: &hm, Kind: "staff", Roles: []string{"hiring_manager"}}})
	if apiCode(err) != "self_approval" {
		t.Fatalf("self approval harus ditolak: %v", err)
	}
	// Peran bukan penyetuju ditolak.
	_, err = e.exec(workflow.Command{ExecutionID: uuid.New(), ApplicationID: a, Kind: "approval", Decision: "approve",
		Actor: workflow.Actor{UserID: &rec, Kind: "staff", Roles: []string{"recruiter"}}})
	if apiCode(err) != "approval_not_permitted" {
		t.Fatalf("recruiter bukan penyetuju: %v", err)
	}

	// Dua penyetuju serentak dengan kuorum 1: tepat satu transisi ke tahap akhir.
	b := e.appAtExam("serentak@contoh.test", "3201014507990202")
	toApproval(b, rec)
	other := e.userID("hr.ops@arunika.example")
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, u := range []uuid.UUID{hm, other} {
		wg.Add(1)
		go func(i int, u uuid.UUID) {
			defer wg.Done()
			_, errs[i] = e.exec(workflow.Command{ExecutionID: uuid.New(), ApplicationID: b, Kind: "approval", Decision: "approve",
				Actor: workflow.Actor{UserID: &u, Kind: "staff", Roles: []string{"hiring_manager"}}})
		}(i, u)
	}
	wg.Wait()
	okCount := 0
	for _, err := range errs {
		switch apiCode(err) {
		case "":
			okCount++
		case "application_inactive":
		default:
			t.Fatalf("hasil approver serentak tak terduga: %v", err)
		}
	}
	if okCount != 1 || e.count(`SELECT count(*) FROM transition_history WHERE application_id = $1 AND trigger = 'approval'`, b) != 1 {
		t.Fatalf("harus tepat satu persetujuan efektif, ok=%d", okCount)
	}
	if st := e.scalar(`SELECT status FROM job_applications WHERE id = $1`, b); st != "hired" {
		t.Fatalf("status setelah disetujui = %v", st)
	}

	// Approval basi setelah withdrawal: lamaran ditarik, persetujuan berikutnya ditolak dan approval lama tidak berlaku.
	c := e.appAtExam("tarik@contoh.test", "3201014507990203")
	toApproval(c, rec)
	cand := e.userID("tarik@contoh.test")
	if _, err := e.exec(workflow.Command{ExecutionID: uuid.New(), ApplicationID: c, Kind: "withdraw", Actor: workflow.Actor{UserID: &cand, Kind: "candidate"}}); err != nil {
		t.Fatal(err)
	}
	_, err = e.exec(workflow.Command{ExecutionID: uuid.New(), ApplicationID: c, Kind: "approval", Decision: "approve",
		Actor: workflow.Actor{UserID: &hm, Kind: "staff", Roles: []string{"hiring_manager"}}})
	if apiCode(err) != "application_inactive" {
		t.Fatalf("approval setelah withdraw: %v", err)
	}
	// Event hasil ujian untuk lamaran yang ditarik diabaikan.
	res, err := e.exec(workflow.Command{ExecutionID: uuid.New(), ApplicationID: c, Kind: "event", Event: "assessment.released", AssessmentKey: "tes-dasar",
		Facts: workflow.Facts{"assessment.status": "released", "assessment.score": 90.0}, Actor: workflow.Actor{Kind: "system"}})
	if err != nil || res.Changed {
		t.Fatalf("event untuk lamaran tidak aktif harus diabaikan: %+v %v", res, err)
	}

	// Via API: HM menyetujui lamaran yang dipindahkan recruiter.
	d := e.appAtExam("api.setuju@contoh.test", "3201014507990204")
	toApproval(d, rec)
	hmc := e.client().mustLogin("hm.ops@arunika.example")
	det := hmc.do("GET", "/applications/"+d.String(), nil)
	r := hmc.do("POST", "/applications/"+d.String()+"/approvals", map[string]any{"decision": "approve", "comment": "Sesuai kebutuhan lini", "expectedVersion": det.Body["version"]})
	expect(t, r, 200, "HM menyetujui")
	if r.Body["toStage"] != "diterima" || r.Body["status"] != "hired" {
		t.Fatalf("hasil approval API: %s", r.Raw)
	}
}

func TestWorkflowPublishVersioningAndPin(t *testing.T) {
	e := newEnv(t)
	admin := e.client().mustLogin("admin@arunika.example")
	list := admin.do("GET", "/workflows", nil)
	wfID := list.Body["items"].([]any)[0].(map[string]any)["id"].(string)
	wf := admin.do("GET", "/workflows/"+wfID, nil)
	rev := wf.Body["draftRevision"]

	// Lamaran lama terikat versi 1.
	old := e.appAtExam("lama@contoh.test", "3201014507990301")

	// Draft invalid boleh disimpan tetapi tidak boleh diterbitkan.
	var def workflow.Definition
	b, _ := json.Marshal(wf.Body["draft"])
	_ = json.Unmarshal(b, &def)
	bad := def
	bad.Edges = append([]workflow.Edge{}, def.Edges...)
	bad.Edges = append(bad.Edges, workflow.Edge{ID: "loop", From: "interview", To: "screening", Trigger: "manual", Label: "Kembali"})
	s := admin.do("PUT", "/workflows/"+wfID+"/draft", map[string]any{"name": "Rekrutmen Standar", "definition": bad, "draftRevision": rev})
	expect(t, s, 200, "simpan draft invalid")
	if len(s.Body["issues"].([]any)) == 0 {
		t.Fatal("draft dengan siklus harus memiliki temuan")
	}
	p := admin.do("POST", "/workflows/"+wfID+"/publish", map[string]any{"draftRevision": s.Body["draftRevision"]})
	if p.errCode() != "workflow_invalid" {
		t.Fatalf("publish draft invalid: %s", p.Raw)
	}
	// Simpan dengan revisi basi ditolak.
	if r := admin.do("PUT", "/workflows/"+wfID+"/draft", map[string]any{"name": "X Y", "definition": def, "draftRevision": rev}); r.errCode() != "stale_version" {
		t.Fatalf("draft basi: %s", r.Raw)
	}
	// Ambang dinaikkan ke 80, diterbitkan sebagai versi 2.
	def.Edges[1].When.All[1].Value = 80.0
	s = admin.do("PUT", "/workflows/"+wfID+"/draft", map[string]any{"name": "Rekrutmen Standar", "definition": def, "draftRevision": s.Body["draftRevision"]})
	expect(t, s, 200, "simpan draft valid")
	sim := admin.do("POST", "/workflows/"+wfID+"/simulate", map[string]any{"steps": []map[string]any{{"kind": "manual", "to": "ujian"},
		{"kind": "event", "event": "assessment.released", "facts": map[string]any{"assessment.status": "released", "assessment.score": 78}}}})
	steps := sim.Body["steps"].([]any)
	if steps[len(steps)-1].(map[string]any)["stage"] != "ditolak" {
		t.Fatalf("simulasi dengan ambang 80: %s", sim.Raw)
	}
	// Dua publish bersamaan: nomor versi tidak ganda.
	var wg sync.WaitGroup
	pubs := make([]resp, 2)
	for i := range pubs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pubs[i] = admin.do("POST", "/workflows/"+wfID+"/publish", map[string]any{"draftRevision": s.Body["draftRevision"], "notes": "Ambang 80"})
		}(i)
	}
	wg.Wait()
	for _, p := range pubs {
		if p.Status != 201 {
			t.Fatalf("publish bersamaan: %s", p.Raw)
		}
	}
	if pubs[0].Body["versionNo"] == pubs[1].Body["versionNo"] {
		t.Fatal("dua publish menghasilkan nomor versi sama")
	}
	if e.count(`SELECT count(DISTINCT version_no) FROM workflow_versions`) != 3 {
		t.Fatal("harus ada 3 versi berbeda")
	}
	// Versi immutable di level database.
	if _, err := e.pool.Exec(context.Background(), `UPDATE workflow_versions SET notes = 'ubah'`); err == nil {
		t.Fatal("workflow_versions harus immutable")
	}

	// Lamaran lama tetap pada versi 1: nilai 78 masih lulus (ambang 75).
	res, err := e.exec(workflow.Command{ExecutionID: uuid.New(), ApplicationID: old, Kind: "event", Event: "assessment.released", AssessmentKey: "tes-dasar",
		Facts: workflow.Facts{"assessment.status": "released", "assessment.score": 78.0}, Actor: workflow.Actor{Kind: "system"}})
	if err != nil || res.ToStage != "interview" {
		t.Fatalf("lamaran lama harus memakai versi 1: %+v %v", res, err)
	}
	// Lamaran baru memakai versi aktif terbaru: nilai 78 ditolak.
	fresh := e.appAtExam("baru@contoh.test", "3201014507990302")
	res, err = e.exec(workflow.Command{ExecutionID: uuid.New(), ApplicationID: fresh, Kind: "event", Event: "assessment.released", AssessmentKey: "tes-dasar",
		Facts: workflow.Facts{"assessment.status": "released", "assessment.score": 78.0}, Actor: workflow.Actor{Kind: "system"}})
	if err != nil || res.ToStage != "ditolak" {
		t.Fatalf("lamaran baru harus memakai versi terbaru: %+v %v", res, err)
	}
	// Rollback ke versi 1 untuk lamaran baru berikutnya.
	vers := admin.do("GET", "/workflows/"+wfID+"/versions", nil).Body["items"].([]any)
	v1 := vers[len(vers)-1].(map[string]any)["id"]
	expect(t, admin.do("POST", "/workflows/"+wfID+"/rollback", map[string]any{"versionId": v1, "reason": "Ambang 80 terlalu ketat"}), 200, "rollback")
	after := e.appAtExam("setelah.rollback@contoh.test", "3201014507990303")
	if got := e.scalar(`SELECT v.version_no FROM job_applications a JOIN workflow_versions v ON v.id = a.workflow_version_id WHERE a.id = $1`, after); got != int32(1) {
		t.Fatalf("lamaran setelah rollback memakai versi %v", got)
	}
	// Recruiter tidak boleh menerbitkan workflow.
	rec := e.client().mustLogin("rekruter.ops@arunika.example")
	expect(t, rec.do("POST", "/workflows/"+wfID+"/publish", map[string]any{"draftRevision": 1}), 403, "recruiter publish")
}
