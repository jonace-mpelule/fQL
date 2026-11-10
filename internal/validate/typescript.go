package validate

import (
	"github.com/jonace-mpelule/fQL/internal/schema"
	"strings"
)

// TypeScript checks target-specific names without changing Dart compatibility.
func TypeScript(s schema.Schema) error {
	reserved := map[string]bool{}
	for _, name := range strings.Fields("Error Promise WeakMap Set Array Number Object String Boolean JSON Math Symbol Infinity NaN Record Readonly Timestamp") {
		reserved[name] = true
	}
	check := func(name string, pos schema.Position) error {
		if reserved[name] {
			return fail(pos, "name conflicts with the TypeScript runtime; rename "+name)
		}
		return nil
	}
	fields := func(fs []schema.Field) error {
		for _, f := range fs {
			if f.Name == "constructor" {
				return fail(f.Position, "constructor is reserved by TypeScript classes; rename this field")
			}
		}
		return nil
	}
	for _, e := range s.Enums {
		if err := check(e.Name, e.Position); err != nil {
			return err
		}
	}
	for _, o := range s.Objects {
		if err := check(o.Name, o.Position); err != nil {
			return err
		}
		if err := fields(o.Fields); err != nil {
			return err
		}
	}
	for _, m := range s.Models {
		if err := check(m.Name, m.Position); err != nil {
			return err
		}
		if err := fields(m.Fields); err != nil {
			return err
		}
		if m.Collection == "constructor" {
			return fail(m.Position, "constructor is reserved by TypeScript classes; rename this collection")
		}
	}
	return nil
}
