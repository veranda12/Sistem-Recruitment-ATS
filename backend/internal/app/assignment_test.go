package app

import "testing"

func TestShortlistAndRecruiterAssignment(t *testing.T) {
	e := newEnv(t)
	cand := e.registerVerified("shortlist@contoh.test", "Kandidat Shortlist", "3201014507997301")
	app := idOf(cand.do("POST", "/jobs/operator-produksi-karawang/apply", nil), "applicationId")
	rec := e.client().mustLogin("rekruter.ops@arunika.example")
	hr := e.client().mustLogin("hr.ops@arunika.example")
	recTI := e.client().mustLogin("rekruter.ti@arunika.example")
	hm := e.client().mustLogin("hm.ops@arunika.example")
	recID := e.userID("rekruter.ops@arunika.example").String()
	version := func() float64 { return rec.do("GET", "/applications/"+app, nil).Body["version"].(float64) }

	// Shortlist: recruiter dalam scope; versi basi ditolak; di luar scope 404; hiring manager (tanpa transition) 403.
	d := rec.do("GET", "/applications/"+app, nil)
	if d.Body["canShortlist"] != true || d.Body["canAssign"] != false || d.Body["recruiter"] != nil {
		t.Fatalf("hak recruiter salah: shortlist=%v assign=%v recruiter=%v", d.Body["canShortlist"], d.Body["canAssign"], d.Body["recruiter"])
	}
	v := version()
	expect(t, rec.do("POST", "/applications/"+app+"/shortlist", map[string]any{"shortlisted": true, "expectedVersion": v}), 200, "shortlist")
	expect(t, rec.do("POST", "/applications/"+app+"/shortlist", map[string]any{"shortlisted": false, "expectedVersion": v}), 409, "shortlist versi basi")
	expect(t, recTI.do("POST", "/applications/"+app+"/shortlist", map[string]any{"shortlisted": false, "expectedVersion": version()}), 404, "shortlist di luar scope")
	expect(t, hm.do("POST", "/applications/"+app+"/shortlist", map[string]any{"shortlisted": false, "expectedVersion": version()}), 403, "hiring manager tidak boleh shortlist")
	list := rec.do("GET", "/applications?shortlisted=true", nil).Body["items"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["id"] != app {
		t.Fatalf("filter shortlist: %v", list)
	}

	// Penugasan: hanya HR (applications.assign); calon penerima harus berhak memindahkan lamaran ini.
	expect(t, rec.do("GET", "/applications/"+app+"/assignable-recruiters", nil), 403, "recruiter tidak boleh menugaskan")
	opts := hr.do("GET", "/applications/"+app+"/assignable-recruiters", nil)
	expect(t, opts, 200, "daftar calon recruiter")
	names := map[string]bool{}
	for _, x := range opts.Body["items"].([]any) {
		names[x.(map[string]any)["displayName"].(string)] = true
	}
	if !names["Maya Sari"] || names["Andi Wijaya"] || names["Ratna Kusuma"] {
		t.Fatalf("calon recruiter harus dalam scope OPS dengan izin transition: %v", names)
	}
	tiID := e.userID("rekruter.ti@arunika.example").String()
	expect(t, hr.do("POST", "/applications/"+app+"/recruiter", map[string]any{"recruiterUserId": tiID, "expectedVersion": version()}), 422, "recruiter di luar scope")
	expect(t, hr.do("POST", "/applications/"+app+"/recruiter", map[string]any{"recruiterUserId": recID, "expectedVersion": version()}), 200, "tugaskan Maya")
	d = rec.do("GET", "/applications/"+app, nil)
	if r, _ := d.Body["recruiter"].(map[string]any); r == nil || r["name"] != "Maya Sari" {
		t.Fatalf("recruiter di detail: %v", d.Body["recruiter"])
	}
	mine := rec.do("GET", "/applications?recruiter=me", nil).Body["items"].([]any)
	if len(mine) != 1 || mine[0].(map[string]any)["recruiterName"] != "Maya Sari" {
		t.Fatalf("filter ditugaskan ke saya: %v", mine)
	}
	expect(t, hr.do("POST", "/applications/"+app+"/recruiter", map[string]any{"recruiterUserId": nil, "expectedVersion": version()}), 200, "lepas penugasan")
	if n := len(rec.do("GET", "/applications?recruiter=me", nil).Body["items"].([]any)); n != 0 {
		t.Fatalf("setelah dilepas masih %d lamaran", n)
	}
	if n := e.count(`SELECT count(*) FROM audit_logs WHERE action IN ('application.shortlist','application.assign_recruiter') AND target_id = $1`, app); n != 3 {
		t.Fatalf("audit shortlist/penugasan: %d", n)
	}
}
