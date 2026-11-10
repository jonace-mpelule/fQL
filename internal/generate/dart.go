package generate

import (
	_ "embed"
	"fmt"
	"github.com/jonace-mpelule/fQL/internal/schema"
	"strconv"
	"strings"
)

//go:embed runtime.dart.tmpl
var runtime string

func dartString(s string) string { return strings.ReplaceAll(quoted(s), "$", `\$`) }
func dartType(f schema.Field) string {
	if f.Server != "" {
		return "FqlServerTimestamp"
	}
	if f.Type == "list" {
		return "List<" + dartType(*f.ElementField) + ">"
	}
	if len(f.EnumValues) > 0 || len(f.ObjectFields) > 0 {
		return f.Type
	}
	switch f.Type {
	case "string":
		return "String"
	case "bool":
		return "bool"
	case "int":
		return "int"
	case "float":
		return "double"
	case "date":
		return "FqlDate"
	default:
		return "Timestamp"
	}
}
func upper(s string) string { return strings.ToUpper(s[:1]) + s[1:] }
func lower(s string) string { return strings.ToLower(s[:1]) + s[1:] }

// Dart renders an SDK-backed library from a previously validated schema.
func Dart(s schema.Schema) []byte {
	var b strings.Builder
	b.WriteString(runtime)
	for _, e := range s.Enums {
		fmt.Fprintf(&b, "\nenum %s { %s }\n", e.Name, strings.Join(e.Values, ", "))
	}
	for _, o := range s.Objects {
		writeMetadata(&b, o.Name, "", o.Fields)
		meta := "_" + lower(o.Name) + "Schema"
		fmt.Fprintf(&b, "\nclass %s extends FqlObject {\n  %s._(Map<String,Object?> values) : super._(values);\n  static %s decode(Map<String, dynamic> data) => %s._(%s.decode('', data));\n  static %sBuilder create() => %sBuilder._();\n", o.Name, o.Name, o.Name, o.Name, meta, o.Name, o.Name)
		writeGetters(&b, o.Fields)
		b.WriteString("}\n")
		fmt.Fprintf(&b, "class %sBuilder {\n  final Map<String,Object?> _values = {};\n  %sBuilder._();\n", o.Name, o.Name)
		writeSetters(&b, o.Name+"Builder", o.Fields, false)
		fmt.Fprintf(&b, "  %s build() => %s.decode(%s.write(_values, update: false));\n}\n", o.Name, o.Name, meta)
	}
	b.WriteString("\n/// Generated client; configure the official SDK before injection.\nclass FqlClient {\n  final FirebaseFirestore _firestore;\n  FqlBatch batch() => FqlBatch._(_firestore);\n  Future<R> transaction<R>(Future<R> Function(FqlTransaction) action) => _runTransaction(_firestore, action);\n")
	for _, m := range s.Models {
		fmt.Fprintf(&b, "  final %sRepository %s;\n", m.Name, m.Collection)
	}
	b.WriteString("  FqlClient(FirebaseFirestore firestore) : _firestore = firestore,\n")
	for i, m := range s.Models {
		end := ","
		if i == len(s.Models)-1 {
			end = ";"
		}
		fmt.Fprintf(&b, "    %s = %sRepository._(firestore)%s\n", m.Collection, m.Name, end)
	}
	b.WriteString("}\n")
	for _, m := range s.Models {
		meta := "_" + lower(m.Name) + "Schema"
		writeMetadata(&b, m.Name, m.Collection, m.Fields)
		fmt.Fprintf(&b, "\nclass %s extends FqlModel {\n  %s._(String id, Map<String, Object?> values) : super._(id, values);\n  static %s decode(String id, Map<String, dynamic> data, {bool hasPendingWrites = false}) => %s._(id, %s.decode(id, data, pending: hasPendingWrites));\n", m.Name, m.Name, m.Name, m.Name, meta)
		for _, f := range m.Fields {
			t := dartType(f)
			if f.Optional || f.Nullable {
				t += "?"
			}
			fmt.Fprintf(&b, "  %s get %s => _values[%s] as %s;\n", t, f.Name, dartString(f.Name), t)
			if f.Optional {
				fmt.Fprintf(&b, "  bool get has%s => hasField(%s);\n", upper(f.Name), dartString(f.Name))
			}
		}
		b.WriteString("}\n")
		fmt.Fprintf(&b, "\nabstract final class %sFields {\n", m.Name)
		for _, f := range m.Fields {
			base := f
			base.Server = ""
			t := dartType(base)
			if f.Nullable {
				t += "?"
			}
			fmt.Fprintf(&b, "  static final %s = FqlField<%s>._(%s, %s);\n", f.Name, t, meta, dartString(f.Name))
		}
		b.WriteString("}\n")
		fmt.Fprintf(&b, "\nclass %sRepository extends FqlRepository<%s> {\n  %sRepository._(FirebaseFirestore db) : super._(db, %s, %s.decode);\n  %sCreate create({String? id}) => %sCreate._(this, id);\n  %sUpdate update(String id) => %sUpdate._(this, id);\n}\n", m.Name, m.Name, m.Name, meta, m.Name, m.Name, m.Name, m.Name, m.Name)
		for _, update := range []bool{false, true} {
			suffix := "Create"
			idType := "String?"
			if update {
				suffix = "Update"
				idType = "String"
			}
			name := m.Name + suffix
			fmt.Fprintf(&b, "\nclass %s {\n  final %sRepository _repository;\n  final %s _id;\n  final Map<String, Object?> _values = {};\n  %s._(this._repository, this._id);\n", name, m.Name, idType, name)
			for _, f := range m.Fields {
				if f.Server != "" || update && f.Immutable {
					continue
				}
				t := dartType(f)
				if f.Nullable {
					t += "?"
				}
				fmt.Fprintf(&b, "  %s set%s(%s value) { _values[%s] = value; return this; }\n", name, upper(f.Name), t, dartString(f.Name))
				if update && f.Optional {
					fmt.Fprintf(&b, "  %s remove%s() { _values[%s] = _removed; return this; }\n", name, upper(f.Name), dartString(f.Name))
				}
			}
			if update {
				b.WriteString("  FqlOperation<void> operation() => _repository._prepareUpdate(_id, _values);\n  Future<void> save() => _repository._update(_id, _values);\n")
			} else {
				fmt.Fprintf(&b, "  FqlOperation<%s> operation() => _repository._prepareCreate(_values, _id);\n", m.Name)
				fmt.Fprintf(&b, "  Future<%s> save() => _repository._create(_values, _id);\n", m.Name)
			}
			b.WriteString("}\n")
		}
	}
	return []byte(b.String())
}

func writeMetadata(b *strings.Builder, name, collection string, fields []schema.Field) {
	fmt.Fprintf(b, "\nfinal _%sSchema = _Schema(%s,%s,[\n", lower(name), dartString(name), dartString(collection))
	for _, f := range fields {
		b.WriteString("  " + fieldMetadata(f) + ",\n")
	}
	b.WriteString("]);\n")
}
func fieldMetadata(f schema.Field) string {
	var b strings.Builder
	kind := f.Type
	if len(f.EnumValues) > 0 {
		kind = "enum"
	}
	if len(f.ObjectFields) > 0 {
		kind = "object"
	}
	fmt.Fprintf(&b, "_Field(%s,%s, optional:%t, nullable:%t, immutable:%t, server:%s", dartString(f.Name), dartString(kind), f.Optional, f.Nullable, f.Immutable, dartString(f.Server))
	for _, n := range []struct {
		name  string
		value *int
	}{{"minLength", f.MinLength}, {"maxLength", f.MaxLength}, {"minItems", f.MinItems}, {"maxItems", f.MaxItems}} {
		v := "null"
		if n.value != nil {
			v = strconv.Itoa(*n.value)
		}
		fmt.Fprintf(&b, ", %s:%s", n.name, v)
	}
	for _, n := range []struct {
		name  string
		value *float64
	}{{"min", f.Min}, {"max", f.Max}} {
		v := "null"
		if n.value != nil {
			v = strconv.FormatFloat(*n.value, 'g', -1, 64)
		}
		fmt.Fprintf(&b, ", %s:%s", n.name, v)
	}
	def := "null"
	if f.Default != nil {
		def = fieldLiteral(f, *f.Default)
	}
	fmt.Fprintf(&b, ", hasDefault:%t, defaultValue:%s, allowed:[", f.Default != nil, def)
	for _, v := range f.Allowed {
		b.WriteString(fieldLiteral(f, v) + ",")
	}
	b.WriteString("], enumValues:{")
	for _, v := range f.EnumValues {
		fmt.Fprintf(&b, "%s:%s.%s,", dartString(v), f.Type, v)
	}
	b.WriteString("}")
	if f.ElementField != nil {
		fmt.Fprintf(&b, ", element:%s, listDecode:(values)=>List<%s>.unmodifiable(values.cast<%s>())", fieldMetadata(*f.ElementField), dartType(*f.ElementField), dartType(*f.ElementField))
	} else {
		b.WriteString(", element:null, listDecode:null")
	}
	if len(f.ObjectFields) > 0 {
		fmt.Fprintf(&b, ", objectDecode:%s.decode, objectAccept:(value)=>value is %s", f.Type, f.Type)
	} else {
		b.WriteString(", objectDecode:null, objectAccept:null")
	}
	b.WriteString(")")
	return b.String()
}
func fieldLiteral(f schema.Field, v schema.Value) string {
	if len(f.EnumValues) > 0 && v.Kind != "null" {
		return f.Type + "." + v.Text
	}
	return literal(v, f.Type, true)
}
func writeGetters(b *strings.Builder, fields []schema.Field) {
	for _, f := range fields {
		t := dartType(f)
		if f.Optional || f.Nullable {
			t += "?"
		}
		fmt.Fprintf(b, "  %s get %s => _values[%s] as %s;\n", t, f.Name, dartString(f.Name), t)
		if f.Optional {
			fmt.Fprintf(b, "  bool get has%s => _values.containsKey(%s);\n", upper(f.Name), dartString(f.Name))
		}
	}
}
func writeSetters(b *strings.Builder, name string, fields []schema.Field, update bool) {
	for _, f := range fields {
		if f.Server != "" || update && f.Immutable {
			continue
		}
		t := dartType(f)
		if f.Nullable {
			t += "?"
		}
		fmt.Fprintf(b, "  %s set%s(%s value) { _values[%s] = value; return this; }\n", name, upper(f.Name), t, dartString(f.Name))
	}
}
