// Package workflow: graph tahapan berversi, rule DSL deklaratif, validasi, simulasi, dan engine transisi.
package workflow

import (
	"fmt"
	"regexp"
	"slices"
)

const (
	MaxStages        = 50
	MaxEdges         = 200
	MaxRuleDepth     = 4
	MaxRuleNodes     = 32
	MaxEvalsPerEvent = 100
)

var StageTypes = []string{"screening", "interview", "assessment", "approval", "decision", "terminal"}
var Triggers = []string{"manual", "event", "approval"}
var Events = []string{"assessment.released"}

type Definition struct {
	Start  string  `json:"start"`
	Stages []Stage `json:"stages"`
	Edges  []Edge  `json:"edges"`
}

type Stage struct {
	Key      string          `json:"key"`
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Outcome  string          `json:"outcome,omitempty"`  // hired | rejected, hanya untuk terminal
	Approval *ApprovalPolicy `json:"approval,omitempty"` // wajib untuk type approval
	// AssessmentKey menautkan stage assessment ke paket ujian; event hasil hanya berlaku bila cocok.
	AssessmentKey string `json:"assessmentKey,omitempty"`
}

type ApprovalPolicy struct {
	Roles             []string `json:"roles"`
	Quorum            int      `json:"quorum"`
	AllowSelfApproval bool     `json:"allowSelfApproval"`
}

type Edge struct {
	ID      string `json:"id"`
	From    string `json:"from"`
	To      string `json:"to"`
	Order   int    `json:"order"`
	Trigger string `json:"trigger"`
	Event   string `json:"event,omitempty"`
	When    *Rule  `json:"when,omitempty"`
	Default bool   `json:"default,omitempty"`
	Label   string `json:"label"`
}

func (d *Definition) Stage(key string) (Stage, bool) {
	for _, s := range d.Stages {
		if s.Key == key {
			return s, true
		}
	}
	return Stage{}, false
}

// RejectStage mengembalikan terminal penolakan. Aksi "Tolak" tersedia dari setiap stage non-terminal.
func (d *Definition) RejectStage() (Stage, bool) {
	for _, s := range d.Stages {
		if s.Type == "terminal" && s.Outcome == "rejected" {
			return s, true
		}
	}
	return Stage{}, false
}

// OutEdges terurut berdasarkan Order (default terakhir).
func (d *Definition) OutEdges(from, trigger string) []Edge {
	var out []Edge
	for _, e := range d.Edges {
		if e.From == from && e.Trigger == trigger {
			out = append(out, e)
		}
	}
	slices.SortStableFunc(out, func(a, b Edge) int {
		if a.Default != b.Default {
			if a.Default {
				return 1
			}
			return -1
		}
		return a.Order - b.Order
	})
	return out
}

// Issue adalah satu temuan validasi dengan path untuk disorot di editor.
type Issue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

var keyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,39}$`)

// Validate menolak siklus, target hilang, cabang tanpa fallback, node tak terjangkau,
// terminal dengan outgoing edge, approval tanpa peran, dan rule di luar allowlist.
func Validate(d Definition) []Issue {
	var is []Issue
	add := func(path, code, format string, a ...any) {
		is = append(is, Issue{Path: path, Code: code, Message: fmt.Sprintf(format, a...)})
	}
	if len(d.Stages) == 0 {
		add("stages", "empty", "Workflow belum memiliki tahapan.")
		return is
	}
	if len(d.Stages) > MaxStages {
		add("stages", "too_many", "Maksimal %d tahapan.", MaxStages)
	}
	if len(d.Edges) > MaxEdges {
		add("edges", "too_many", "Maksimal %d transisi.", MaxEdges)
	}
	stages := map[string]Stage{}
	rejectCount := 0
	for i, s := range d.Stages {
		p := fmt.Sprintf("stages[%d]", i)
		if !keyRe.MatchString(s.Key) {
			add(p+".key", "invalid_key", "Kunci tahap %q harus huruf kecil, angka, atau garis bawah.", s.Key)
		}
		if _, dup := stages[s.Key]; dup {
			add(p+".key", "duplicate", "Kunci tahap %q dipakai lebih dari sekali.", s.Key)
		}
		stages[s.Key] = s
		if len(s.Name) < 2 || len(s.Name) > 80 {
			add(p+".name", "invalid_name", "Nama tahap wajib 2 sampai 80 karakter.")
		}
		if !slices.Contains(StageTypes, s.Type) {
			add(p+".type", "invalid_type", "Jenis tahap %q tidak dikenal.", s.Type)
		}
		if s.Type == "terminal" {
			if s.Outcome != "hired" && s.Outcome != "rejected" {
				add(p+".outcome", "terminal_outcome", "Tahap akhir harus berhasil 'hired' atau 'rejected'.")
			}
			if s.Outcome == "rejected" {
				rejectCount++
			}
		} else if s.Outcome != "" {
			add(p+".outcome", "outcome_on_nonterminal", "Hanya tahap akhir yang boleh memiliki hasil.")
		}
		if s.Type == "approval" {
			if s.Approval == nil || len(s.Approval.Roles) == 0 {
				add(p+".approval", "approval_roles_empty", "Tahap persetujuan wajib menyebut peran penyetuju.")
			} else if s.Approval.Quorum < 1 || s.Approval.Quorum > 10 {
				add(p+".approval.quorum", "approval_quorum", "Kuorum persetujuan harus 1 sampai 10.")
			}
		}
		if s.Type == "assessment" && s.AssessmentKey == "" {
			add(p+".assessmentKey", "assessment_key", "Tahap ujian wajib menyebut kunci paket ujian.")
		}
	}
	if rejectCount != 1 {
		add("stages", "reject_terminal", "Workflow wajib memiliki tepat satu tahap akhir 'rejected'.")
	}
	start, ok := stages[d.Start]
	if !ok {
		add("start", "start_missing", "Tahap awal %q tidak ada.", d.Start)
	} else if start.Type == "terminal" {
		add("start", "start_terminal", "Tahap awal tidak boleh tahap akhir.")
	}

	outgoing := map[string][]Edge{}
	edgeIDs := map[string]bool{}
	for i, e := range d.Edges {
		p := fmt.Sprintf("edges[%d]", i)
		if e.ID == "" || edgeIDs[e.ID] {
			add(p+".id", "edge_id", "ID transisi wajib unik.")
		}
		edgeIDs[e.ID] = true
		from, okf := stages[e.From]
		if !okf {
			add(p+".from", "target_missing", "Tahap asal %q tidak ada.", e.From)
		}
		if _, okt := stages[e.To]; !okt {
			add(p+".to", "target_missing", "Tahap tujuan %q tidak ada.", e.To)
		}
		if okf && from.Type == "terminal" {
			add(p+".from", "terminal_outgoing", "Tahap akhir %q tidak boleh memiliki transisi keluar.", e.From)
		}
		if e.From == e.To {
			add(p, "self_loop", "Transisi tidak boleh kembali ke tahap yang sama.")
		}
		if !slices.Contains(Triggers, e.Trigger) {
			add(p+".trigger", "invalid_trigger", "Pemicu %q tidak dikenal.", e.Trigger)
		}
		if e.Trigger == "event" && !slices.Contains(Events, e.Event) {
			add(p+".event", "invalid_event", "Event %q tidak dikenal.", e.Event)
		}
		if e.Trigger == "event" && okf && from.Type != "assessment" {
			add(p+".event", "event_stage", "Event hasil ujian hanya berlaku dari tahap ujian.")
		}
		if e.Trigger == "approval" && okf && from.Type != "approval" {
			add(p+".trigger", "approval_stage", "Pemicu persetujuan hanya dari tahap persetujuan.")
		}
		if e.Default && e.When != nil {
			add(p+".when", "default_with_rule", "Transisi default tidak boleh memiliki syarat.")
		}
		if e.When != nil {
			for _, ri := range ValidateRule(*e.When) {
				add(p+".when"+ri.Path, ri.Code, "%s", ri.Message)
			}
		}
		if len(e.Label) > 80 {
			add(p+".label", "label_long", "Label transisi maksimal 80 karakter.")
		}
		outgoing[e.From] = append(outgoing[e.From], e)
	}

	// Setiap tahap non-terminal wajib punya jalan keluar; grup bersyarat wajib punya satu default.
	for _, s := range d.Stages {
		if s.Type == "terminal" {
			continue
		}
		if len(outgoing[s.Key]) == 0 {
			add("stages."+s.Key, "dead_end", "Tahap %q tidak memiliki transisi keluar.", s.Key)
		}
		groups := map[string][]Edge{}
		for _, e := range outgoing[s.Key] {
			groups[e.Trigger+"|"+e.Event] = append(groups[e.Trigger+"|"+e.Event], e)
		}
		for g, es := range groups {
			conditional, defaults := 0, 0
			for _, e := range es {
				if e.When != nil {
					conditional++
				}
				if e.Default {
					defaults++
				}
			}
			if defaults > 1 {
				add("stages."+s.Key, "multiple_default", "Tahap %q memiliki lebih dari satu transisi default untuk %s.", s.Key, g)
			}
			if conditional > 0 && defaults == 0 && es[0].Trigger != "manual" {
				add("stages."+s.Key, "missing_fallback", "Cabang bersyarat dari %q wajib memiliki transisi default.", s.Key)
			}
		}
		if s.Type == "approval" {
			has := false
			for _, e := range outgoing[s.Key] {
				has = has || e.Trigger == "approval"
			}
			if !has {
				add("stages."+s.Key, "approval_edges", "Tahap persetujuan %q wajib memiliki transisi pemicu persetujuan.", s.Key)
			}
		}
	}

	if ok {
		// Keterjangkauan dari start. Terminal 'rejected' selalu terjangkau via aksi Tolak.
		seen := map[string]bool{d.Start: true}
		queue := []string{d.Start}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, e := range outgoing[cur] {
				if !seen[e.To] {
					seen[e.To] = true
					queue = append(queue, e.To)
				}
			}
		}
		for _, s := range d.Stages {
			if !seen[s.Key] && !(s.Type == "terminal" && s.Outcome == "rejected") {
				add("stages."+s.Key, "unreachable", "Tahap %q tidak dapat dicapai dari tahap awal.", s.Key)
			}
		}
		// Deteksi siklus (DFS tiga warna).
		color := map[string]int{}
		var visit func(string) bool
		visit = func(n string) bool {
			color[n] = 1
			for _, e := range outgoing[n] {
				if color[e.To] == 1 {
					add("edges", "cycle", "Terdapat siklus melalui %q -> %q.", n, e.To)
					return true
				}
				if color[e.To] == 0 && visit(e.To) {
					return true
				}
			}
			color[n] = 2
			return false
		}
		for _, s := range d.Stages {
			if color[s.Key] == 0 && visit(s.Key) {
				break
			}
		}
	}
	return is
}
