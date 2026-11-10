package compiler_test

import "testing"

func TestNamedTypesAndManagedFields(t *testing.T) {
	s, e := compile(`schema "0.1"
enum State { open closed }
object Detail { fields { label string.required().minLength(1) } }
model Example { collection="examples" fields {
 state State.required().default("open")
 detail Detail.required()
 tags list<State>.optional().maxItems(4)
 createdAt timestamp.required().serverCreated()
 updatedAt timestamp.required().serverUpdated()
} }`)
	if e != nil {
		t.Fatal(e)
	}
	m := s.Models[0]
	if !m.Fields[3].Immutable || m.Fields[0].EnumValues[0] != "open" || m.Fields[1].ObjectFields[0].MinLength == nil || m.Fields[2].ElementField == nil {
		t.Fatal("normalization lost semantics")
	}
}
func TestRejectUnsupportedNestedSemantics(t *testing.T) {
	for _, source := range []string{
		`schema "0.1" model Example{collection="examples" fields{flag bool.required().default(-true)}}`,
		`schema "0.1" object Loop {fields{next Loop.optional()}} model Example{collection="examples" fields{loop Loop.required()}}`,
		`schema "0.1" model Example{collection="examples" fields{tags list<string>.required()}}`,
		`schema "0.1" model Example{collection="examples" fields{tags list<string>.required().maxItems(17)}}`,
		`schema "0.1" model Example{collection="examples" fields{at timestamp.optional().serverCreated()}}`,
		`schema "0.1" model Example{collection="examples" fields{at timestamp.required().serverCreated().serverUpdated()}}`,
		`schema "0.1" object Detail{fields{at timestamp.required().serverUpdated()}} model Example{collection="examples" fields{detail Detail.required()}}`,
		`schema "0.1" enum State{open closed} model Example{collection="examples" fields{state State.required().default("other")}}`,
		`schema "0.1" enum State{open open} model Example{collection="examples" fields{state State.required()}}`,
	} {
		if _, e := compile(source); e == nil {
			t.Fatalf("accepted invalid schema: %s", source)
		}
	}
}
