package generate

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jonace-mpelule/fQL/internal/schema"
)

//go:embed runtime.ts.tmpl
var typescriptRuntime string

func tsType(f schema.Field) string {
	if f.Server != "" {
		return "FqlServerTimestamp"
	}
	if f.ElementField != nil {
		return "readonly (" + tsType(*f.ElementField) + ")[]"
	}
	if len(f.EnumValues) > 0 || len(f.ObjectFields) > 0 {
		return f.Type
	}
	switch f.Type {
	case "string":
		return "string"
	case "bool":
		return "boolean"
	case "int", "float":
		return "number"
	case "date":
		return "FqlDate"
	default:
		return "Timestamp"
	}
}
func tsLiteral(f schema.Field, v schema.Value) string {
	if v.Kind == "null" {
		return "null"
	}
	if f.Type == "timestamp" {
		t, _ := time.Parse(time.RFC3339Nano, v.Text)
		return fmt.Sprintf("new Timestamp(%d, %d)", t.Unix(), t.Nanosecond())
	}
	if f.Type == "date" {
		return "FqlDate.parse(" + quoted(v.Text) + ")"
	}
	if v.Kind == "string" {
		return quoted(v.Text)
	}
	return v.Text
}
func tsField(f schema.Field) string {
	kind := f.Type
	if len(f.EnumValues) > 0 {
		kind = "enum"
	}
	if len(f.ObjectFields) > 0 {
		kind = "object"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "{ name: %s, type: %s, optional: %t, nullable: %t, immutable: %t, server: %s, hasDefault: %t", quoted(f.Name), quoted(kind), f.Optional, f.Nullable, f.Immutable, quoted(f.Server), f.Default != nil)
	if f.Default != nil {
		b.WriteString(", defaultValue: " + tsLiteral(f, *f.Default))
	}
	for _, n := range []struct {
		name  string
		value *int
	}{{"minLength", f.MinLength}, {"maxLength", f.MaxLength}, {"minItems", f.MinItems}, {"maxItems", f.MaxItems}} {
		if n.value != nil {
			b.WriteString(", " + n.name + ": " + strconv.Itoa(*n.value))
		}
	}
	for _, n := range []struct {
		name  string
		value *float64
	}{{"min", f.Min}, {"max", f.Max}} {
		if n.value != nil {
			b.WriteString(", " + n.name + ": " + strconv.FormatFloat(*n.value, 'g', -1, 64))
		}
	}
	b.WriteString(", allowed: [")
	for _, v := range f.Allowed {
		b.WriteString(tsLiteral(f, v) + ", ")
	}
	b.WriteString("]")
	if len(f.EnumValues) > 0 {
		b.WriteString(", enumValues: [")
		for _, v := range f.EnumValues {
			b.WriteString(quoted(v) + ", ")
		}
		b.WriteString("]")
	}
	if f.ElementField != nil {
		b.WriteString(", element: " + tsField(*f.ElementField))
	}
	if len(f.ObjectFields) > 0 {
		fmt.Fprintf(&b, ", objectDecode: value => %s.decode(value), objectAccept: value => value instanceof %s", f.Type, f.Type)
	}
	b.WriteString(" }")
	return b.String()
}
func tsMetadata(b *strings.Builder, name, collection string, fields []schema.Field) {
	fmt.Fprintf(b, "\nconst _%sSchema = new _Schema(%s, %s, [\n", name, quoted(name), quoted(collection))
	for _, f := range fields {
		b.WriteString("  " + tsField(f) + ",\n")
	}
	b.WriteString("]);\n")
}
func tsGetters(b *strings.Builder, fields []schema.Field) {
	for _, f := range fields {
		t := tsType(f)
		if f.Nullable {
			t += " | null"
		}
		if f.Optional {
			t += " | undefined"
		}
		fmt.Fprintf(b, "  get %s(): %s { return _get(this, %s) as %s; }\n", f.Name, t, quoted(f.Name), t)
		if f.Optional {
			fmt.Fprintf(b, "  get has%s(): boolean { return this.hasField(%s); }\n", upper(f.Name), quoted(f.Name))
		}
	}
}
func tsSetters(b *strings.Builder, fields []schema.Field, update bool) {
	for _, f := range fields {
		if f.Server != "" || update && f.Immutable {
			continue
		}
		t := tsType(f)
		if f.Nullable {
			t += " | null"
		}
		fmt.Fprintf(b, "  set%s(value: %s): this { this.values[%s] = value; return this; }\n", upper(f.Name), t, quoted(f.Name))
		if update && f.Optional {
			fmt.Fprintf(b, "  remove%s(): this { this.values[%s] = _removed; return this; }\n", upper(f.Name), quoted(f.Name))
		}
	}
}

// TypeScript renders a modular Firebase SDK client from a validated schema.
func TypeScript(s schema.Schema) []byte {
	var b strings.Builder
	b.WriteString(typescriptRuntime)
	for _, e := range s.Enums {
		fmt.Fprintf(&b, "\nexport const %s = Object.freeze({\n", e.Name)
		for _, v := range e.Values {
			fmt.Fprintf(&b, "  %s: %s,\n", v, quoted(v))
		}
		fmt.Fprintf(&b, "} as const);\nexport type %s = typeof %s[keyof typeof %s];\n", e.Name, e.Name, e.Name)
	}
	for _, o := range s.Objects {
		tsMetadata(&b, o.Name, "", o.Fields)
		fmt.Fprintf(&b, "\nexport class %s extends FqlObject {\n  private constructor(values: Record<string, unknown>) { super(values); Object.freeze(this); }\n  static decode(data: Record<string, unknown>): %s { return new %s(_%sSchema.decode(undefined, data)); }\n  static create(): %sBuilder { return new %sBuilder(); }\n", o.Name, o.Name, o.Name, o.Name, o.Name, o.Name)
		tsGetters(&b, o.Fields)
		fmt.Fprintf(&b, "}\nexport class %sBuilder {\n  private readonly values: Record<string, unknown> = Object.create(null);\n", o.Name)
		tsSetters(&b, o.Fields, false)
		fmt.Fprintf(&b, "  build(): %s { return %s.decode(_%sSchema.write(this.values, false)); }\n}\n", o.Name, o.Name, o.Name)
	}
	for _, m := range s.Models {
		tsMetadata(&b, m.Name, m.Collection, m.Fields)
		fmt.Fprintf(&b, "\nexport class %s extends FqlModel {\n  private constructor(id: string, values: Record<string, unknown>) { super(id, values); Object.freeze(this); }\n  static decode(id: string, data: Record<string, unknown>, hasPendingWrites = false): %s { return new %s(id, _%sSchema.decode(id, data, hasPendingWrites)); }\n", m.Name, m.Name, m.Name, m.Name)
		tsGetters(&b, m.Fields)
		fmt.Fprintf(&b, "}\nexport const %sFields = Object.freeze({\n", m.Name)
		for _, f := range m.Fields {
			base := f
			base.Server = ""
			t := tsType(base)
			if f.Nullable {
				t += " | null"
			}
			fmt.Fprintf(&b, "  %s: new FqlField<%s>(_%sSchema, %s),\n", f.Name, t, m.Name, quoted(f.Name))
		}
		fmt.Fprintf(&b, "});\nexport class %sRepository extends FqlRepository<%s> {\n  constructor(db: _Firestore) { super(db, _%sSchema, %s.decode); }\n  create(id?: string): %sCreate { return new %sCreate(this, id); }\n  update(id: string): %sUpdate { return new %sUpdate(this, id); }\n}\n", m.Name, m.Name, m.Name, m.Name, m.Name, m.Name, m.Name, m.Name)
		for _, update := range []bool{false, true} {
			suffix, idType := "Create", "id?: string"
			if update {
				suffix, idType = "Update", "id: string"
			}
			fmt.Fprintf(&b, "export class %s%s {\n  private readonly values: Record<string, unknown> = Object.create(null);\n  constructor(private readonly repository: %sRepository, private readonly %s) {}\n", m.Name, suffix, m.Name, idType)
			tsSetters(&b, m.Fields, update)
			fmt.Fprintf(&b, "  operation(): FqlOperation { return this.repository.prepare(this.values, %t, this.id); }\n", update)
			if update {
				b.WriteString("  save(): Promise<void> { return this.repository.saveUpdate(this.values, this.id); }\n")
			} else {
				fmt.Fprintf(&b, "  save(): Promise<%s> { return this.repository.saveCreate(this.values, this.id); }\n", m.Name)
			}
			b.WriteString("}\n")
		}
	}
	b.WriteString("\nexport class FqlClient {\n")
	for _, m := range s.Models {
		fmt.Fprintf(&b, "  readonly %s: %sRepository;\n", m.Collection, m.Name)
	}
	b.WriteString("  constructor(private readonly firestore: _Firestore) {\n")
	for _, m := range s.Models {
		fmt.Fprintf(&b, "    this.%s = new %sRepository(firestore);\n", m.Collection, m.Name)
	}
	b.WriteString("    Object.freeze(this);\n  }\n  batch(): FqlBatch { return new FqlBatch(this.firestore); }\n  transaction<R>(action: (tx: FqlTransaction) => Promise<R>): Promise<R> { return _transaction(this.firestore, action); }\n}\n")
	return []byte(b.String())
}
