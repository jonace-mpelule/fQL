package generate_test

import (
	"github.com/jonace-mpelule/fQL/internal/generate"
	"github.com/jonace-mpelule/fQL/internal/syntax"
	"github.com/jonace-mpelule/fQL/internal/validate"
	"strings"
	"testing"
)

func TestLiteralEscapingAndPolicies(t *testing.T) {
	source := `schema "0.1" model Sample { collection = "samples" fields {
 label string.required().default("\a$quote\"\n")
 birthday date.required().default("2000-02-29")
 createdAt timestamp.required().default("2026-01-01T00:00:00.123456Z")
 rate float.required().default(1e-5).oneOf(1e-5)
 maybe string.required().nullable().default(null)
 } access { read = authenticated create = deny } }`
	s, e := syntax.Parse("test.fQL", []byte(source))
	if e != nil {
		t.Fatal(e)
	}
	s, e = validate.Schema(s)
	if e != nil {
		t.Fatal(e)
	}
	dart := string(generate.Dart(s))
	for _, want := range []string{`\u0007\$quote\"\n`, `FqlDate.parse("2000-02-29")`, `Timestamp.fromDate(DateTime.parse("2026-01-01T00:00:00.123456Z").toUtc())`, `defaultValue:1e-05`, `defaultValue:null`} {
		if !strings.Contains(dart, want) {
			t.Errorf("generated Dart missing %q", want)
		}
	}
	rules, _, e := generate.Firestore(s)
	if e != nil {
		t.Fatal(e)
	}
	for _, want := range []string{"allow read: if request.auth != null;", "allow create: if (false)", "allow update: if (false)", "allow delete: if false;", "d[\"rate\"] in [0.00001]"} {
		if !strings.Contains(string(rules), want) {
			t.Errorf("generated rules missing %q", want)
		}
	}
}
