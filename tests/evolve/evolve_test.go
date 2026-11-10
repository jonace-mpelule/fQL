package evolve_test

import (
	"github.com/jonace-mpelule/fQL/internal/evolve"
	"github.com/jonace-mpelule/fQL/internal/schema"
	"github.com/jonace-mpelule/fQL/internal/syntax"
	"github.com/jonace-mpelule/fQL/internal/validate"
	"testing"
)

func parsed(t *testing.T, source string) schema.Schema {
	t.Helper()
	s, e := syntax.Parse("source.fQL", []byte(source))
	if e != nil {
		t.Fatal(e)
	}
	s, e = validate.Schema(s)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestDiffIgnoresLocationsAndFlagsBreakingChanges(t *testing.T) {
	a := parsed(t, `schema "0.1" model Example{collection="examples" fields{label string.required()}}`)
	b := parsed(t, "// moved\n"+`schema "0.1" model Example{collection="examples" fields{label string.required()}}`)
	if len(evolve.Compare(a, b).Changes) != 0 {
		t.Fatal("source-only edits changed schema")
	}
	b = parsed(t, `schema "0.1" model Example{collection="examples" fields{label string.required().maxLength(10) extra bool.optional()}}`)
	r := evolve.Compare(a, b)
	if !r.Breaking || !r.Review || len(r.Changes) != 2 {
		t.Fatalf("unexpected report: %+v", r)
	}
}
