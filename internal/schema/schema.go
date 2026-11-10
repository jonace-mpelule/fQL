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
type Schema struct {
	Enums   []Enum
	Objects []Object
	Version string
	Models  []Model
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
