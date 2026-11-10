// Package data validates stored wire values without defaults, coercion, or writes.
package data

import (
	"github.com/jonace-mpelule/fQL/internal/schema"
	"math"
	"strconv"
	"time"
	"unicode/utf8"
)

type Timestamp string
type Invalid struct{}
type Issue struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func Validate(fields []schema.Field, values map[string]any) []Issue {
	issues := []Issue{}
	known := map[string]bool{}
	for _, f := range fields {
		known[f.Name] = true
		v, present := values[f.Name]
		if !present {
			if !f.Optional {
				issues = append(issues, Issue{f.Name, "missing", "Required field is missing"})
			}
			continue
		}
		issues = append(issues, field(f, v, f.Name)...)
	}
	for k := range values {
		if !known[k] {
			issues = append(issues, Issue{k, "unknown", "Unknown stored field"})
		}
	}
	return issues
}
func field(f schema.Field, v any, path string) []Issue {
	fail := func(message string) []Issue { return []Issue{{path, "invalid", message}} }
	if v == nil {
		if f.Nullable {
			return nil
		}
		return fail("Null is not allowed")
	}
	valid := false
	var number float64
	switch f.Type {
	case "string":
		x, ok := v.(string)
		valid = ok && utf8.ValidString(x) && len(x) <= 1048487 && (f.MinLength == nil || len(x) >= *f.MinLength) && (f.MaxLength == nil || len(x) <= *f.MaxLength)
	case "bool":
		_, valid = v.(bool)
	case "int":
		x, ok := v.(int64)
		valid = ok && x >= -9007199254740991 && x <= 9007199254740991
		number = float64(x)
	case "float":
		switch x := v.(type) {
		case int64:
			number = float64(x)
			valid = true
		case float64:
			number = x
			valid = true
		}
		valid = valid && !math.IsNaN(number) && !math.IsInf(number, 0) && math.Abs(number) <= 9007199254740991
	case "date":
		x, ok := v.(string)
		t, e := time.Parse("2006-01-02", x)
		valid = ok && e == nil && t.Year() >= 1 && t.Year() <= 9999 && t.Format("2006-01-02") == x
	case "timestamp":
		x, ok := v.(Timestamp)
		t, e := time.Parse(time.RFC3339Nano, string(x))
		valid = ok && e == nil && t.UTC().Year() >= 1 && t.UTC().Year() <= 9999 && t.Nanosecond()%1000 == 0
	case "list":
		x, ok := v.([]any)
		if !ok || f.MaxItems == nil || len(x) > *f.MaxItems || f.MinItems != nil && len(x) < *f.MinItems {
			return fail("Invalid bounded list")
		}
		issues := []Issue{}
		for i, value := range x {
			issues = append(issues, field(*f.ElementField, value, path+"["+strconv.Itoa(i)+"]")...)
		}
		return issues
	default:
		if len(f.EnumValues) > 0 {
			if x, ok := v.(string); ok {
				for _, a := range f.EnumValues {
					if a == x {
						valid = true
					}
				}
			}
		}
		if len(f.ObjectFields) > 0 {
			x, ok := v.(map[string]any)
			if !ok {
				return fail("Expected object map")
			}
			issues := Validate(f.ObjectFields, x)
			for i := range issues {
				issues[i].Field = path + "." + issues[i].Field
			}
			return issues
		}
	}
	if !valid {
		return fail("Stored value violates its declared type, range, or constraints")
	}
	if (f.Type == "int" || f.Type == "float") && (f.Min != nil && number < *f.Min || f.Max != nil && number > *f.Max) {
		return fail("Number violates bounds")
	}
	if len(f.Allowed) > 0 {
		for _, a := range f.Allowed {
			match := false
			switch f.Type {
			case "int", "float":
				n, e := strconv.ParseFloat(a.Text, 64)
				match = e == nil && n == number
			case "bool":
				x, _ := v.(bool)
				match = a.Text == strconv.FormatBool(x)
			case "timestamp":
				x, _ := v.(Timestamp)
				t, e := time.Parse(time.RFC3339Nano, string(x))
				u, err := time.Parse(time.RFC3339Nano, a.Text)
				match = e == nil && err == nil && t.Equal(u)
			default:
				x, ok := v.(string)
				match = ok && a.Text == x
			}
			if match {
				return nil
			}
		}
		return fail("Value is outside the allowed set")
	}
	return nil
}
