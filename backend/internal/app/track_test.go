package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"ats/internal/workflow"
)

// fieldErrors: kunci field yang ditandai pada respons 422 (error.fieldErrors).
func fieldErrors(r resp) map[string]any {
	e, _ := r.Body["error"].(map[string]any)
	fe, _ := e["fieldErrors"].(map[string]any)
	return fe
}

func expectFieldErr(t *testing.T, r resp, field, what string) {
	t.Helper()
	expect(t, r, 422, what)
	if _, ok := fieldErrors(r)[field]; !ok {
		t.Fatalf("%s: field %q tidak ditandai: %s", what, field, r.Raw)
	}
}

// requisitionDisetujui: requisition OPS dibuat HM, diajukan, lalu disetujui HR unit OPS.
func requisitionDisetujui(t *testing.T, e *env, track string, headcount int) string {
	t.Helper()
	hm := e.client().mustLogin("hm.ops@arunika.example")
	r := hm.do("POST", "/requisitions", map[string]any{"unitCode": "OPS", "locationCode": "KRW", "title": "Permintaan uji " + track,
		"headcount": headcount, "employmentType": "contract", "justification": "Uji track", "track": track, "reasonCategory": "penambahan"})
	expect(t, r, 201, "buat requisition "+track)
	id := idOf(r, "id")
	expect(t, hm.do("POST", "/requisitions/"+id+"/submit", map[string]any{"version": 1}), 200, "ajukan requisition")
	hr := e.client().mustLogin("hr.ops@arunika.example")
	expect(t, hr.do("POST", "/requisitions/"+id+"/approve", map[string]any{"version": 2, "note": "Sesuai anggaran"}), 200, "setujui requisition")
	return id
}

// lowonganUji membuat lowongan draft di unit OPS; extra menimpa field default.
func lowonganUji(hr *client, slug string, extra map[string]any) resp {
	body := map[string]any{"unitCode": "OPS", "locationCode": "KRW", "workflowKey": "standar", "title": "Lowongan uji " + slug,
		"slug": slug, "employmentType": "contract", "quota": 1}
	for k, v := range extra {
		body[k] = v
	}
	return hr.do("POST", "/staff/jobs", body)
}

func hasManualEdge(def workflow.Definition, from, to string) bool {
	for _, ed := range def.Edges {
		if ed.From == from && ed.To == to && ed.Trigger == "manual" {
			return true
		}
	}
	return false
}

func TestTrackRequisitionCreateRead(t *testing.T) {
	e := newEnv(t)
	hm := e.client().mustLogin("hm.ops@arunika.example")
	dibuat := map[string]string{}
	for _, track := range []string{"OPERATOR", "STAFF", "MT_S1_D4"} {
		r := hm.do("POST", "/requisitions", map[string]any{"unitCode": "OPS", "locationCode": "KRW", "title": "Permintaan uji " + track,
			"headcount": 1, "employmentType": "contract", "track": track, "reasonCategory": "penggantian"})
		expect(t, r, 201, "buat requisition "+track)
		if r.Body["track"] != track {
			t.Fatalf("track di respons %v, ingin %s", r.Body["track"], track)
		}
		dibuat[idOf(r, "id")] = track
	}
	terbaca := 0
	for _, it := range hm.do("GET", "/requisitions", nil).Body["items"].([]any) {
		m := it.(map[string]any)
		if track, ok := dibuat[m["id"].(string)]; ok {
			terbaca++
			if m["track"] != track || m["reasonCategory"] != "penggantian" {
				t.Fatalf("track terbaca kembali salah: %v", m)
			}
		}
	}
	if terbaca != 3 {
		t.Fatalf("GET /requisitions memuat %d dari 3 requisition uji", terbaca)
	}
	if n := e.count(`SELECT count(*) FROM requisitions WHERE track IN ('OPERATOR', 'STAFF', 'MT_S1_D4') AND title LIKE 'Permintaan uji %'`); n != 3 {
		t.Fatalf("baris requisition di DB: %d, ingin 3", n)
	}
}

func TestTrackValidation(t *testing.T) {
	e := newEnv(t)
	hm := e.client().mustLogin("hm.ops@arunika.example")
	hr := e.client().mustLogin("hr.ops@arunika.example")
	for i, track := range []string{"MANAGER", "", "staff"} {
		expectFieldErr(t, hm.do("POST", "/requisitions", map[string]any{"unitCode": "OPS", "locationCode": "KRW", "title": "Permintaan uji track",
			"headcount": 1, "employmentType": "contract", "track": track, "reasonCategory": "penambahan"}), "track", "requisition track "+track)
		expectFieldErr(t, lowonganUji(hr, fmt.Sprintf("track-uji-%d", i), map[string]any{"track": track}), "track", "lowongan track "+track)
	}
}

func TestJobInheritsRequisitionTrack(t *testing.T) {
	e := newEnv(t)
	reqID := requisitionDisetujui(t, e, "MT_S1_D4", 2)
	hr := e.client().mustLogin("hr.ops@arunika.example")
	expectFieldErr(t, lowonganUji(hr, "mt-track-staff", map[string]any{"requisitionId": reqID, "workflowKey": "track-mt-s1-d4", "track": "STAFF"}),
		"track", "lowongan MT dengan track STAFF")
	expect(t, lowonganUji(hr, "mt-track-sama", map[string]any{"requisitionId": reqID, "workflowKey": "track-mt-s1-d4", "track": "MT_S1_D4"}), 201,
		"lowongan MT dengan track sama")
	warisi := lowonganUji(hr, "mt-track-warisi", map[string]any{"requisitionId": reqID, "workflowKey": "track-mt-s1-d4"})
	expect(t, warisi, 201, "lowongan MT tanpa track")
	if warisi.Body["track"] != "MT_S1_D4" {
		t.Fatalf("track lowongan warisan: %v", warisi.Body["track"])
	}
	if n := e.count(`SELECT count(*) FROM job_openings WHERE requisition_id = $1 AND track = 'MT_S1_D4'`, reqID); n != 2 {
		t.Fatalf("lowongan MT tertaut requisition di DB: %d, ingin 2", n)
	}
}

func TestPublicJobsTrackFilter(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	r := c.do("GET", "/jobs?track=OPERATOR", nil)
	expect(t, r, 200, "daftar publik track OPERATOR")
	items := r.Body["items"].([]any)
	want := e.count(`SELECT count(*) FROM job_openings WHERE status = 'published' AND track = 'OPERATOR'`)
	if want == 0 || int64(len(items)) != want {
		t.Fatalf("lowongan OPERATOR publik: %d, di DB %d", len(items), want)
	}
	for _, it := range items {
		if it.(map[string]any)["track"] != "OPERATOR" {
			t.Fatalf("filter track bocor: %v", it)
		}
	}
	expectFieldErr(t, c.do("GET", "/jobs?track=XYZ", nil), "track", "filter track tidak dikenal")
	all := c.do("GET", "/jobs", nil)
	expect(t, all, 200, "daftar publik tanpa filter")
	if n := int64(len(all.Body["items"].([]any))); n != e.count(`SELECT count(*) FROM job_openings WHERE status = 'published'`) {
		t.Fatalf("daftar publik tanpa filter berubah: %d", n)
	}
}

func TestDraftJobTrackHiddenFromPublic(t *testing.T) {
	e := newEnv(t)
	hr := e.client().mustLogin("hr.ops@arunika.example")
	expect(t, lowonganUji(hr, "draf-mt-uji", map[string]any{"workflowKey": "track-mt-s1-d4", "track": "MT_S1_D4"}), 201, "lowongan draft MT")
	c := e.client()
	for _, path := range []string{"/jobs?track=MT_S1_D4", "/jobs"} {
		for _, it := range c.do("GET", path, nil).Body["items"].([]any) {
			if it.(map[string]any)["slug"] == "draf-mt-uji" {
				t.Fatalf("lowongan draft muncul di %s", path)
			}
		}
	}
	expect(t, c.do("GET", "/jobs/draf-mt-uji", nil), 404, "detail lowongan draft")
}

func TestRequisitionTargetJoinDate(t *testing.T) {
	e := newEnv(t)
	hm := e.client().mustLogin("hm.ops@arunika.example")
	wib := time.FixedZone("WIB", 7*60*60) // Asia/Jakarta (zona organisasi seed), tanpa perubahan musim.
	now := time.Now().In(wib)
	kemarin := now.AddDate(0, 0, -1).Format("2006-01-02")
	besok := now.AddDate(0, 0, 1).Format("2006-01-02")
	buat := func(target any) string {
		body := map[string]any{"unitCode": "OPS", "locationCode": "KRW", "title": "Permintaan tanggal target", "headcount": 1,
			"employmentType": "contract", "track": "STAFF", "reasonCategory": "penambahan"}
		if target != nil {
			body["targetJoinDate"] = target
		}
		r := hm.do("POST", "/requisitions", body)
		expect(t, r, 201, "buat requisition dengan tanggal target")
		return idOf(r, "id")
	}
	lewat := buat(kemarin)
	expectFieldErr(t, hm.do("POST", "/requisitions/"+lewat+"/submit", map[string]any{"version": 1}), "targetJoinDate", "submit tanggal kemarin")
	if s := e.scalar(`SELECT status FROM requisitions WHERE id = $1`, lewat); s != "draft" {
		t.Fatalf("status setelah submit ditolak: %v", s)
	}
	depan := buat(besok)
	expect(t, hm.do("POST", "/requisitions/"+depan+"/submit", map[string]any{"version": 1}), 200, "submit tanggal besok")
	kosong := buat(nil)
	expect(t, hm.do("POST", "/requisitions/"+kosong+"/submit", map[string]any{"version": 1}), 200, "submit tanpa tanggal")
}

func TestReasonCategoryEnum(t *testing.T) {
	e := newEnv(t)
	hm := e.client().mustLogin("hm.ops@arunika.example")
	body := func(reason any) map[string]any {
		m := map[string]any{"unitCode": "OPS", "locationCode": "KRW", "title": "Permintaan alasan", "headcount": 1,
			"employmentType": "contract", "track": "STAFF"}
		if reason != nil {
			m["reasonCategory"] = reason
		}
		return m
	}
	expectFieldErr(t, hm.do("POST", "/requisitions", body("lainnya")), "reasonCategory", "alasan di luar enum")
	expectFieldErr(t, hm.do("POST", "/requisitions", body(nil)), "reasonCategory", "alasan kosong")
	for _, reason := range []string{"penambahan", "penggantian", "proyek_baru"} {
		expect(t, hm.do("POST", "/requisitions", body(reason)), 201, "alasan "+reason)
	}
}

func TestJobQuotaWithinHeadcount(t *testing.T) {
	e := newEnv(t)
	reqID := requisitionDisetujui(t, e, "STAFF", 10)
	hr := e.client().mustLogin("hr.ops@arunika.example")
	a := lowonganUji(hr, "kuota-a", map[string]any{"requisitionId": reqID, "quota": 6})
	expect(t, a, 201, "lowongan A kuota 6")
	idA := idOf(a, "id")
	expectFieldErr(t, lowonganUji(hr, "kuota-b", map[string]any{"requisitionId": reqID, "quota": 5}), "quota", "lowongan B kuota 5 melebihi sisa")
	expect(t, lowonganUji(hr, "kuota-b", map[string]any{"requisitionId": reqID, "quota": 4}), 201, "lowongan B kuota 4")
	expectFieldErr(t, hr.do("PATCH", "/staff/jobs/"+idA, map[string]any{"expectedVersion": 1, "quota": 7}), "quota", "ubah kuota A jadi 7")
	if q := fmt.Sprint(e.scalar(`SELECT quota FROM job_openings WHERE id = $1`, idA)); q != "6" {
		t.Fatalf("kuota A setelah ditolak: %s, ingin 6", q)
	}
	expect(t, hr.do("PATCH", "/staff/jobs/"+idA, map[string]any{"expectedVersion": 1, "quota": 5}), 200, "ubah kuota A jadi 5")
	expect(t, hr.do("PATCH", "/staff/jobs/"+idA, map[string]any{"expectedVersion": 1, "quota": 5}), 409, "ubah dengan versi basi")
}

func TestJobQuotaRace(t *testing.T) {
	e := newEnv(t)
	reqID := requisitionDisetujui(t, e, "STAFF", 10)
	const n = 2
	clients := make([]*client, n)
	for i := range clients {
		clients[i] = e.client().mustLogin("hr.ops@arunika.example")
	}
	var wg sync.WaitGroup
	statuses := make([]resp, n)
	for i, c := range clients {
		wg.Add(1)
		go func(i int, c *client) {
			defer wg.Done()
			statuses[i] = lowonganUji(c, fmt.Sprintf("balap-kuota-%d", i), map[string]any{"requisitionId": reqID, "quota": 6})
		}(i, c)
	}
	wg.Wait()
	ok := 0
	for i, s := range statuses {
		switch {
		case s.Status == 201:
			ok++
		case s.Status == 422 && fieldErrors(s)["quota"] != nil:
		default:
			t.Fatalf("lowongan %d: hasil tak terduga %d %s", i, s.Status, s.Raw)
		}
	}
	if ok != 1 {
		t.Fatalf("lowongan berhasil %d dari %d, ingin tepat 1", ok, n)
	}
	if got := e.count(`SELECT count(*) FROM job_openings WHERE requisition_id = $1`, reqID); got != 1 {
		t.Fatalf("lowongan tersimpan %d, ingin 1", got)
	}
}

func TestClosedJobExcludedFromQuota(t *testing.T) {
	e := newEnv(t)
	reqID := requisitionDisetujui(t, e, "STAFF", 6)
	hr := e.client().mustLogin("hr.ops@arunika.example")
	a := lowonganUji(hr, "tutup-a", map[string]any{"requisitionId": reqID, "quota": 6})
	expect(t, a, 201, "lowongan A kuota 6")
	idA := idOf(a, "id")
	expectFieldErr(t, lowonganUji(hr, "tutup-b", map[string]any{"requisitionId": reqID, "quota": 1}), "quota", "sebelum tutup: kuota penuh")
	expect(t, hr.do("POST", "/staff/jobs/"+idA+"/close", map[string]any{"version": 1, "reason": "Kuota terpenuhi"}), 200, "tutup lowongan A")
	expect(t, lowonganUji(hr, "tutup-c", map[string]any{"requisitionId": reqID, "quota": 6}), 201, "setelah tutup: lowongan baru kuota 6")
	expect(t, hr.do("PATCH", "/staff/jobs/"+idA, map[string]any{"expectedVersion": 2, "quota": 5}), 422, "ubah kuota lowongan closed")
}

func TestTrackScopeNotFound(t *testing.T) {
	e := newEnv(t)
	reqID := requisitionDisetujui(t, e, "STAFF", 2)
	hr := e.client().mustLogin("hr.ops@arunika.example")
	jobID := idOf(lowonganUji(hr, "scope-ops", map[string]any{"requisitionId": reqID, "quota": 1}), "id")
	hrTI := e.client().mustLogin("hr.ti@arunika.example")
	rekrut := e.client().mustLogin("rekruter.ti@arunika.example")
	for _, who := range []*client{hrTI, rekrut} {
		for _, it := range who.do("GET", "/requisitions", nil).Body["items"].([]any) {
			if it.(map[string]any)["id"] == reqID {
				t.Fatal("requisition unit OPS terlihat dari luar scope")
			}
		}
		for _, it := range who.do("GET", "/staff/jobs", nil).Body["items"].([]any) {
			if it.(map[string]any)["id"] == jobID {
				t.Fatal("lowongan unit OPS terlihat dari luar scope")
			}
		}
	}
	expect(t, hrTI.do("PATCH", "/staff/jobs/"+jobID, map[string]any{"expectedVersion": 1, "quota": 2}), 404, "ubah kuota di luar scope")
	expect(t, hrTI.do("POST", "/requisitions/"+reqID+"/approve", map[string]any{"version": 3}), 404, "setujui di luar scope")
	draf := idOf(e.client().mustLogin("hm.ops@arunika.example").do("POST", "/requisitions", map[string]any{"unitCode": "OPS", "locationCode": "KRW",
		"title": "Permintaan di luar scope", "headcount": 1, "employmentType": "contract", "track": "STAFF", "reasonCategory": "penambahan"}), "id")
	expect(t, rekrut.do("POST", "/requisitions/"+draf+"/submit", map[string]any{"version": 1}), 404, "ajukan di luar scope")
}

func TestTrackQuotaAudit(t *testing.T) {
	e := newEnv(t)
	reqID := requisitionDisetujui(t, e, "MT_S1_D4", 2)
	if v := e.scalar(`SELECT detail->>'track' FROM audit_logs WHERE action = 'requisition.create' AND target_id = $1`, reqID); v != "MT_S1_D4" {
		t.Fatalf("audit requisition.create: track %v", v)
	}
	hr := e.client().mustLogin("hr.ops@arunika.example")
	job := lowonganUji(hr, "audit-kuota", map[string]any{"requisitionId": reqID, "quota": 2, "workflowKey": "track-mt-s1-d4"})
	expect(t, job, 201, "lowongan untuk audit")
	jobID := idOf(job, "id")
	if v := e.scalar(`SELECT detail->>'track' || '/' || (detail->>'quota') FROM audit_logs WHERE action = 'job.create' AND target_id = $1`, jobID); v != "MT_S1_D4/2" {
		t.Fatalf("audit job.create: %v", v)
	}
	expect(t, hr.do("PATCH", "/staff/jobs/"+jobID, map[string]any{"expectedVersion": 1, "quota": 1}), 200, "ubah kuota lowongan")
	if v := e.scalar(`SELECT detail->>'track' || '/' || (detail->>'quotaFrom') || '->' || (detail->>'quotaTo') FROM audit_logs WHERE action = 'job.quota_update' AND target_id = $1`, jobID); v != "MT_S1_D4/2->1" {
		t.Fatalf("audit job.quota_update: %v", v)
	}
}

func TestSeedWorkflowTemplates(t *testing.T) {
	e := newEnv(t)
	admin := e.client().mustLogin("admin@arunika.example")
	items := admin.do("GET", "/workflows", nil).Body["items"].([]any)
	if items[0].(map[string]any)["name"] != "Rekrutmen Standar" {
		t.Fatalf("urutan workflow berubah: %v; test migrasi mengandalkan Rekrutmen Standar di urutan pertama", items[0])
	}
	for key := range map[string]string{"track-operator": "", "track-staff": "", "track-mt-s1-d4": ""} {
		raw := e.scalarBytes(`SELECT v.definition FROM workflows w JOIN workflow_versions v ON v.id = w.active_version_id WHERE w.org_id = $1 AND w.key = $2`,
			e.orgID.String(), key)
		var def workflow.Definition
		if err := json.Unmarshal(raw, &def); err != nil {
			t.Fatal(err)
		}
		if issues := workflow.Validate(def); len(issues) > 0 {
			t.Fatalf("workflow %s tidak lolos validasi: %v", key, issues)
		}
		var interview []string
		cekFisik := false
		for _, s := range def.Stages {
			if s.Type == "interview" {
				interview = append(interview, s.Key)
			}
			if strings.Contains(strings.ToLower(s.Name), "cek fisik") || strings.Contains(s.Key, "cek_fisik") {
				cekFisik = true
			}
		}
		switch key {
		case "track-mt-s1-d4":
			want := []string{"interview_hr", "interview_user", "interview_manajemen"}
			if !slices.Equal(interview, want) {
				t.Fatalf("tahap interview MT: %v, ingin %v", interview, want)
			}
			for i := 0; i+1 < len(want); i++ {
				if !hasManualEdge(def, want[i], want[i+1]) {
					t.Fatalf("tahap interview MT tidak berurutan: %s -> %s", want[i], want[i+1])
				}
			}
		case "track-staff":
			if cekFisik {
				t.Fatal("workflow Staff memuat tahap cek fisik")
			}
		case "track-operator":
			if !cekFisik {
				t.Fatal("workflow Operator tidak memuat tahap Cek Fisik")
			}
		}
	}
}

// Kuota yang sama dengan nilai saat ini: 200, versi tetap, dan tidak ada audit job.quota_update.
func TestPatchKuotaSamaTanpaPerubahan(t *testing.T) {
	e := newEnv(t)
	reqID := requisitionDisetujui(t, e, "STAFF", 3)
	hr := e.client().mustLogin("hr.ops@arunika.example")
	job := lowonganUji(hr, "kuota-sama", map[string]any{"requisitionId": reqID, "quota": 2})
	expect(t, job, 201, "lowongan kuota 2")
	id := idOf(job, "id")
	r := hr.do("PATCH", "/staff/jobs/"+id, map[string]any{"expectedVersion": 1, "quota": 2})
	expect(t, r, 200, "PATCH kuota sama")
	if r.Body["version"] != float64(1) {
		t.Fatalf("versi di respons berubah: %v", r.Body["version"])
	}
	if v := fmt.Sprint(e.scalar(`SELECT version FROM job_openings WHERE id = $1`, id)); v != "1" {
		t.Fatalf("versi di DB berubah: %s", v)
	}
	if n := e.count(`SELECT count(*) FROM audit_logs WHERE action = 'job.quota_update' AND target_id = $1`, id); n != 0 {
		t.Fatalf("audit job.quota_update tertulis %d kali, ingin 0", n)
	}
}

// Batas 100 karakter untuk line dihitung setelah spasi di awal dan akhir dibuang.
func TestRequisitionLineTrimSebelumBatas(t *testing.T) {
	e := newEnv(t)
	hm := e.client().mustLogin("hm.ops@arunika.example")
	dasar := map[string]any{"unitCode": "OPS", "locationCode": "KRW", "title": "Permintaan line uji", "headcount": 1,
		"employmentType": "contract", "track": "STAFF", "reasonCategory": "penambahan"}
	dengan := func(line string) map[string]any {
		m := map[string]any{}
		for k, v := range dasar {
			m[k] = v
		}
		m["line"] = line
		return m
	}
	satu := strings.Repeat("L", 98)
	r := hm.do("POST", "/requisitions", dengan("  "+satu+"  "))
	expect(t, r, 201, "line 98 karakter dengan spasi di awal dan akhir")
	if r.Body["line"] != satu {
		t.Fatalf("line tersimpan %q, ingin %q", r.Body["line"], satu)
	}
	expectFieldErr(t, hm.do("POST", "/requisitions", dengan(strings.Repeat("L", 101))), "line", "line 101 karakter ditolak")
}

// reqUnitUji: body POST /requisitions untuk unit tertentu, dengan track dan alasan yang wajib diisi.
func reqUnitUji(kode, judul string) map[string]any {
	return map[string]any{"unitCode": kode, "locationCode": "KRW", "title": judul, "headcount": 1, "employmentType": "contract",
		"track": "STAFF", "reasonCategory": "penambahan"}
}

// idRequisitionUnit: id requisition yang dikembalikan GET /requisitions dengan unitCode tertentu.
func idRequisitionUnit(t *testing.T, c *client, kode string) []string {
	t.Helper()
	r := c.do("GET", "/requisitions?unitCode="+url.QueryEscape(kode), nil)
	expect(t, r, 200, "daftar unitCode="+kode)
	var id []string
	for _, it := range r.Body["items"].([]any) {
		id = append(id, it.(map[string]any)["id"].(string))
	}
	return id
}

// Filter unitCode hanya mengembalikan requisition unit tersebut.
func TestFilterUnitCodeHanyaUnitTerpilih(t *testing.T) {
	e := newEnv(t)
	hm := e.client().mustLogin("hm.ops@arunika.example")
	ops := idOf(hm.do("POST", "/requisitions", reqUnitUji("OPS", "Filter OPS satu")), "id")
	ti := idOf(hm.do("POST", "/requisitions", reqUnitUji("TI", "Filter TI satu")), "id")
	if got := idRequisitionUnit(t, hm, "TI"); !slices.Equal(got, []string{ti}) {
		t.Fatalf("unitCode=TI: %v, ingin [%s]", got, ti)
	}
	if got := idRequisitionUnit(t, hm, "OPS"); !slices.Equal(got, []string{ops}) {
		t.Fatalf("unitCode=OPS: %v, ingin [%s]", got, ops)
	}
}

// Kode unit yang formatnya salah atau tidak dikenal ditolak 422.
func TestFilterUnitCodeTidakValid422(t *testing.T) {
	e := newEnv(t)
	hm := e.client().mustLogin("hm.ops@arunika.example")
	for _, kode := range []string{"ops", "TI!", "A", strings.Repeat("X", 21), "XYZ"} {
		expectFieldErr(t, hm.do("GET", "/requisitions?unitCode="+url.QueryEscape(kode), nil), "unitCode", "unitCode "+kode)
	}
}

// Recruiter OPS dengan unitCode=TI tidak melihat requisition TI: filter hanya mempersempit scope.
func TestFilterUnitCodeTidakMembukaScope(t *testing.T) {
	e := newEnv(t)
	hm := e.client().mustLogin("hm.ops@arunika.example")
	opsReq := idOf(hm.do("POST", "/requisitions", reqUnitUji("OPS", "Scope OPS")), "id")
	tiReq := idOf(hm.do("POST", "/requisitions", reqUnitUji("TI", "Scope TI")), "id")
	if n := e.count(`SELECT count(*) FROM requisitions WHERE id = $1`, tiReq); n != 1 {
		t.Fatalf("requisition TI tidak tersimpan: %d", n)
	}
	rekrut := e.client().mustLogin("rekruter.ops@arunika.example")
	if got := idRequisitionUnit(t, rekrut, "OPS"); !slices.Equal(got, []string{opsReq}) {
		t.Fatalf("recruiter OPS, unitCode=OPS: %v, ingin [%s]", got, opsReq)
	}
	if got := idRequisitionUnit(t, rekrut, "TI"); len(got) != 0 {
		t.Fatalf("recruiter OPS melihat requisition TI lewat filter: %v", got)
	}
}

// Dua unit dengan nama sama tetapi kode berbeda tidak tercampur di filter.
func TestFilterUnitCodeNamaSamaKodeBeda(t *testing.T) {
	e := newEnv(t)
	if _, err := e.pool.Exec(context.Background(), `INSERT INTO units (org_id, code, name) VALUES ($1, 'OPS2', 'Operasional Pabrik')`, e.orgID.String()); err != nil {
		t.Fatal(err)
	}
	hm := e.client().mustLogin("hm.ops@arunika.example")
	a := idOf(hm.do("POST", "/requisitions", reqUnitUji("OPS", "Nama sama kode OPS")), "id")
	b := idOf(hm.do("POST", "/requisitions", reqUnitUji("OPS2", "Nama sama kode OPS2")), "id")
	if got := idRequisitionUnit(t, hm, "OPS"); !slices.Equal(got, []string{a}) {
		t.Fatalf("unitCode=OPS: %v, ingin [%s]", got, a)
	}
	if got := idRequisitionUnit(t, hm, "OPS2"); !slices.Equal(got, []string{b}) {
		t.Fatalf("unitCode=OPS2: %v, ingin [%s]", got, b)
	}
}

func TestCandidateAppliesTwoTracks(t *testing.T) {
	e := newEnv(t)
	const email = "track.kandidat@contoh.test"
	cand := e.registerVerified(email, "Kandidat Track", "3201014507997401")
	op := cand.do("POST", "/jobs/operator-forklift-karawang/apply", nil)
	expect(t, op, 201, "lamar lowongan OPERATOR")
	mt := cand.do("POST", "/jobs/management-trainee-teknologi/apply", nil)
	expect(t, mt, 201, "lamar lowongan MT")
	vOp := e.scalar(`SELECT workflow_version_id::text FROM job_applications WHERE id = $1`, idOf(op, "applicationId"))
	vMt := e.scalar(`SELECT workflow_version_id::text FROM job_applications WHERE id = $1`, idOf(mt, "applicationId"))
	if vOp == vMt {
		t.Fatalf("workflow_version_id sama untuk dua track: %v", vOp)
	}
	if n := e.count(`SELECT count(*) FROM job_applications a JOIN users u ON u.id = a.candidate_user_id WHERE u.email = $1`, email); n != 2 {
		t.Fatalf("lamaran kandidat: %d, ingin 2", n)
	}
}
