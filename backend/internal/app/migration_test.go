package app

import (
	"encoding/json"
	"testing"
)

// Versi baru mengganti kunci tahap screening; lamaran pada versi lama dipindahkan lewat pratinjau lalu eksekusi.
func TestWorkflowApplicationMigration(t *testing.T) {
	e := newEnv(t)
	cand := e.registerVerified("migrasi@contoh.test", "Kandidat Migrasi", "3201014507997309")
	app := idOf(cand.do("POST", "/jobs/operator-produksi-karawang/apply", nil), "applicationId")
	other := e.registerVerified("migrasi2@contoh.test", "Kandidat Ujian", "3201014507997310")
	app2 := idOf(other.do("POST", "/jobs/operator-produksi-karawang/apply", nil), "applicationId")
	rec := e.client().mustLogin("rekruter.ops@arunika.example")
	d2 := rec.do("GET", "/applications/"+app2, nil)
	expect(t, rec.do("POST", "/applications/"+app2+"/transitions", map[string]any{"kind": "manual", "to": "ujian", "reason": "", "expectedVersion": d2.Body["version"]}), 200, "ke ujian")

	admin := e.client().mustLogin("admin@arunika.example")
	wfID := admin.do("GET", "/workflows", nil).Body["items"].([]any)[0].(map[string]any)["id"].(string)
	wf := admin.do("GET", "/workflows/"+wfID, nil)
	oldVersion := wf.Body["activeVersionId"].(string)
	b, _ := json.Marshal(wf.Body["draft"])
	var def map[string]any
	_ = json.Unmarshal(b, &def)
	// Ganti kunci tahap screening -> seleksi_berkas (nama juga berubah); tepi ikut disesuaikan.
	def["start"] = "seleksi_berkas"
	for _, s := range def["stages"].([]any) {
		if m := s.(map[string]any); m["key"] == "screening" {
			m["key"], m["name"] = "seleksi_berkas", "Seleksi Berkas"
		}
	}
	for _, ed := range def["edges"].([]any) {
		if m := ed.(map[string]any); m["from"] == "screening" {
			m["from"] = "seleksi_berkas"
		}
	}
	s := admin.do("PUT", "/workflows/"+wfID+"/draft", map[string]any{"name": "Rekrutmen Standar", "definition": def, "draftRevision": wf.Body["draftRevision"]})
	expect(t, s, 200, "simpan draft")
	expect(t, admin.do("POST", "/workflows/"+wfID+"/publish", map[string]any{"draftRevision": s.Body["draftRevision"], "notes": "Ganti nama tahap"}), 201, "terbitkan v2")
	newVersion := admin.do("GET", "/workflows/"+wfID, nil).Body["activeVersionId"].(string)

	ov := admin.do("GET", "/workflows/"+wfID+"/migrations", nil)
	expect(t, ov, 200, "ringkasan versi")
	prev := admin.do("POST", "/workflows/"+wfID+"/migrations/preview", map[string]any{"fromVersionId": oldVersion})
	expect(t, prev, 200, "pratinjau tanpa pemetaan")
	if prev.Body["ready"] != false || prev.Body["total"].(float64) < 2 {
		t.Fatalf("tahap screening harus perlu dipetakan: %v", prev.Body)
	}
	body := map[string]any{"fromVersionId": oldVersion, "mapping": map[string]string{"screening": "seleksi_berkas"}, "expectedActiveVersionId": newVersion, "reason": "Penyesuaian nama tahap"}
	expect(t, admin.do("POST", "/workflows/"+wfID+"/migrations", map[string]any{"fromVersionId": oldVersion, "mapping": map[string]string{"screening": "diterima"},
		"expectedActiveVersionId": newVersion, "reason": "Coba ke tahap akhir"}), 409, "tujuan keputusan akhir ditolak")
	expect(t, admin.do("POST", "/workflows/"+wfID+"/migrations", map[string]any{"fromVersionId": oldVersion, "mapping": body["mapping"],
		"expectedActiveVersionId": oldVersion, "reason": "Versi basi"}), 409, "versi aktif basi")
	if p := admin.do("POST", "/workflows/"+wfID+"/migrations/preview", body); p.Body["ready"] != true {
		t.Fatalf("pratinjau dengan pemetaan: %v", p.Body)
	}
	hr := e.client().mustLogin("hr.ops@arunika.example")
	expect(t, hr.do("POST", "/workflows/"+wfID+"/migrations", body), 200, "HR (workflows.publish) memindahkan")

	d := rec.do("GET", "/applications/"+app, nil)
	if d.Body["stage"].(map[string]any)["name"] != "Seleksi Berkas" {
		t.Fatalf("tahap setelah pindah: %v", d.Body["stage"])
	}
	if d2 := rec.do("GET", "/applications/"+app2, nil); d2.Body["stage"].(map[string]any)["key"] != "ujian" {
		t.Fatalf("tahap ujian harus tetap: %v", d2.Body["stage"])
	}
	if n := e.count(`SELECT count(*) FROM transition_history WHERE trigger = 'migration' AND application_id IN ($1, $2)`, app, app2); n != 2 {
		t.Fatalf("riwayat migrasi: %d", n)
	}
	// Workflow versi baru berlaku untuk tindakan berikutnya.
	expect(t, rec.do("POST", "/applications/"+app+"/transitions", map[string]any{"kind": "manual", "to": "ujian", "reason": "", "expectedVersion": d.Body["version"]}), 200, "transisi di versi baru")
	if n := e.count(`SELECT count(*) FROM job_applications WHERE workflow_version_id::text = $1 AND status = 'active'`, oldVersion); n != 0 {
		t.Fatalf("masih ada lamaran aktif di versi lama: %d", n)
	}
}
