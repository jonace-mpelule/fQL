package syntax

import (
	"fmt"
	"github.com/jonace-mpelule/fQL/internal/schema"
	"regexp"
	"strconv"
	"strings"
	"text/scanner"
)

const MaxSourceBytes = 1 << 20

var decimalNumber = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

type parser struct {
	scanner scanner.Scanner
	token   rune
	text    string
	pos     schema.Position
	err     error
	tokens  int
}

func newParser(file string, source []byte) *parser {
	p := &parser{}
	p.scanner.Init(strings.NewReader(string(source)))
	p.scanner.Filename = file
	p.scanner.Mode = scanner.ScanIdents | scanner.ScanStrings | scanner.ScanInts | scanner.ScanFloats | scanner.ScanComments | scanner.SkipComments
	p.scanner.Error = func(s *scanner.Scanner, msg string) {
		if p.err == nil {
			p.err = schema.Diagnostic{Position: schema.Position{File: file, Line: s.Pos().Line, Column: s.Pos().Column}, Message: "invalid token"}
		}
	}
	if len(source) > MaxSourceBytes {
		p.err = fmt.Errorf("%s: source exceeds 1 MiB limit", file)
	}
	p.next()
	return p
}
func (p *parser) next() {
	p.token = p.scanner.Scan()
	p.text = p.scanner.TokenText()
	s := p.scanner.Position
	p.pos = schema.Position{File: s.Filename, Line: s.Line, Column: s.Column, Offset: s.Offset, EndOffset: s.Offset + len(p.text)}
	p.tokens++
	if p.tokens > 100000 {
		p.fail("source exceeds token limit")
	}
}
func (p *parser) fail(msg string) {
	if p.err == nil {
		p.err = schema.Diagnostic{Position: p.pos, Message: msg}
	}
}
func (p *parser) take(want string) {
	if p.err != nil {
		return
	}
	if p.text != want {
		p.fail("expected " + strconv.Quote(want))
		return
	}
	p.next()
}
func (p *parser) ident() string {
	if p.err != nil {
		return ""
	}
	if p.token != scanner.Ident {
		p.fail("expected identifier")
		return ""
	}
	s := p.text
	p.next()
	return s
}
func (p *parser) value() schema.Value {
	v := schema.Value{Position: p.pos}
	if p.text == "-" {
		p.next()
		v.Text = "-"
	}
	if v.Text == "-" && p.token != scanner.Int && p.token != scanner.Float {
		p.fail("expected number after minus")
		return v
	}
	switch p.token {
	case scanner.String:
		if v.Text != "" {
			p.fail("expected number")
			return v
		}
		v.Kind = "string"
		var err error
		v.Text, err = strconv.Unquote(p.text)
		if err != nil {
			p.fail("invalid string literal")
			return v
		}
	case scanner.Int, scanner.Float:
		v.Kind = "number"
		v.Text += p.text
		if !decimalNumber.MatchString(v.Text) {
			p.fail("expected decimal number without leading zeros")
			return v
		}
	case scanner.Ident:
		if p.text != "true" && p.text != "false" && p.text != "null" {
			p.fail("expected literal value")
			return v
		}
		v.Kind = p.text
		v.Text = p.text
	default:
		p.fail("expected literal value")
		return v
	}
	p.next()
	return v
}
func (p *parser) str() string {
	v := p.value()
	if v.Kind != "string" {
		p.fail("expected quoted string")
	}
	return v.Text
}

// Parse reads one FQL source file. Semantic checks run separately.
func Parse(file string, source []byte) (schema.Schema, error) {
	p := newParser(file, source)
	out := schema.Schema{}
	p.take("schema")
	out.Version = p.str()
	for p.err == nil && p.token != scanner.EOF {
		switch p.text {
		case "model":
			out.Models = append(out.Models, p.model())
		case "enum":
			e := schema.Enum{Position: p.pos}
			p.next()
			e.Name = p.ident()
			p.take("{")
			for p.err == nil && p.text != "}" {
				e.Values = append(e.Values, p.ident())
				if p.text == "," {
					p.next()
				}
			}
			p.take("}")
			out.Enums = append(out.Enums, e)
		case "object":
			o := schema.Object{Position: p.pos}
			p.next()
			o.Name = p.ident()
			p.take("{")
			p.take("fields")
			o.Fields = p.fields()
			p.take("}")
			out.Objects = append(out.Objects, o)
		default:
			p.fail("expected model, object, or enum")
		}
		if len(out.Models) > 100 {
			p.fail("at most 100 models are permitted per file")
		}
	}
	return out, p.err
}
func (p *parser) model() schema.Model {
	m := schema.Model{Position: p.pos}
	p.take("model")
	m.Name = p.ident()
	p.take("{")
	seen := map[string]bool{}
	for p.err == nil && p.text != "}" {
		section := p.ident()
		if seen[section] {
			p.fail("duplicate model section")
			break
		}
		seen[section] = true
		switch section {
		case "collection":
			p.take("=")
			m.Collection = p.str()
		case "fields":
			m.Fields = p.fields()
		case "access":
			p.take("{")
			for p.err == nil && p.text != "}" {
				a := schema.Policy{Position: p.pos}
				a.Operation = p.ident()
				p.take("=")
				a.Kind = p.ident()
				if a.Kind == "owner" {
					p.take("(")
					a.Field = p.ident()
					p.take(")")
				}
				m.Policies = append(m.Policies, a)
			}
			p.take("}")
		case "indexes":
			p.take("{")
			for p.err == nil && p.text != "}" {
				p.take("index")
				p.take("(")
				var fields []schema.IndexField
				for p.err == nil {
					f := schema.IndexField{Name: p.ident()}
					p.take(".")
					f.Direction = p.ident()
					fields = append(fields, f)
					if p.text != "," {
						break
					}
					p.next()
				}
				p.take(")")
				m.Indexes = append(m.Indexes, fields)
			}
			p.take("}")
		default:
			p.fail("expected collection, fields, access, or indexes")
		}
	}
	p.take("}")
	return m
}

func (p *parser) fields() []schema.Field {
	p.take("{")
	var fields []schema.Field
	for p.err == nil && p.text != "}" {
		f := schema.Field{Position: p.pos}
		f.Name = p.ident()
		f.Type = p.ident()
		if f.Type == "list" {
			p.take("<")
			f.Element = p.ident()
			p.take(">")
		}
		for p.err == nil && p.text == "." {
			p.next()
			mod := schema.Modifier{Position: p.pos}
			mod.Name = p.ident()
			p.take("(")
			for p.err == nil && p.text != ")" {
				mod.Args = append(mod.Args, p.value())
				if p.text != "," {
					break
				}
				p.next()
			}
			p.take(")")
			f.Modifiers = append(f.Modifiers, mod)
		}
		fields = append(fields, f)
	}
	p.take("}")
	return fields
}

// ParseConfig reads the versioned project configuration without executing code.
func ParseConfig(file string, source []byte) (schema.Config, error) {
	p := newParser(file, source)
	values := map[string]string{}
	for p.err == nil && p.token != scanner.EOF {
		key := p.ident()
		if _, ok := values[key]; ok {
			p.fail("duplicate configuration key")
			break
		}
		switch key {
		case "version", "target", "schemas", "output", "firebase":
		default:
			p.fail("unknown configuration key")
		}
		p.take("=")
		values[key] = p.str()
	}
	if p.err != nil {
		return schema.Config{}, p.err
	}
	for _, key := range []string{"version", "target", "schemas", "output", "firebase"} {
		if values[key] == "" {
			return schema.Config{}, fmt.Errorf("%s: missing configuration key %s", file, key)
		}
	}
	return schema.Config{Version: values["version"], Target: values["target"], Schemas: values["schemas"], Output: values["output"], Firebase: values["firebase"]}, nil
}
