package validate

import (
	"fmt"
	"github.com/jonace-mpelule/fQL/internal/schema"
	"strings"
)

func resolveNames(s schema.Schema) (schema.Schema, error) {
	if len(s.Enums) > 64 || len(s.Objects) > 64 {
		return s, fmt.Errorf("at most 64 enums and objects are supported")
	}
	enums := map[string][]string{}
	objects := map[string]schema.Object{}
	names := map[string]bool{}
	for _, e := range s.Enums {
		if len(e.Name) > 64 || !modelName.MatchString(e.Name) || reserved[e.Name] || strings.HasPrefix(e.Name, "Fql") || names[e.Name] || len(e.Values) == 0 || len(e.Values) > 32 {
			return s, fail(e.Position, "enum requires a unique nonreserved type name and 1..32 members")
		}
		names[e.Name] = true
		values := map[string]bool{}
		for _, v := range e.Values {
			if !fieldName.MatchString(v) || len(v) > 64 || (reserved[v] || v == "name" || v == "index" || v == "values") || values[v] {
				return s, fail(e.Position, "enum members must be unique nonreserved lowerCamelCase identifiers")
			}
			values[v] = true
		}
		enums[e.Name] = e.Values
	}
	for _, o := range s.Objects {
		if len(o.Name) > 64 || !modelName.MatchString(o.Name) || reserved[o.Name] || strings.HasPrefix(o.Name, "Fql") || names[o.Name] || len(o.Fields) == 0 {
			return s, fail(o.Position, "object requires a unique nonreserved type name and fields")
		}
		names[o.Name] = true
		objects[o.Name] = o
	}
	for _, m := range s.Models {
		if names[m.Name] {
			return s, fail(m.Position, "duplicate type name")
		}
	}
	var resolve func(schema.Field, int, map[string]bool) (schema.Field, error)
	resolve = func(f schema.Field, depth int, stack map[string]bool) (schema.Field, error) {
		if depth > 4 {
			return f, fail(f.Position, "nested types exceed maximum depth 4")
		}
		if values, ok := enums[f.Type]; ok {
			f.EnumValues = values
			return f, nil
		}
		if o, ok := objects[f.Type]; ok {
			if stack[o.Name] {
				return f, fail(f.Position, "cyclic object reference")
			}
			next := map[string]bool{}
			for k, v := range stack {
				next[k] = v
			}
			next[o.Name] = true
			f.ObjectFields = nil
			for _, child := range o.Fields {
				c, e := resolve(child, depth+1, next)
				if e != nil {
					return f, e
				}
				f.ObjectFields = append(f.ObjectFields, c)
			}
			return f, nil
		}
		if f.Type == "list" {
			if f.Element == "list" || f.Element == "" {
				return f, fail(f.Position, "list elements must be scalar, enum, or named object")
			}
			child, e := resolve(schema.Field{Name: f.Name, Type: f.Element, Position: f.Position}, depth+1, stack)
			if e != nil {
				return f, e
			}
			switch child.Type {
			case "string", "bool", "int", "float", "date", "timestamp":
			default:
				if len(child.EnumValues) == 0 && len(child.ObjectFields) == 0 {
					return f, fail(f.Position, "unresolved list element type")
				}
			}
			f.ElementField = &child
			return f, nil
		}
		switch f.Type {
		case "string", "bool", "int", "float", "date", "timestamp":
		default:
			return f, fail(f.Position, "unresolved or unsupported type "+f.Type)
		}
		return f, nil
	}
	for i := range s.Objects {
		for j, f := range s.Objects[i].Fields {
			r, e := resolve(f, 0, map[string]bool{s.Objects[i].Name: true})
			if e != nil {
				return s, e
			}
			s.Objects[i].Fields[j] = r
		}
	}
	for i := range s.Models {
		for j, f := range s.Models[i].Fields {
			r, e := resolve(f, 0, map[string]bool{})
			if e != nil {
				return s, e
			}
			s.Models[i].Fields[j] = r
		}
	}
	// Nested server/immutable operations would need path-specific update semantics.
	for _, o := range s.Objects {
		for _, f := range o.Fields {
			if f.Server != "" || f.Immutable {
				return s, fail(f.Position, "managed/immutable fields are supported only at document root; mark the containing field immutable")
			}
		}
	}
	for _, m := range s.Models {
		budget := 0
		cost := 0
		for _, f := range m.Fields {
			budget += fieldBudget(f)
			cost += ruleCost(f)
		}
		if budget > 128 || cost > 700 {
			return s, fail(m.Position, "expanded Rules validation exceeds the supported value/expression budget; reduce fields/list bounds or nesting")
		}
	}
	return s, nil
}
func fieldBudget(f schema.Field) int {
	n := 1
	for _, c := range f.ObjectFields {
		n += fieldBudget(c)
	}
	if f.ElementField != nil && f.MaxItems != nil {
		n += *f.MaxItems * fieldBudget(*f.ElementField)
	}
	return n
}

func ruleCost(f schema.Field) int {
	n := 14 + len(f.Allowed)*2 + len(f.EnumValues)*2
	if f.Type == "date" {
		n += 40
	}
	for _, c := range f.ObjectFields {
		n += ruleCost(c)
	}
	if f.ElementField != nil && f.MaxItems != nil {
		n += *f.MaxItems * (4 + ruleCost(*f.ElementField))
	}
	return n
}
