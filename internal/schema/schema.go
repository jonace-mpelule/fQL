package schema

import "fmt"

// Position identifies a source token without embedding source contents in errors.
type Position struct {
	File                            string
	Line, Column, Offset, EndOffset int
}
type Diagnostic struct {
	Position Position
	Message  string
}

func (d Diagnostic) Error() string {
	return fmt.Sprintf("%s:%d:%d: %s", d.Position.File, d.Position.Line, d.Position.Column, d.Message)
}

type Value struct {
	Kind, Text string
	Position   Position
}
type Modifier struct {
	Name     string
	Args     []Value
	Position Position
}
type Field struct {
	Name, Type                    string
	Element                       string
	EnumValues                    []string
	ObjectFields                  []Field
	ElementField                  *Field
	MinItems, MaxItems            *int
	Server                        string
	Position                      Position
	Modifiers                     []Modifier
	Optional, Nullable, Immutable bool
	MinLength, MaxLength          *int
	Min, Max                      *float64
	Default                       *Value
	Allowed                       []Value
}
type IndexField struct{ Name, Direction string }
type Policy struct {
	Operation, Kind, Field string
	Position               Position
	Expression             *Expression `json:",omitempty"`
}
type Model struct {
	Name, Collection string
	Position         Position
	Fields           []Field
	Policies         []Policy
	Indexes          [][]IndexField
}
type Enum struct {
	Name     string
	Values   []string
	Position Position
}
type Object struct {
	Name     string
	Fields   []Field
	Position Position
}

// Expression is a source-located policy syntax tree. Validation expands named
// calls and resolves scalar types and model collections before generation.
type Expression struct {
	Kind, Name       string
	Value            Value         `json:",omitempty"`
	Args             []*Expression `json:",omitempty"`
	Position         Position
	Type, Collection string `json:",omitempty"`
}
type Parameter struct {
	Name, Type string
	Position   Position
}
type NamedPolicy struct {
	Name       string
	Parameters []Parameter
	Expression *Expression
	Position   Position
}
type Claim struct {
	Name, Type string
	Position   Position
}
type Schema struct {
	Claims        []Claim       `json:",omitempty"`
	ClaimBlocks   []Position    `json:",omitempty"`
	NamedPolicies []NamedPolicy `json:",omitempty"`
	Enums         []Enum
	Objects       []Object
	Version       string
	Models        []Model
}
type Config struct{ Version, Target, Schemas, Output, Firebase string }

// FieldNamed resolves a field in this model.
func (m Model) FieldNamed(name string) (Field, bool) {
	for _, f := range m.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

// DocumentAccessCost bounds generated access calls without assuming caching.
// Scalar guards are included, even when rendering can deduplicate them.
func (x *Expression) DocumentAccessCost() int {
	if x == nil {
		return 0
	}
	if x.Kind == "member" && len(x.Args) == 1 && x.Args[0].Kind == "lookup" {
		n := 2
		if x.Type == "date" {
			n = 4
		}
		if x.Type == "timestamp" {
			n = 3
		}
		if x.Type == "int" {
			n = 4
		}
		if x.Type == "float" {
			n = 5
		}
		return n
	}
	n := 0
	if x.Kind == "lookup" {
		n = 1
	}
	for _, a := range x.Args {
		n += a.DocumentAccessCost()
	}
	return n
}
