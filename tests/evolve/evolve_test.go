package evolve_test

import (
	"github.com/jonace-mpelule/fQL/internal/evolve"
	"github.com/jonace-mpelule/fQL/internal/schema"
	"github.com/jonace-mpelule/fQL/internal/syntax"
	"github.com/jonace-mpelule/fQL/internal/validate"
	"strings"
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

func TestPolicyDiffsIgnoreLocationsAndReportTrustedChanges(t *testing.T) {
	source := `schema "0.1" claims {deviceId string} policy device(value string) {auth.claims.deviceId == value} model Example {collection="examples" fields{deviceId string.required()} access{get = device(existing.deviceId)}}`
	a := parsed(t, source)
	b := parsed(t, "// moved\n"+source)
	if changes := evolve.Compare(a, b).Changes; len(changes) != 0 {
		t.Fatalf("locations changed baseline: %+v", changes)
	}
	b = parsed(t, strings.Replace(source, "== value", "!= value", 1))
	if r := evolve.Compare(a, b); !r.Review {
		t.Fatal("named policy change missing authorization review")
	}
	b = parsed(t, strings.Replace(source, "claims {deviceId string}", "claims {deviceId string generation int}", 1))
	if r := evolve.Compare(a, b); !r.Review {
		t.Fatal("claim change missing authorization review")
	}
}
