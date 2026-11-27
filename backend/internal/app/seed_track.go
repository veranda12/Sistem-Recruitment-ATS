package app

import "ats/internal/workflow"

// trackTemplate: workflow default per track (BRD Lampiran B). Hanya memakai tipe tahap yang sudah ada.
type trackTemplate struct {
	key, name string
	def       workflow.Definition
}

// trackTemplates: alur Operator/Umum, Staff/Professional, dan MT S1/D4 tanpa tahap persetujuan Hiring Manager.
// Tahap ujian memakai paket tes-dasar; skor di bawah ambang menuju Ditolak, seperti workflow standar.
// MCU memakai tipe screening dan Offering memakai tipe decision. Engine tidak memberi perilaku khusus pada keduanya.
func trackTemplates() []trackTemplate {
	return []trackTemplate{
		{key: "track-operator", name: "Track Operator", def: workflow.Definition{
			Start: "administrasi",
			Stages: []workflow.Stage{
				trackStage("administrasi", "Administrasi & Histori", "screening"),
				trackAssessment("tes_dasar", "Tes Dasar"),
				trackStage("cek_fisik", "Cek Fisik (bila relevan)", "screening"),
				trackStage("interview", "Interview", "interview"),
				trackStage("mcu", "MCU", "screening"),
				trackStage("offering_pkwt", "Offering / PKWT", "decision"),
				trackTerminal("diterima", "Diterima", "hired"),
				trackTerminal("ditolak", "Ditolak", "rejected"),
			},
			Edges: append(trackUjian("tes_dasar", "cek_fisik", "ditolak"),
				trackManual("administrasi", "tes_dasar", "Lolos administrasi"),
				trackManual("cek_fisik", "interview", "Lolos cek fisik"),
				trackManual("interview", "mcu", "Lanjut ke MCU"),
				trackManual("mcu", "offering_pkwt", "MCU memenuhi syarat"),
				trackManual("offering_pkwt", "diterima", "Kandidat menerima offering"),
			),
		}},
		{key: "track-staff", name: "Track Staff", def: workflow.Definition{
			Start: "screening",
			Stages: []workflow.Stage{
				trackStage("screening", "Screening Kompetensi", "screening"),
				trackAssessment("tes_kompetensi", "Tes Kompetensi"),
				trackStage("interview_hr", "Interview HR", "interview"),
				trackStage("interview_user", "Interview User", "interview"),
				trackStage("mcu", "MCU (bila perlu)", "screening"),
				trackStage("offering", "Offering", "decision"),
				trackTerminal("diterima", "Diterima", "hired"),
				trackTerminal("ditolak", "Ditolak", "rejected"),
			},
			Edges: append(trackUjian("tes_kompetensi", "interview_hr", "ditolak"),
				trackManual("screening", "tes_kompetensi", "Lolos screening"),
				trackManual("interview_hr", "interview_user", "Lanjut ke interview User"),
				trackManual("interview_user", "mcu", "Lanjut ke MCU"),
				trackManual("mcu", "offering", "MCU memenuhi syarat"),
				trackManual("offering", "diterima", "Kandidat menerima offering"),
			),
		}},
		{key: "track-mt-s1-d4", name: "Track MT S1/D4", def: workflow.Definition{
			Start: "administrasi",
			Stages: []workflow.Stage{
				trackStage("administrasi", "Administrasi", "screening"),
				trackAssessment("tes_pc", "Tes PC"),
				trackStage("interview_hr", "Interview HR", "interview"),
				trackStage("interview_user", "Interview User / Dept. Head", "interview"),
				trackStage("interview_manajemen", "Interview Manajemen / Direksi", "interview"),
				trackStage("mcu", "MCU", "screening"),
				trackStage("offering", "Offering", "decision"),
				trackTerminal("diterima", "Diterima", "hired"),
				trackTerminal("ditolak", "Ditolak", "rejected"),
			},
			Edges: append(trackUjian("tes_pc", "interview_hr", "ditolak"),
				trackManual("administrasi", "tes_pc", "Lolos administrasi"),
				trackManual("interview_hr", "interview_user", "Lanjut ke interview User"),
				trackManual("interview_user", "interview_manajemen", "Lanjut ke interview Manajemen"),
				trackManual("interview_manajemen", "mcu", "Lanjut ke MCU"),
				trackManual("mcu", "offering", "MCU memenuhi syarat"),
				trackManual("offering", "diterima", "Kandidat menerima offering"),
			),
		}},
	}
}

func trackStage(key, name, typ string) workflow.Stage {
	return workflow.Stage{Key: key, Name: name, Type: typ}
}

func trackAssessment(key, name string) workflow.Stage {
	return workflow.Stage{Key: key, Name: name, Type: "assessment", AssessmentKey: "tes-dasar"}
}

func trackTerminal(key, name, outcome string) workflow.Stage {
	return workflow.Stage{Key: key, Name: name, Type: "terminal", Outcome: outcome}
}

// trackManual: transisi yang dipicu staf.
func trackManual(from, to, label string) workflow.Edge {
	return workflow.Edge{ID: from + "_" + to, From: from, To: to, Order: 1, Trigger: "manual", Label: label}
}

// trackUjian: hasil ujian released dengan skor >= 75 lanjut ke tahap berikutnya; selain itu ke tahap tolak (default).
func trackUjian(from, lanjut, tolak string) []workflow.Edge {
	lulus := &workflow.Rule{All: []workflow.Rule{
		{Field: "assessment.status", Op: "eq", Value: "released"},
		{Field: "assessment.score", Op: "gte", Value: float64(75)},
	}}
	return []workflow.Edge{
		{ID: from + "_lulus", From: from, To: lanjut, Order: 1, Trigger: "event", Event: "assessment.released", When: lulus, Label: "Nilai memenuhi ambang"},
		{ID: from + "_gagal", From: from, To: tolak, Order: 2, Trigger: "event", Event: "assessment.released", Default: true, Label: "Nilai di bawah ambang"},
	}
}
