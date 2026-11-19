package workflow

import (
	"encoding/json"
	"testing"
)

func codes(is []Issue) map[string]bool {
	m := map[string]bool{}
	for _, i := range is {
		m[i.Code] = true
	}
	return m
}

func TestDefaultDefinitionValid(t *testing.T) {
	if is := Validate(DefaultDefinition()); len(is) > 0 {
		t.Fatalf("definisi default harus valid: %+v", is)
	}
}

func TestValidateRejectsBrokenGraphs(t *testing.T) {
	cases := map[string]func(d *Definition){
		"cycle": func(d *Definition) {
			d.Edges = append(d.Edges, Edge{ID: "x", From: "interview", To: "screening", Trigger: "manual", Label: "balik"})
		},
		"target_missing": func(d *Definition) {
			d.Edges = append(d.Edges, Edge{ID: "x", From: "screening", To: "hilang", Trigger: "manual"})
		},
		"missing_fallback": func(d *Definition) {
			var es []Edge
			for _, e := range d.Edges {
				if e.ID != "e4" {
					es = append(es, e)
				}
			}
			d.Edges = es
		},
		"unreachable": func(d *Definition) {
			d.Stages = append(d.Stages, Stage{Key: "yatim", Name: "Yatim", Type: "screening"})
			d.Edges = append(d.Edges, Edge{ID: "x", From: "yatim", To: "interview", Trigger: "manual"})
		},
		"terminal_outgoing": func(d *Definition) {
			d.Edges = append(d.Edges, Edge{ID: "x", From: "diterima", To: "interview", Trigger: "manual"})
		},
		"approval_roles_empty": func(d *Definition) {
			for i := range d.Stages {
				if d.Stages[i].Type == "approval" {
					d.Stages[i].Approval = &ApprovalPolicy{Quorum: 1}
				}
			}
		},
		"rule_field": func(d *Definition) {
			d.Edges[1].When = &Rule{Field: "candidate.nik", Op: "eq", Value: "x"}
		},
		"start_missing": func(d *Definition) { d.Start = "tidak_ada" },
	}
	for want, mutate := range cases {
		d := DefaultDefinition()
		mutate(&d)
		got := codes(Validate(d))
		if !got[want] {
			t.Errorf("%s: kode tidak ditemukan, temuan = %v", want, got)
		}
	}
}

func TestEvalNullIsNotSatisfied(t *testing.T) {
	r := Rule{Field: "assessment.score", Op: "lt", Value: float64(75)}
	if Eval(r, Facts{}) {
		t.Fatal("fakta absen tidak boleh memenuhi syarat")
	}
	if Eval(r, Facts{"assessment.score": nil}) {
		t.Fatal("null tidak boleh memenuhi syarat")
	}
	if !Eval(r, Facts{"assessment.score": 60.0}) {
		t.Fatal("60 < 75 harus benar")
	}
}

func TestRuleRoundTripJSON(t *testing.T) {
	d := DefaultDefinition()
	b, _ := json.Marshal(d)
	var back Definition
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if is := Validate(back); len(is) > 0 {
		t.Fatalf("hasil round-trip JSON harus tetap valid: %+v", is)
	}
	e, _ := Decide(&back, "ujian", "event", "assessment.released", Facts{"assessment.status": "released", "assessment.score": 80.0})
	if e == nil || e.To != "interview" {
		t.Fatalf("nilai 80 harus ke interview, dapat %+v", e)
	}
}

func TestSimulateBranches(t *testing.T) {
	d := DefaultDefinition()
	type tc struct {
		facts Facts
		want  string
	}
	for name, c := range map[string]tc{
		"lulus":  {Facts{"assessment.status": "released", "assessment.score": 80.0}, "interview"},
		"ambang": {Facts{"assessment.status": "released", "assessment.score": 75.0}, "interview"},
		"kurang": {Facts{"assessment.status": "released", "assessment.score": 60.0}, "ditolak"},
		"manual": {Facts{"assessment.status": "released", "assessment.pending_manual": true}, "review"},
		"kosong": {Facts{}, "ditolak"},
	} {
		res := Simulate(d, []SimStep{{Kind: "manual", To: "ujian"}, {Kind: "event", Event: "assessment.released", Facts: c.facts}})
		if got := res[len(res)-1].Stage; got != c.want {
			t.Errorf("%s: tahap akhir %s, ingin %s", name, got, c.want)
		}
	}
	res := Simulate(d, []SimStep{{Kind: "manual", To: "interview"}})
	if res[len(res)-1].OK {
		t.Fatal("screening -> interview tidak boleh tersedia secara manual")
	}
}
