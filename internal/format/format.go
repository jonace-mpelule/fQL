// Package format formats validated FQL without dropping comments or rewriting literals.
package format

import (
	"fmt"
	"github.com/jonace-mpelule/fQL/internal/fileset"
	"github.com/jonace-mpelule/fQL/internal/project"
	"github.com/jonace-mpelule/fQL/internal/syntax"
	"os"
	"path/filepath"
	"strings"
	"text/scanner"
)

type token struct {
	text   string
	offset int
	kind   rune
}

func tokens(source []byte) []token {
	var s scanner.Scanner
	s.Init(strings.NewReader(string(source)))
	s.Mode = scanner.ScanIdents | scanner.ScanStrings | scanner.ScanInts | scanner.ScanFloats | scanner.ScanComments
	var out []token
	for k := s.Scan(); k != scanner.EOF; k = s.Scan() {
		out = append(out, token{s.TokenText(), s.Position.Offset, k})
	}
	return out
}

// Source requires a syntactically valid file; semantic validation belongs to Project.
func Source(file string, source []byte) ([]byte, error) {
	ast, e := syntax.Parse(file, source)
	if e != nil {
		return nil, e
	}
	starts := map[int]bool{}
	for _, c := range ast.Claims {
		starts[c.Position.Offset] = true
	}
	for _, m := range ast.Models {
		for _, f := range m.Fields {
			starts[f.Position.Offset] = true
		}
		for _, a := range m.Policies {
			starts[a.Position.Offset] = true
		}
	}
	for _, o := range ast.Objects {
		for _, f := range o.Fields {
			starts[f.Position.Offset] = true
		}
	}
	var lines []string
	line := ""
	preserveTrailing := false
	depth := 0
	contexts := []string{}
	paren := 0
	prev := ""
	flush := func() {
		if strings.TrimSpace(line) != "" {
			content := strings.TrimLeft(line, " \t")
			if !preserveTrailing {
				content = strings.TrimRight(content, " \t")
			}
			lines = append(lines, strings.Repeat("  ", depth)+content)
			preserveTrailing = false
			line = ""
		}
	}
	add := func(text string) { line += text }
	for _, t := range tokens(source) {
		text := t.text
		ctx := ""
		if len(contexts) > 0 {
			ctx = contexts[len(contexts)-1]
		}
		if t.kind == scanner.Comment {
			preserveTrailing = true
			if strings.HasPrefix(text, "//") {
				if line != "" {
					line += " "
				}
				line += text
				flush()
			} else {
				if line != "" {
					line += " "
				}
				line += text
				flush()
			}
			prev = ""
			continue
		}
		if (ctx == "model" && (text == "fields" || text == "access" || text == "indexes" || text == "collection")) || starts[t.offset] || ctx == "enum" && t.kind == scanner.Ident || ctx == "indexes" && text == "index" && paren == 0 {
			flush()
		}
		switch text {
		case "{":
			name := strings.Fields(line)
			kind := ""
			if len(name) > 0 {
				kind = name[0]
			}
			if line != "" {
				line = strings.TrimRight(line, " ") + " "
			}
			add("{")
			flush()
			depth++
			contexts = append(contexts, kind)
		case "}":
			flush()
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("invalid formatter nesting")
			}
			add("}")
			flush()
			contexts = contexts[:len(contexts)-1]
			if depth == 0 {
				lines = append(lines, "")
			}
		case "(":
			add("(")
			paren++
		case ")":
			line = strings.TrimRight(line, " ")
			add(")")
			paren--
		case ".":
			line = strings.TrimRight(line, " ")
			add(text)
		case "<", ">", "!":
			line = strings.TrimRight(line, " ")
			if text != "!" && paren == 0 && ctx == "fields" {
				add(text)
			} else {
				add(" " + text + " ")
			}
		case ",":
			line = strings.TrimRight(line, " ")
			add(", ")
		case "=":
			if prev == "=" || prev == "!" || prev == "<" || prev == ">" {
				line = strings.TrimRight(line, " ") + "= "
				break
			}
			line = strings.TrimRight(line, " ")
			add(" = ")
		case "-":
			if line != "" && !strings.HasSuffix(line, "(") && !strings.HasSuffix(line, " ") {
				add(" ")
			}
			add("-")
		default:
			if line != "" && !strings.HasSuffix(line, " ") && prev != "." && prev != "(" && prev != "<" && prev != "-" {
				add(" ")
			}
			add(text)
			if depth == 0 && t.kind == scanner.String {
				flush()
				lines = append(lines, "")
			}
		}
		prev = text
	}
	flush()
	out := []byte(strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n")
	if _, e = syntax.Parse(file, out); e != nil {
		return nil, fmt.Errorf("formatter produced invalid syntax: %w", e)
	}
	before, after := tokens(source), tokens(out)
	if len(before) != len(after) {
		return nil, fmt.Errorf("formatter token preservation failed")
	}
	for i := range before {
		if before[i].text != after[i].text {
			return nil, fmt.Errorf("formatter token preservation failed")
		}
	}
	return out, nil
}

// Project validates all files before preparing or writing any replacement.
func Project(root string, check bool) error {
	p, e := project.Load(root)
	if e != nil {
		return e
	}
	busy := filepath.Join(p.Root, "fQL/.generate.lock")
	if e = project.SafePath(p.Root, busy); e != nil {
		return e
	}
	if !check {
		if e = os.Mkdir(busy, 0700); e != nil {
			return fmt.Errorf("project is busy: %w", e)
		}
		defer os.Remove(busy)
	}
	files := map[string]fileset.Replacement{}
	e = filepath.WalkDir(p.Source, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !project.IsSchemaPath(path) {
			return nil
		}
		if e = project.SafePath(p.Root, path); e != nil {
			return e
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		after, err := Source(path, b)
		if err != nil {
			return err
		}
		files[path] = fileset.Replacement{Before: b, After: after}
		return nil
	})
	if e != nil {
		return e
	}
	// Revalidate after acquiring the lock and verify the same input fingerprint.
	latest, e := project.Load(root)
	if e != nil {
		return e
	}
	if latest.Fingerprint != p.Fingerprint {
		return fmt.Errorf("schema changed during formatting; retry")
	}
	return fileset.Apply(p.Root, files, check)
}
