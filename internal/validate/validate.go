package validate

import (
	"fmt"
	"github.com/jonace-mpelule/fQL/internal/schema"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const Version = "0.1"
const MaxSafeInteger = 9007199254740991

var modelName = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)
var fieldName = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)
var collectionName = regexp.MustCompile(`^[a-z][A-Za-z0-9_]*$`)
var reserved = map[string]bool{}

func init() {
	for _, s := range strings.Fields("abstract as assert async await base break case catch class const continue covariant default deferred do dynamic else enum export extends extension external factory false final finally for Function get hide if implements import in interface is late library mixin new null of on operator part required rethrow return sealed set show static super switch sync this throw true try typedef var void when while with yield id data toMap decode fields hasField document firestore version create update get delete query watch collection metadata cursor FqlClient FqlDate FqlModel FqlError FqlSnapshot FqlField FqlQuery FqlRepository String DateTime Timestamp Object Map List Future Stream FieldValue FirebaseFirestore hashCode runtimeType noSuchMethod toString Set Record Null Never Type Iterable Exception Comparable Source GetOptions CollectionReference DocumentReference DocumentSnapshot Query QuerySnapshot FirebaseException SnapshotMetadata FqlPredicate FqlCursor FqlBatch FqlTransaction FqlOperation FqlServerTimestamp FqlObject batch transaction Enum Transaction WriteBatch") {
		reserved[s] = true
	}
}
func fail(pos schema.Position, msg string) error {
	return schema.Diagnostic{Position: pos, Message: msg}
}

// Schema normalizes and validates the complete graph before generation.
func Schema(s schema.Schema) (schema.Schema, error) {
	if s.Version != Version {
		return s, fmt.Errorf("unsupported schema version %q; expected %s", s.Version, Version)
	}
	if len(s.Models) == 0 || len(s.Models) > 100 {
		return s, fmt.Errorf("schema must contain between 1 and 100 models")
	}
	var err error
	s, err = resolveNames(s)
	if err != nil {
		return s, err
	}
	for i := range s.Objects {
		s.Objects[i].Fields, err = normalizeFields(s.Objects[i].Fields, s.Objects[i].Position)
		if err != nil {
			return s, err
		}
	}
	names, collections := map[string]bool{}, map[string]bool{}
	for mi := range s.Models {
		m := &s.Models[mi]
		if len(m.Name) > 64 || !modelName.MatchString(m.Name) || reserved[m.Name] || strings.HasPrefix(m.Name, "Fql") {
			return s, fail(m.Position, "model name must be a nonreserved UpperCamelCase identifier")
		}
		if names[m.Name] {
			return s, fail(m.Position, "duplicate model "+m.Name)
		}
		names[m.Name] = true
		if len(m.Collection) > 64 || !collectionName.MatchString(m.Collection) || reserved[m.Collection] || strings.HasPrefix(m.Collection, "__") {
			return s, fail(m.Position, "collection must be a nonreserved top-level identifier")
		}
		if collections[m.Collection] {
			return s, fail(m.Position, "duplicate collection mapping "+m.Collection)
		}
		collections[m.Collection] = true
		normalized, err := normalizeFields(m.Fields, m.Position)
		if err != nil {
			return s, err
		}
		m.Fields = normalized
		ops := map[string]bool{}
		for _, a := range m.Policies {
			if ops[a.Operation] {
				return s, fail(a.Position, "duplicate access operation")
			}
			ops[a.Operation] = true
			switch a.Operation {
			case "read", "get", "list", "create", "update", "delete":
			default:
				return s, fail(a.Position, "access operation must be read, get, list, create, update, or delete")
			}
			if a.Expression != nil {
				continue
			}
			switch a.Kind {
			case "deny", "authenticated":
			case "owner":
				if a.Field != "id" {
					f, ok := m.FieldNamed(a.Field)
					if !ok || f.Type != "string" || f.Optional || f.Nullable || !f.Immutable {
						return s, fail(a.Position, "owner field must be a required, nonnullable, immutable string (or id)")
					}
				}
			default:
				return s, fail(a.Position, "unsupported access policy")
			}
		}
		if ops["read"] && (ops["get"] || ops["list"]) {
			return s, fail(m.Position, "read cannot coexist with get/list policies; replace read with separate get and list grants")
		}
		indexes := map[string]bool{}
		for _, idx := range m.Indexes {
			if len(idx) < 2 || len(idx) > 8 {
				return s, fail(m.Position, "composite indexes require 2 to 8 fields")
			}
			used := map[string]bool{}
			key := ""
			for _, f := range idx {
				if _, ok := m.FieldNamed(f.Name); !ok {
					return s, fail(m.Position, "unknown index field "+f.Name)
				}
				if used[f.Name] {
					return s, fail(m.Position, "duplicate field in index")
				}
				used[f.Name] = true
				if f.Direction != "asc" && f.Direction != "desc" {
					return s, fail(m.Position, "index direction must be asc or desc")
				}
				key += f.Name + ":" + f.Direction + ","
			}
			if indexes[key] {
				return s, fail(m.Position, "duplicate index")
			}
			indexes[key] = true
		}
	}
	// All generated library names share a namespace.
	generated := map[string]bool{}
	for _, e := range s.Enums {
		generated[e.Name] = true
	}
	for _, o := range s.Objects {
		if generated[o.Name] {
			return s, fail(o.Position, "duplicate type name")
		}
		generated[o.Name] = true
		if generated[o.Name+"Builder"] {
			return s, fail(o.Position, "generated object builder name collision")
		}
		generated[o.Name+"Builder"] = true
	}
	for _, m := range s.Models {
		for _, name := range []string{m.Name, m.Name + "Fields", m.Name + "Create", m.Name + "Update", m.Name + "Repository"} {
			if generated[name] {
				return s, fail(m.Position, "generated type name collision: "+name)
			}
			generated[name] = true
		}
	}
	s, err = resolveNames(s)
	if err != nil {
		return s, err
	}
	return validatePolicies(s)
}
func literal(f schema.Field, v schema.Value, allowed bool) error {
	if v.Kind == "null" {
		if f.Nullable {
			return nil
		}
		return fmt.Errorf("null requires nullable()")
	}
	valid := false
	switch f.Type {
	case "string":
		valid = v.Kind == "string"
		n := len(v.Text)
		if f.MinLength != nil && n < *f.MinLength || f.MaxLength != nil && n > *f.MaxLength {
			return fmt.Errorf("literal violates string length constraint")
		}
		if !utf8.ValidString(v.Text) {
			valid = false
		}
	case "bool":
		valid = v.Kind == "true" || v.Kind == "false"
	case "int":
		n, e := strconv.ParseInt(v.Text, 10, 64)
		valid = v.Kind == "number" && e == nil && n >= -MaxSafeInteger && n <= MaxSafeInteger
	case "float":
		n, e := strconv.ParseFloat(v.Text, 64)
		valid = v.Kind == "number" && e == nil && !math.IsNaN(n) && !math.IsInf(n, 0) && math.Abs(n) <= MaxSafeInteger
	case "date":
		d, e := time.Parse("2006-01-02", v.Text)
		valid = v.Kind == "string" && e == nil && d.Year() >= 1 && d.Year() <= 9999 && d.Format("2006-01-02") == v.Text
	default:
		if len(f.EnumValues) > 0 && v.Kind == "string" {
			for _, x := range f.EnumValues {
				if x == v.Text {
					valid = true
				}
			}
		}
	case "timestamp":
		d, e := time.Parse(time.RFC3339Nano, v.Text)
		valid = v.Kind == "string" && e == nil && d.UTC().Year() >= 1 && d.UTC().Year() <= 9999 && d.Nanosecond()%1000 == 0
	}
	if !valid {
		return fmt.Errorf("literal does not match field type or supported range")
	}
	if f.Type == "int" || f.Type == "float" {
		n, _ := strconv.ParseFloat(v.Text, 64)
		if f.Min != nil && n < *f.Min || f.Max != nil && n > *f.Max {
			return fmt.Errorf("literal violates numeric bound")
		}
	}
	if allowed && len(f.Allowed) > 0 {
		for _, x := range f.Allowed {
			if equalLiteral(x, v, f.Type) {
				return nil
			}
		}
		return fmt.Errorf("default is not in oneOf values")
	}
	return nil
}

func equalLiteral(a, b schema.Value, kind string) bool {
	if a.Kind == "null" || b.Kind == "null" {
		return a.Kind == b.Kind
	}
	if kind == "float" || kind == "int" {
		x, xe := strconv.ParseFloat(a.Text, 64)
		y, ye := strconv.ParseFloat(b.Text, 64)
		return xe == nil && ye == nil && x == y
	}
	if kind == "timestamp" {
		x, xe := time.Parse(time.RFC3339Nano, a.Text)
		y, ye := time.Parse(time.RFC3339Nano, b.Text)
		return xe == nil && ye == nil && x.Equal(y)
	}
	return a.Kind == b.Kind && a.Text == b.Text
}

func normalizeFields(fields []schema.Field, position schema.Position) ([]schema.Field, error) {
	if len(fields) == 0 || len(fields) > 32 {
		return fields, fail(position, "models must contain between 1 and 32 fields")
	}
	fieldNames := map[string]bool{}
	for fi := range fields {
		f := &fields[fi]
		if len(f.Name) > 64 || !fieldName.MatchString(f.Name) || reserved[f.Name] || strings.HasPrefix(f.Name, "has") {
			return fields, fail(f.Position, "field name must be a nonreserved lowerCamelCase identifier (has prefix is reserved)")
		}
		if fieldNames[f.Name] {
			return fields, fail(f.Position, "duplicate field "+f.Name)
		}
		fieldNames[f.Name] = true
		switch f.Type {
		case "string", "bool", "int", "float", "date", "timestamp", "list":
		default:
			if len(f.EnumValues) == 0 && len(f.ObjectFields) == 0 {
				return fields, fail(f.Position, "unsupported field type "+f.Type)
			}
		}
		seen := map[string]bool{}
		presence := ""
		for _, mod := range f.Modifiers {
			if seen[mod.Name] {
				return fields, fail(mod.Position, "duplicate modifier "+mod.Name)
			}
			seen[mod.Name] = true
			n := len(mod.Args)
			switch mod.Name {
			case "required", "optional", "nullable", "immutable":
				if n != 0 {
					return fields, fail(mod.Position, "modifier takes no arguments")
				}
				switch mod.Name {
				case "required", "optional":
					if presence != "" {
						return fields, fail(mod.Position, "choose required or optional, not both")
					}
					presence = mod.Name
					f.Optional = mod.Name == "optional"
				case "nullable":
					f.Nullable = true
				case "immutable":
					f.Immutable = true
				}
			case "minLength", "maxLength":
				if n != 1 || f.Type != "string" {
					return fields, fail(mod.Position, "length constraint requires one integer argument on a string")
				}
				v, e := strconv.Atoi(mod.Args[0].Text)
				if e != nil || v < 0 || v > 1048487 || mod.Args[0].Kind != "number" {
					return fields, fail(mod.Position, "length must be an integer from 0 to 1048487 UTF-8 bytes")
				}
				if mod.Name == "minLength" {
					f.MinLength = &v
				} else {
					f.MaxLength = &v
				}
			case "min", "max":
				if n != 1 || (f.Type != "int" && f.Type != "float") || mod.Args[0].Kind != "number" {
					return fields, fail(mod.Position, "numeric bound requires one numeric argument on int or float")
				}
				v, e := strconv.ParseFloat(mod.Args[0].Text, 64)
				if e != nil || math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > MaxSafeInteger {
					return fields, fail(mod.Position, "numeric bound is outside the supported finite safe range")
				}
				if f.Type == "int" {
					n, err := strconv.ParseInt(mod.Args[0].Text, 10, 64)
					if err != nil || n < -MaxSafeInteger || n > MaxSafeInteger {
						return fields, fail(mod.Position, "integer bounds require safe-range decimal integer literals")
					}
					v = float64(n)
				}
				if mod.Name == "min" {
					f.Min = &v
				} else {
					f.Max = &v
				}
			case "minItems", "maxItems":
				if n != 1 || f.Type != "list" {
					return fields, fail(mod.Position, "item constraints require one integer on a list")
				}
				v, e := strconv.Atoi(mod.Args[0].Text)
				if e != nil || v < 0 || v > 16 || mod.Args[0].Kind != "number" {
					return fields, fail(mod.Position, "list bounds must be integers from 0 to 16")
				}
				if mod.Name == "maxItems" {
					f.MaxItems = &v
				} else {
					f.MinItems = &v
				}
			case "serverCreated", "serverUpdated":
				if n != 0 || f.Type != "timestamp" || f.Server != "" {
					return fields, fail(mod.Position, "choose one server timestamp mode on timestamp fields")
				}
				f.Server = mod.Name
			case "default":
				if n != 1 {
					return fields, fail(mod.Position, "default requires one literal argument")
				}
				v := mod.Args[0]
				f.Default = &v
			case "oneOf":
				if n == 0 || n > 32 {
					return fields, fail(mod.Position, "oneOf requires between 1 and 32 literal arguments")
				}
				f.Allowed = mod.Args
			default:
				return fields, fail(mod.Position, "unsupported modifier "+mod.Name)
			}
		}
		if f.Type == "list" && (f.MaxItems == nil || f.MinItems != nil && *f.MinItems > *f.MaxItems) {
			return fields, fail(f.Position, "lists require maxItems(0..16) with consistent bounds for complete Rules enforcement")
		}
		if f.Server != "" {
			if f.Optional || f.Nullable || f.Default != nil || len(f.Allowed) > 0 {
				return fields, fail(f.Position, "server timestamps must be required, nonnullable, without defaults or oneOf")
			}
			if f.Server == "serverCreated" {
				f.Immutable = true
			} else if f.Immutable {
				return fields, fail(f.Position, "serverUpdated cannot be immutable")
			}
		}
		if presence == "" {
			return fields, fail(f.Position, "declare required() or optional() explicitly")
		}
		if f.MinLength != nil && f.MaxLength != nil && *f.MinLength > *f.MaxLength || f.Min != nil && f.Max != nil && *f.Min > *f.Max {
			return fields, fail(f.Position, "minimum exceeds maximum")
		}
		for _, v := range f.Allowed {
			if err := literal(*f, v, false); err != nil {
				return fields, fail(v.Position, err.Error())
			}
		}
		if f.Default != nil {
			if err := literal(*f, *f.Default, true); err != nil {
				return fields, fail(f.Default.Position, err.Error())
			}
		}
	}

	return fields, nil
}
