package generate

import (
	"fmt"
	"github.com/jonace-mpelule/fQL/internal/schema"
	"strings"
)

type policyRenderer struct {
	guards []string
	seen   map[string]bool
	err    error
}

func (r *policyRenderer) guard(s string) {
	if !r.seen[s] {
		r.seen[s] = true
		r.guards = append(r.guards, s)
	}
}
func (r *policyRenderer) scalar(v, t string) {
	switch t {
	case "string":
		r.guard(v + " is string")
	case "date":
		r.guard(v + " is string")
		r.guard(v + ".matches('[0-9]{4}-[0-9]{2}-[0-9]{2}')")
		r.guard("validDate(" + v + ")")
	case "bool":
		r.guard(v + " is bool")
	case "timestamp":
		// An explicit `is timestamp` guard cannot be proven for range queries.
		// These exhaustive ordered comparisons require a timestamp at runtime
		// and remain provable for queries constrained relative to request.time.
		r.guard("(" + v + " <= request.time || " + v + " > request.time)")
	case "int":
		r.guard(v + " is int")
		r.guard(v + " >= -9007199254740991")
		r.guard(v + " <= 9007199254740991")
	case "float":
		r.guard("(" + v + " is int || " + v + " is float)")
		r.guard(v + " >= -9007199254740991")
		r.guard(v + " <= 9007199254740991")
	default:
		r.err = fmt.Errorf("unresolved policy scalar type")
	}
}
func (r *policyRenderer) expr(x *schema.Expression) string {
	if x == nil {
		r.err = fmt.Errorf("missing validated policy expression")
		return "false"
	}
	switch x.Kind {
	case "literal":
		return literal(x.Value, x.Type, false)
	case "name":
		switch x.Name {
		case "authenticated":
			return "request.auth != null"
		case "deny":
			return "false"
		case "id":
			return "documentId"
		case "now":
			return "request.time"
		}
	case "uid":
		r.guard("request.auth != null")
		v := "request.auth.uid"
		r.scalar(v, "string")
		return v
	case "claim":
		r.guard("request.auth != null")
		v := "request.auth.token[" + quoted(x.Name) + "]"
		r.scalar(v, x.Type)
		return v
	case "owner":
		r.guard("request.auth != null")
		v := "documentId"
		if x.Name != "id" {
			base := "resource.data"
			if x.Collection == "incoming" {
				base = "request.resource.data"
			}
			v = base + "[" + quoted(x.Name) + "]"
			r.scalar(v, "string")
		}
		return "(" + v + " == request.auth.uid)"
	case "member":
		if len(x.Args) != 1 {
			break
		}
		base := x.Args[0]
		v := ""
		if base.Kind == "state" {
			if base.Name == "incoming" {
				v = "request.resource.data"
			} else {
				v = "resource.data"
			}
		} else {
			v = r.expr(base) + ".data"
		}
		v += "[" + quoted(x.Name) + "]"
		r.scalar(v, x.Type)
		return v
	case "lookup":
		if len(x.Args) != 1 {
			break
		}
		id := r.expr(x.Args[0])
		r.scalar(id, "string")
		r.guard(id + ".size() > 0")
		r.guard(id + ".toUtf8().size() <= 1500")
		r.guard(id + ".matches('[^/]+')")
		r.guard(id + " != '.'")
		r.guard(id + " != '..'")
		return x.Name + "(/databases/$(database)/documents/" + x.Collection + "/$(" + id + "))"
	case "unary":
		if len(x.Args) == 1 {
			return "!(" + r.expr(x.Args[0]) + ")"
		}
	case "binary":
		if len(x.Args) == 2 {
			a := r.expr(x.Args[0])
			b := r.expr(x.Args[1])
			op := x.Name
			if op == "and" {
				op = "&&"
			}
			if op == "or" {
				op = "||"
			}
			return "(" + a + " " + op + " " + b + ")"
		}
	}
	r.err = fmt.Errorf("unresolved or unsupported policy expression; validate the schema before generation")
	return "false"
}
func expressionPolicy(x *schema.Expression) (string, error) {
	r := policyRenderer{seen: map[string]bool{}}
	body := r.expr(x)
	if r.err != nil {
		return "", r.err
	}
	r.guards = append(r.guards, body)
	out := "(" + strings.Join(r.guards, "\n          && ") + ")"
	// Count emitted calls, including guards and repetitions; do not assume caching.
	calls := x.DocumentAccessCost()
	if calls > 10 {
		return "", schema.Diagnostic{Position: x.Position, Message: "policy exceeds ten emitted document access calls; reduce related-document checks"}
	}
	if len(out) > 60000 {
		return "", schema.Diagnostic{Position: x.Position, Message: "expanded policy exceeds 60000-byte output budget"}
	}
	return out, nil
}
