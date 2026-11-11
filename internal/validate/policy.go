package validate

import (
	"fmt"
	"github.com/jonace-mpelule/fQL/internal/schema"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

var policyReserved = map[string]bool{"authenticated": true, "deny": true, "owner": true, "get": true, "getAfter": true, "exists": true, "auth": true, "existing": true, "incoming": true, "now": true, "id": true, "and": true, "or": true, "not": true, "true": true, "false": true, "null": true}

func policyIdent(s string) bool {
	return len(s) <= 64 && fieldName.MatchString(s) && !policyReserved[s]
}
func policyScalar(t string) bool {
	switch t {
	case "string", "bool", "int", "float", "timestamp", "date":
		return true
	}
	return false
}

type policyChecker struct {
	s      schema.Schema
	model  *schema.Model
	op     string
	claims map[string]string
	named  map[string]schema.NamedPolicy
	nodes  int
	stack  map[string]bool
}

func validatePolicies(s schema.Schema) (schema.Schema, error) {
	if len(s.ClaimBlocks) > 1 {
		return s, fail(s.ClaimBlocks[1], "declare one claims block for the combined project")
	}
	if len(s.Claims) > 32 || len(s.NamedPolicies) > 64 {
		return s, fmt.Errorf("at most 32 claims and 64 named policies are supported")
	}
	c := policyChecker{s: s, claims: map[string]string{}, named: map[string]schema.NamedPolicy{}, stack: map[string]bool{}}
	for _, a := range s.Claims {
		if !policyIdent(a.Name) || c.claims[a.Name] != "" {
			return s, fail(a.Position, "claim requires a unique lowerCamelCase name")
		}
		if !policyScalar(a.Type) || a.Type == "date" || a.Type == "timestamp" {
			return s, fail(a.Position, "auth claims support string, bool, int, or float")
		}
		c.claims[a.Name] = a.Type
	}
	for _, n := range s.NamedPolicies {
		if !policyIdent(n.Name) || c.named[n.Name].Name != "" {
			return s, fail(n.Position, "named policy requires a unique nonreserved lowerCamelCase name")
		}
		if len(n.Parameters) > 8 {
			return s, fail(n.Position, "named policies accept at most eight parameters")
		}
		used := map[string]bool{}
		for _, p := range n.Parameters {
			if !policyIdent(p.Name) || used[p.Name] || !policyScalar(p.Type) {
				return s, fail(p.Position, "policy parameters require unique nonreserved names and scalar types")
			}
			used[p.Name] = true
		}
		c.named[n.Name] = n
	}
	// Validate even unused definitions; they cannot capture a model's implicit data.
	for _, n := range s.NamedPolicies {
		env := map[string]*schema.Expression{}
		for _, p := range n.Parameters {
			env[p.Name] = &schema.Expression{Kind: "parameter", Name: p.Name, Type: p.Type, Position: p.Position}
		}
		c.model = nil
		c.op = "named"
		c.nodes = 0
		c.stack = map[string]bool{n.Name: true}
		x, e := c.check(n.Expression, env, 0)
		if e != nil {
			return s, e
		}
		if x.Type != "bool" {
			return s, fail(n.Position, "named policy must return bool")
		}
	}
	for i := range s.Models {
		m := &s.Models[i]
		c.model = m
		for j := range m.Policies {
			a := &m.Policies[j]
			if a.Expression == nil {
				continue
			}
			c.op = a.Operation
			c.nodes = 0
			c.stack = map[string]bool{}
			x, e := c.check(a.Expression, nil, 0)
			if e != nil {
				return s, e
			}
			if x.Type != "bool" {
				return s, fail(a.Position, "access policy must return bool")
			}
			if x.DocumentAccessCost() > 10 {
				return s, fail(a.Position, "expanded policy exceeds ten document access calls; reduce related-document checks")
			}
			cost := c.nodes*5 + policyDateCost(x)
			for _, f := range m.Fields {
				cost += ruleCost(f)
			}
			if cost > 900 {
				return s, fail(a.Position, "combined policy and document validation exceeds expression budget; simplify the policy or model")
			}
			a.Expression = x
		}
	}
	return s, nil
}
func (c *policyChecker) check(in *schema.Expression, env map[string]*schema.Expression, depth int) (*schema.Expression, error) {
	if in == nil {
		return nil, fmt.Errorf("missing policy expression")
	}
	c.nodes++
	if c.nodes > 256 || depth > 32 {
		return nil, fail(in.Position, "expanded policy exceeds 256 nodes or depth 32; simplify policy calls")
	}
	x := *in
	x.Args = nil
	bad := func(msg string) (*schema.Expression, error) { return nil, fail(in.Position, msg) }
	child := func(a *schema.Expression) (*schema.Expression, error) { return c.check(a, env, depth+1) }
	switch in.Kind {
	case "parameter":
		return &x, nil
	case "literal":
		switch x.Value.Kind {
		case "string":
			if !utf8.ValidString(x.Value.Text) {
				return bad("policy strings must be valid UTF-8")
			}
			x.Type = "string"
		case "true", "false":
			x.Type = "bool"
		case "number":
			n, e := strconv.ParseFloat(x.Value.Text, 64)
			if e != nil || math.IsInf(n, 0) || math.IsNaN(n) || math.Abs(n) > MaxSafeInteger {
				return bad("policy number must be finite and within the safe numeric range")
			}
			x.Type = "float"
			if !strings.ContainsAny(x.Value.Text, ".eE") {
				if _, e := strconv.ParseInt(x.Value.Text, 10, 64); e != nil {
					return bad("invalid policy integer")
				}
				x.Type = "int"
			}
		default:
			return bad("null is unsupported in policies; use required nonnullable authorization fields")
		}
	case "name":
		if p, ok := env[x.Name]; ok {
			return child(p)
		}
		switch x.Name {
		case "authenticated", "deny":
			x.Type = "bool"
		case "now":
			x.Type = "timestamp"
		case "id":
			if c.model == nil {
				return bad("named policies must receive document IDs as parameters")
			}
			x.Type = "string"
		default:
			return bad("unknown policy value; use a parameter, auth.uid, auth.claims, existing, or incoming")
		}
	case "member":
		if len(in.Args) != 1 {
			return bad("invalid member access")
		}
		base := in.Args[0]
		if base.Kind == "name" && base.Name == "auth" && x.Name == "uid" {
			x.Kind = "uid"
			x.Type = "string"
			break
		}
		if base.Kind == "member" && base.Name == "claims" && len(base.Args) == 1 && base.Args[0].Kind == "name" && base.Args[0].Name == "auth" {
			t := c.claims[x.Name]
			if t == "" {
				return bad("undeclared authentication claim; add its type to the claims block")
			}
			x.Kind = "claim"
			x.Type = t
			break
		}
		var model *schema.Model
		if (base.Kind == "name" || base.Kind == "state") && (base.Name == "existing" || base.Name == "incoming") {
			if c.model == nil {
				return bad("named policies must receive document fields as parameters")
			}
			if base.Name == "existing" && c.op == "create" || base.Name == "incoming" && c.op != "create" && c.op != "update" {
				return bad("document state is unavailable for this access operation")
			}
			model = c.model
			x.Args = []*schema.Expression{{Kind: "state", Name: base.Name}}
		} else {
			b, e := child(base)
			if e != nil {
				return nil, e
			}
			if b.Kind != "lookup" {
				return bad("field access requires existing, incoming, get, or getAfter")
			}
			for i := range c.s.Models {
				if c.s.Models[i].Name == b.Type {
					model = &c.s.Models[i]
					break
				}
			}
			x.Args = []*schema.Expression{b}
		}
		if model == nil {
			return bad("unknown policy model")
		}
		f, ok := model.FieldNamed(x.Name)
		if !ok {
			return bad("unknown policy field " + x.Name + " on " + model.Name)
		}
		if f.Optional || f.Nullable || !policyScalar(f.Type) && len(f.EnumValues) == 0 {
			return bad("policy fields must be required nonnullable root scalars")
		}
		x.Type = f.Type
		if len(f.EnumValues) > 0 {
			x.Type = "string"
		}
	case "call":
		switch x.Name {
		case "owner":
			if c.model == nil || len(in.Args) != 1 || in.Args[0].Kind != "name" {
				return bad("owner requires one model field or id in a model access block")
			}
			name := in.Args[0].Name
			if name != "id" {
				f, ok := c.model.FieldNamed(name)
				if !ok || f.Type != "string" || f.Optional || f.Nullable || !f.Immutable {
					return bad("owner field must be a required, nonnullable, immutable string (or id)")
				}
			}
			x.Kind = "owner"
			x.Name = name
			x.Type = "bool"
			if c.op == "create" {
				x.Collection = "incoming"
			} else {
				x.Collection = "existing"
			}
		case "get", "getAfter", "exists":
			if len(in.Args) != 2 || in.Args[0].Kind != "name" {
				return bad("lookup requires a model name and a string document ID")
			}
			if x.Name == "getAfter" && (c.op == "read" || c.op == "get" || c.op == "list") {
				return bad("getAfter is available only in write policies")
			}
			name := in.Args[0].Name
			var model *schema.Model
			for i := range c.s.Models {
				if c.s.Models[i].Name == name {
					model = &c.s.Models[i]
					break
				}
			}
			if model == nil {
				return bad("unknown lookup model " + name)
			}
			id, e := child(in.Args[1])
			if e != nil {
				return nil, e
			}
			if id.Type != "string" {
				return bad("lookup document ID must be a string")
			}
			if id.Kind == "literal" && (id.Value.Text == "" || id.Value.Text == "." || id.Value.Text == ".." || len(id.Value.Text) > 1500 || strings.Contains(id.Value.Text, "/")) {
				return bad("lookup ID must be a nonempty document path segment of at most 1500 UTF-8 bytes, excluding . and ..")
			}

			// Nested lookups create multiplicative access costs and are deliberately unsupported.
			if hasLookup(id) {
				return bad("lookup IDs cannot depend on other document lookups; pass an explicit ID")
			}
			x.Kind = "lookup"
			x.Type = model.Name
			if x.Name == "exists" {
				x.Type = "bool"
			}
			x.Collection = model.Collection
			x.Args = []*schema.Expression{id}
		default:
			n, ok := c.named[x.Name]
			if !ok {
				return bad("unsupported policy function " + x.Name)
			}
			if c.stack[x.Name] {
				return bad("recursive policy call; named policies must be acyclic")
			}
			if len(in.Args) != len(n.Parameters) {
				return bad("wrong argument count for policy " + x.Name)
			}
			next := map[string]*schema.Expression{}
			for i, a := range in.Args {
				v, e := child(a)
				if e != nil {
					return nil, e
				}
				if v.Type != n.Parameters[i].Type {
					return bad("argument type mismatch for policy " + x.Name)
				}
				next[n.Parameters[i].Name] = v
			}
			c.stack[x.Name] = true
			// Arguments are already resolved, so parameter substitution cannot capture names.
			out, e := c.check(n.Expression, next, depth+1)
			delete(c.stack, x.Name)
			return out, e
		}
	case "uid", "claim", "lookup", "owner":
		// Resolved arguments are copied when substituted into an expanded named call.
		for _, a := range in.Args {
			v, e := child(a)
			if e != nil {
				return nil, e
			}
			x.Args = append(x.Args, v)
		}
	case "state":
		return &x, nil
	case "unary":
		if len(in.Args) != 1 || x.Name != "not" {
			return bad("unsupported unary policy expression")
		}
		v, e := child(in.Args[0])
		if e != nil {
			return nil, e
		}
		if v.Type != "bool" {
			return bad("not requires a boolean")
		}
		x.Args = []*schema.Expression{v}
		x.Type = "bool"
	case "binary":
		if len(in.Args) != 2 {
			return bad("invalid binary policy expression")
		}
		for _, a := range in.Args {
			v, e := child(a)
			if e != nil {
				return nil, e
			}
			x.Args = append(x.Args, v)
		}
		l, r := x.Args[0].Type, x.Args[1].Type
		if x.Name == "and" || x.Name == "or" {
			if l != "bool" || r != "bool" {
				return bad("and/or require boolean operands")
			}
		} else {
			numeric := (l == "int" || l == "float") && (r == "int" || r == "float")
			if !policyScalar(l) || !policyScalar(r) || l != r && !numeric {
				return bad("comparison requires compatible scalar types")
			}
			if x.Name != "==" && x.Name != "!=" && l == "bool" {
				return bad("booleans support only == and !=")
			}
			switch x.Name {
			case "==", "!=", "<", "<=", ">", ">=":
			default:
				return bad("unsupported comparison")
			}
		}
		x.Type = "bool"
	default:
		return bad("unsupported policy expression")
	}
	return &x, nil
}
func hasLookup(x *schema.Expression) bool {
	if x.Kind == "lookup" {
		return true
	}
	for _, a := range x.Args {
		if hasLookup(a) {
			return true
		}
	}
	return false
}

func policyDateCost(x *schema.Expression) int {
	if x == nil {
		return 0
	}
	cost := 0
	if x.Kind == "member" && x.Type == "date" {
		cost = 40
	}
	for _, a := range x.Args {
		cost += policyDateCost(a)
	}
	return cost
}
