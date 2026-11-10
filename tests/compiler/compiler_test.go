package compiler_test

import (
	"github.com/jonace-mpelule/fQL/internal/project"
	"github.com/jonace-mpelule/fQL/internal/schema"
	"github.com/jonace-mpelule/fQL/internal/syntax"
	"github.com/jonace-mpelule/fQL/internal/validate"
	"strings"
	"testing"
)

func compile(source string) (schema.Schema, error) {
	s, e := syntax.Parse("test.fQL", []byte(source))
	if e != nil {
		return s, e
	}
	return validate.Schema(s)
}
func TestSupportedFields(t *testing.T) {
	s, e := compile(`schema "0.1"
model Sample { collection = "samples" fields {
 label string.required().minLength(1).maxLength(20).oneOf("one", "two").default("one")
 enabled bool.required().default(false)
 count int.required().min(-10).max(10).default(0)
 ratio float.optional().nullable().min(0).max(1).default(0.5)
 birthday date.required().default("2000-02-29")
 createdAt timestamp.required().immutable().default("2026-01-01T00:00:00.123456Z")
} access { read = authenticated create = deny update = deny delete = deny }
indexes { index(label.asc, count.desc) }
}`)
	if e != nil {
		t.Fatal(e)
	}
	if len(s.Models[0].Fields) != 6 || !s.Models[0].Fields[3].Optional || !s.Models[0].Fields[3].Nullable {
		t.Fatal("field semantics lost")
	}
}
func TestRejectInvalidSchemas(t *testing.T) {
	cases := map[string]string{
		"rounded integer bound":  `x int.required().min(9007199254740990.9)`,
		"nondecimal":             `x int.required().default(0xFF)`,
		"leading zero":           `x int.required().default(012)`,
		"UTC overflow":           `x timestamp.required().default("9999-12-31T23:59:59-01:00")`,
		"unsupported type":       `x reference.required()`,
		"implicit presence":      `x string`,
		"contradictory presence": `x string.required().optional()`,
		"duplicate modifier":     `x string.required().required()`,
		"wrong default":          `x date.required().default("2025-02-29")`,
		"year zero":              `x date.required().default("0000-01-01")`,
		"wrong literal":          `x bool.required().default("true")`,
		"unsafe integer":         `x int.required().default(9007199254740992)`,
		"fractional integer":     `x int.required().default(1.1)`,
		"nonfinite":              `x float.required().default(1e999)`,
		"unsupported modifier":   `x string.required().unique()`,
		"length on number":       `x int.required().minLength(2)`,
		"reversed bounds":        `x int.required().min(5).max(1)`,
		"outside allowed set":    `x string.required().oneOf("a").default("b")`,
		"invalid allowed value":  `x bool.required().oneOf(1)`,
		"duplicate field":        `x bool.required() x bool.required()`,
		"reserved field":         `toString string.required()`,
		"null on nonnullable":    `x string.required().default(null)`,
		"nanosecond timestamp":   `x timestamp.required().default("2026-01-01T00:00:00.123456789Z")`,
		"invalid default bound":  `x int.required().min(5).default(1)`,
	}
	for name, fields := range cases {
		t.Run(name, func(t *testing.T) {
			_, e := compile(`schema "0.1" model Sample { collection = "samples" fields { ` + fields + ` } }`)
			if e == nil {
				t.Fatal("expected diagnostic")
			}
			if !strings.Contains(e.Error(), "test.fQL:") {
				t.Fatalf("missing source location: %v", e)
			}
		})
	}
	for name, source := range map[string]string{
		"version":              `schema "9" model User { collection="users" fields { name string.required() } }`,
		"duplicate model":      project.UserSchema + strings.TrimPrefix(project.UserSchema, `schema "0.1"`),
		"duplicate collection": project.UserSchema + ` model Other { collection="users" fields { name string.required() } }`,
		"mutable owner":        `schema "0.1" model User { collection="users" fields { owner string.required() } access { read=owner(owner) } }`,
		"unknown reference":    `schema "0.1" model User { collection="users" fields { name string.required() } access { read=owner(missing) } }`,
		"duplicate policy":     `schema "0.1" model User { collection="users" fields { name string.required() } access { read=deny read=authenticated } }`,
		"bad index":            `schema "0.1" model User { collection="users" fields { name string.required() } indexes { index(missing.asc, name.desc) } }`,
		"collection path":      `schema "0.1" model User { collection="users/sub" fields { name string.required() } }`,
		"generated collision":  `schema "0.1" model User { collection="users" fields { name string.required() } } model UserFields { collection="other" fields { name string.required() } }`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, e := compile(source); e == nil {
				t.Fatal("expected error")
			}
		})
	}
}
func TestParserLocationsAndBounds(t *testing.T) {
	_, e := syntax.Parse("broken.fQL", []byte("schema \"0.1\"\nmodel User {\nfields { name string.required( } }"))
	if e == nil || !strings.Contains(e.Error(), "broken.fQL:3:") {
		t.Fatalf("expected source location, got %v", e)
	}
	if _, e := syntax.Parse("large.fQL", []byte(strings.Repeat(" ", syntax.MaxSourceBytes+1))); e == nil {
		t.Fatal("expected size limit")
	}
}
func FuzzParser(f *testing.F) {
	f.Add(project.UserSchema)
	f.Add(project.MessageSchema)
	f.Add(`schema "0.1" model`)
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > syntax.MaxSourceBytes {
			return
		}
		parsed, e := syntax.Parse("fuzz.fQL", []byte(s))
		if e == nil {
			_, _ = validate.Schema(parsed)
		}
	})
}
