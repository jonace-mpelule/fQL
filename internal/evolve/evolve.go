// Package evolve compares effective schema snapshots without migrating documents.
package evolve

import (
	"encoding/json"
	"fmt"
	"github.com/jonace-mpelule/fQL/internal/schema"
	"io"
	"os"
	"sort"
)

// Canonical removes source-only metadata from the persisted comparison baseline.
func Canonical(s schema.Schema) schema.Schema {
	var fields func([]schema.Field) []schema.Field
	fields = func(in []schema.Field) []schema.Field {
		out := make([]schema.Field, len(in))
		for i, f := range in {
			f.Position = schema.Position{}
			f.Modifiers = nil
			f.ObjectFields = fields(f.ObjectFields)
			if f.ElementField != nil {
				x := fields([]schema.Field{*f.ElementField})[0]
				f.ElementField = &x
			}
			if f.Default != nil {
				x := *f.Default
				x.Position = schema.Position{}
				f.Default = &x
			}
			f.Allowed = append([]schema.Value(nil), f.Allowed...)
			for j := range f.Allowed {
				f.Allowed[j].Position = schema.Position{}
			}
			out[i] = f
		}
		return out
	}
	s.Models = append([]schema.Model(nil), s.Models...)
	for i := range s.Models {
		s.Models[i].Position = schema.Position{}
		s.Models[i].Fields = fields(s.Models[i].Fields)
		s.Models[i].Policies = append([]schema.Policy(nil), s.Models[i].Policies...)
		for j := range s.Models[i].Policies {
			s.Models[i].Policies[j].Position = schema.Position{}
		}
		sort.Slice(s.Models[i].Policies, func(a, b int) bool { return s.Models[i].Policies[a].Operation < s.Models[i].Policies[b].Operation })
	}
	s.Objects = append([]schema.Object(nil), s.Objects...)
	for i := range s.Objects {
		s.Objects[i].Position = schema.Position{}
		s.Objects[i].Fields = fields(s.Objects[i].Fields)
	}
	s.Enums = append([]schema.Enum(nil), s.Enums...)
	for i := range s.Enums {
		s.Enums[i].Position = schema.Position{}
	}
	return s
}

type Change struct {
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Risk    string `json:"risk"`
	Message string `json:"message"`
}
type Report struct {
	Changes  []Change `json:"changes"`
	Breaking bool     `json:"breaking"`
	Review   bool     `json:"review"`
}

func signature(v any) string { b, _ := json.Marshal(v); return string(b) }
func Compare(before, after schema.Schema) Report {
	before = Canonical(before)
	after = Canonical(after)
	report := Report{Changes: []Change{}}
	add := func(path, kind, risk, message string) {
		report.Changes = append(report.Changes, Change{path, kind, risk, message})
		if risk == "breaking" {
			report.Breaking = true
		}
		if risk == "review" {
			report.Review = true
		}
	}
	old, new := map[string]schema.Model{}, map[string]schema.Model{}
	for _, m := range before.Models {
		old[m.Name] = m
	}
	for _, m := range after.Models {
		new[m.Name] = m
	}
	var compareFields func(string, []schema.Field, []schema.Field)
	compareFields = func(prefix string, a, b []schema.Field) {
		x, y := map[string]schema.Field{}, map[string]schema.Field{}
		for _, f := range a {
			x[f.Name] = f
		}
		for _, f := range b {
			y[f.Name] = f
		}
		for name, old := range x {
			current, ok := y[name]
			path := prefix + "." + name
			if !ok {
				add(path, "removed", "breaking", "Stored documents and existing readers may still depend on this field")
				continue
			}
			oa, ob := old.ObjectFields, current.ObjectFields
			old.ObjectFields = nil
			current.ObjectFields = nil
			if old.ElementField != nil && current.ElementField != nil {
				compareFields(path+"[]", old.ElementField.ObjectFields, current.ElementField.ObjectFields)
				left, right := *old.ElementField, *current.ElementField
				left.ObjectFields = nil
				right.ObjectFields = nil
				old.ElementField = &left
				current.ElementField = &right
			}
			if signature(old) != signature(current) {
				add(path, "changed", "breaking", "Field type, presence, constraints, defaults, or write semantics changed; audit data and coordinate client rollout")
			}
			compareFields(path, oa, ob)
		}
		for name, f := range y {
			if _, ok := x[name]; !ok {
				risk := "review"
				if !f.Optional {
					risk = "breaking"
				}
				add(prefix+"."+name, "added", risk, "Required fields need existing-data handling; even optional additions can affect older strict readers")
			}
		}
	}
	for name, m := range old {
		n, ok := new[name]
		if !ok {
			add(name, "removed", "breaking", "Model/collection is no longer generated; stored data is not deleted")
			continue
		}
		if m.Collection != n.Collection {
			add(name, "collection", "breaking", "Collection mapping changed; documents are not moved")
		}
		compareFields(name, m.Fields, n.Fields)
		if signature(m.Policies) != signature(n.Policies) {
			add(name+".access", "changed", "review", "Authorization changed; review existing and new clients before deployment")
		}
		if signature(m.Indexes) != signature(n.Indexes) {
			add(name+".indexes", "changed", "review", "Index configuration changed; deployment and readiness are separate")
		}
	}
	for name := range new {
		if _, ok := old[name]; !ok {
			add(name, "added", "compatible", "New model/collection")
		}
	}
	if signature(before.Enums) != signature(after.Enums) {
		add("enums", "changed", "review", "Enum definitions changed; older clients can reject new stored members")
	}
	if signature(before.Objects) != signature(after.Objects) {
		add("objects", "changed", "review", "Object definitions changed; inspect affected nested fields")
	}
	sort.Slice(report.Changes, func(i, j int) bool {
		if report.Changes[i].Path == report.Changes[j].Path {
			return report.Changes[i].Kind < report.Changes[j].Kind
		}
		return report.Changes[i].Path < report.Changes[j].Path
	})
	return report
}
func Baseline(path string) (schema.Schema, error) {
	f, e := os.Open(path)
	if e != nil {
		return schema.Schema{}, fmt.Errorf("no comparison baseline; generate once or use --against PROJECT")
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 16<<20+1))
	if e != nil || len(b) > 16<<20 {
		return schema.Schema{}, fmt.Errorf("invalid or oversized baseline")
	}
	var m struct {
		Format   string         `json:"format"`
		Baseline *schema.Schema `json:"baseline"`
	}
	if e = json.Unmarshal(b, &m); e != nil || m.Format != "2" || m.Baseline == nil {
		return schema.Schema{}, fmt.Errorf("lockfile has no supported schema baseline; generate once or use --against PROJECT")
	}
	if len(m.Baseline.Models) > 100 || len(m.Baseline.Enums) > 64 || len(m.Baseline.Objects) > 64 {
		return schema.Schema{}, fmt.Errorf("baseline exceeds schema budgets")
	}
	nodes := 0
	var bounded func([]schema.Field, int) bool
	bounded = func(fields []schema.Field, depth int) bool {
		if depth > 8 || len(fields) > 32 {
			return false
		}
		for _, f := range fields {
			nodes++
			if nodes > 20000 || !bounded(f.ObjectFields, depth+1) {
				return false
			}
			if f.ElementField != nil && !bounded([]schema.Field{*f.ElementField}, depth+1) {
				return false
			}
		}
		return true
	}
	for _, model := range m.Baseline.Models {
		if !bounded(model.Fields, 0) {
			return schema.Schema{}, fmt.Errorf("baseline exceeds nesting/value budgets")
		}
	}
	for _, object := range m.Baseline.Objects {
		if !bounded(object.Fields, 0) {
			return schema.Schema{}, fmt.Errorf("baseline exceeds nesting/value budgets")
		}
	}
	return *m.Baseline, nil
}
