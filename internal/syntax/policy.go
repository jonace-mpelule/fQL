package syntax

import (
	"github.com/jonace-mpelule/fQL/internal/schema"
	"text/scanner"
)

// expression uses a bounded precedence parser. Assignment is not an expression.
func (p *parser) expression(min int) *schema.Expression {
	p.expressionDepth++
	defer func() { p.expressionDepth-- }()
	if p.expressionDepth > 32 {
		p.fail("policy expression exceeds nesting limit 32")
		return nil
	}
	x := &schema.Expression{Position: p.pos}
	switch {
	case p.text == "not":
		x.Kind = "unary"
		x.Name = "not"
		p.next()
		x.Args = []*schema.Expression{p.expression(4)}
	case p.text == "(":
		p.next()
		x = p.expression(0)
		p.take(")")
	case p.token == scanner.String || p.token == scanner.Int || p.token == scanner.Float || p.text == "-" || p.text == "true" || p.text == "false" || p.text == "null":
		x.Kind = "literal"
		x.Value = p.value()
	default:
		x.Kind = "name"
		x.Name = p.ident()
		if p.text == "(" {
			x.Kind = "call"
			p.next()
			for p.err == nil && p.text != ")" {
				x.Args = append(x.Args, p.expression(0))
				if p.text != "," {
					break
				}
				p.next()
			}
			p.take(")")
		}
	}
	for p.err == nil && p.text == "." {
		pos := p.pos
		p.next()
		x = &schema.Expression{Kind: "member", Name: p.ident(), Args: []*schema.Expression{x}, Position: pos}
	}
	for p.err == nil {
		op := p.text
		prec := 0
		switch op {
		case "or":
			prec = 1
		case "and":
			prec = 2
		case "=", "!", "<", ">":
			prec = 3
		}
		if prec == 0 || prec < min {
			break
		}
		pos := p.pos
		p.next()
		if op == "=" || op == "!" {
			p.take("=")
			op += "="
		} else if (op == "<" || op == ">") && p.text == "=" {
			p.next()
			op += "="
		}
		right := p.expression(prec + 1)
		x = &schema.Expression{Kind: "binary", Name: op, Args: []*schema.Expression{x, right}, Position: pos}
	}
	return x
}
