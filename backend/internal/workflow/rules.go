package workflow

import (
	"fmt"
	"slices"
)

// Rule adalah AST deklaratif. Node gabungan memakai All/Any; node daun memakai Field/Op/Value.
// Tidak ada eval, SQL, URL callback, atau scripting.
type Rule struct {
	All   []Rule `json:"all,omitempty"`
	Any   []Rule `json:"any,omitempty"`
	Field string `json:"field,omitempty"`
	Op    string `json:"op,omitempty"`
	Value any    `json:"value,omitempty"`
}

type fieldSpec struct {
	Type   string   `json:"type"` // number | string | bool
	Values []string `json:"values,omitempty"`
}

// Fields adalah allowlist field fakta yang boleh dipakai rule.
var Fields = map[string]fieldSpec{
	"assessment.score":          {Type: "number"},
	"assessment.status":         {Type: "string", Values: []string{"pending", "graded", "released"}},
	"assessment.passed":         {Type: "bool"},
	"assessment.pending_manual": {Type: "bool"},
	"approval.outcome":          {Type: "string", Values: []string{"approved", "rejected"}},
}

var opsByType = map[string][]string{
	"number": {"eq", "neq", "gt", "gte", "lt", "lte"},
	"string": {"eq", "neq", "in"},
	"bool":   {"eq", "neq"},
}

// Facts: nilai yang tidak ada diperlakukan null dan tidak memenuhi syarat apa pun.
type Facts map[string]any

func ValidateRule(r Rule) []Issue {
	var out []Issue
	nodes := 0
	var walk func(r Rule, path string, depth int)
	walk = func(r Rule, path string, depth int) {
		nodes++
		if depth > MaxRuleDepth {
			out = append(out, Issue{Path: path, Code: "rule_depth", Message: fmt.Sprintf("Kedalaman syarat maksimal %d.", MaxRuleDepth)})
			return
		}
		kinds := 0
		if len(r.All) > 0 {
			kinds++
		}
		if len(r.Any) > 0 {
			kinds++
		}
		if r.Field != "" {
			kinds++
		}
		if kinds != 1 {
			out = append(out, Issue{Path: path, Code: "rule_shape", Message: "Setiap syarat harus berupa 'all', 'any', atau perbandingan field."})
			return
		}
		for i, c := range r.All {
			walk(c, fmt.Sprintf("%s.all[%d]", path, i), depth+1)
		}
		for i, c := range r.Any {
			walk(c, fmt.Sprintf("%s.any[%d]", path, i), depth+1)
		}
		if r.Field == "" {
			return
		}
		spec, ok := Fields[r.Field]
		if !ok {
			out = append(out, Issue{Path: path + ".field", Code: "rule_field", Message: fmt.Sprintf("Field %q tidak diizinkan.", r.Field)})
			return
		}
		if !slices.Contains(opsByType[spec.Type], r.Op) {
			out = append(out, Issue{Path: path + ".op", Code: "rule_op", Message: fmt.Sprintf("Operator %q tidak berlaku untuk %s.", r.Op, r.Field)})
			return
		}
		if !valueMatches(spec, r.Op, r.Value) {
			out = append(out, Issue{Path: path + ".value", Code: "rule_value", Message: fmt.Sprintf("Nilai untuk %s tidak sesuai tipe %s.", r.Field, spec.Type)})
		}
	}
	walk(r, "", 1)
	if nodes > MaxRuleNodes {
		out = append(out, Issue{Path: "", Code: "rule_size", Message: fmt.Sprintf("Syarat maksimal %d node.", MaxRuleNodes)})
	}
	return out
}

func valueMatches(spec fieldSpec, op string, v any) bool {
	switch spec.Type {
	case "number":
		_, ok := toFloat(v)
		return ok
	case "bool":
		_, ok := v.(bool)
		return ok
	case "string":
		check := func(s any) bool {
			str, ok := s.(string)
			return ok && (len(spec.Values) == 0 || slices.Contains(spec.Values, str))
		}
		if op == "in" {
			arr, ok := v.([]any)
			if !ok || len(arr) == 0 {
				return false
			}
			for _, x := range arr {
				if !check(x) {
					return false
				}
			}
			return true
		}
		return check(v)
	}
	return false
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// Eval mengevaluasi rule terhadap fakta. Fakta null/absen = tidak memenuhi syarat.
func Eval(r Rule, f Facts) bool {
	if len(r.All) > 0 {
		for _, c := range r.All {
			if !Eval(c, f) {
				return false
			}
		}
		return true
	}
	if len(r.Any) > 0 {
		for _, c := range r.Any {
			if Eval(c, f) {
				return true
			}
		}
		return false
	}
	v, ok := f[r.Field]
	if !ok || v == nil {
		return false
	}
	switch Fields[r.Field].Type {
	case "number":
		a, ok1 := toFloat(v)
		b, ok2 := toFloat(r.Value)
		if !ok1 || !ok2 {
			return false
		}
		switch r.Op {
		case "eq":
			return a == b
		case "neq":
			return a != b
		case "gt":
			return a > b
		case "gte":
			return a >= b
		case "lt":
			return a < b
		case "lte":
			return a <= b
		}
	case "string":
		s, ok := v.(string)
		if !ok {
			return false
		}
		switch r.Op {
		case "eq":
			return s == r.Value
		case "neq":
			return s != r.Value
		case "in":
			arr, _ := r.Value.([]any)
			for _, x := range arr {
				if x == s {
					return true
				}
			}
			return false
		}
	case "bool":
		b, ok := v.(bool)
		if !ok {
			return false
		}
		switch r.Op {
		case "eq":
			return b == r.Value
		case "neq":
			return b != r.Value
		}
	}
	return false
}

// Decide memilih edge pertama yang syaratnya terpenuhi, lalu default. Dipakai engine dan simulasi.
func Decide(d *Definition, from, trigger, event string, f Facts) (*Edge, string) {
	evals := 0
	for _, e := range d.OutEdges(from, trigger) {
		if event != "" && e.Event != event {
			continue
		}
		evals++
		if evals > MaxEvalsPerEvent {
			return nil, "batas evaluasi tercapai"
		}
		if e.Default {
			e := e
			return &e, "default"
		}
		if e.When == nil || Eval(*e.When, f) {
			e := e
			return &e, "rule"
		}
	}
	return nil, "tidak ada transisi yang memenuhi syarat"
}
