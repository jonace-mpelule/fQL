// Package project loads a bounded, self-contained FQL project.
package project

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/jonace-mpelule/fQL/internal/schema"
	"github.com/jonace-mpelule/fQL/internal/syntax"
	"github.com/jonace-mpelule/fQL/internal/validate"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Project struct {
	Root, Source, Output, Firebase, Fingerprint string
	Schema                                      schema.Schema
}

func Inside(root, path string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// SafePath rejects symlink components and paths outside the project.
func SafePath(root, path string) error {
	if !Inside(root, path) {
		return fmt.Errorf("path must remain inside the project")
	}
	rel, _ := filepath.Rel(root, path)
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		s, e := os.Lstat(current)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if s.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not allowed in managed project paths")
		}
	}
	return nil
}
func read(path string, limit int64) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil {
		return nil, e
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s: exceeds size limit", filepath.Base(path))
	}
	return b, nil
}
func Load(root string) (Project, error) {
	var p Project
	abs, e := filepath.Abs(root)
	if e != nil {
		return p, e
	}
	p.Root, e = filepath.EvalSymlinks(abs)
	if e != nil {
		return p, e
	}
	cfgPath := filepath.Join(p.Root, "fQL", "fql.config")
	if e = SafePath(p.Root, cfgPath); e != nil {
		return p, e
	}
	b, e := read(cfgPath, syntax.MaxSourceBytes)
	if e != nil {
		return p, e
	}
	cfg, e := syntax.ParseConfig("fQL/fql.config", b)
	if e != nil {
		return p, e
	}
	if cfg.Version != validate.Version {
		return p, fmt.Errorf("unsupported configuration version %q", cfg.Version)
	}
	if cfg.Target != "dart" {
		return p, fmt.Errorf("unsupported generator target %q; v0.1 supports dart", cfg.Target)
	}
	paths := []string{cfg.Schemas, cfg.Output, cfg.Firebase}
	resolved := []*string{&p.Source, &p.Output, &p.Firebase}
	for i, s := range paths {
		if filepath.IsAbs(s) || strings.Contains(s, "\\") {
			return p, fmt.Errorf("configuration paths must be relative and use forward slashes")
		}
		*resolved[i] = filepath.Clean(filepath.Join(filepath.Dir(cfgPath), filepath.FromSlash(s)))
		if e = SafePath(p.Root, *resolved[i]); e != nil {
			return p, e
		}
	}
	if p.Source == p.Root || p.Source == filepath.Dir(cfgPath) {
		return p, fmt.Errorf("schema directory must be separate from the project configuration")
	}
	for _, out := range []string{p.Output, p.Firebase} {
		if Inside(out, p.Source) || Inside(p.Source, out) || Inside(out, cfgPath) {
			return p, fmt.Errorf("generated outputs must not overlap schema sources or project configuration")
		}
	}
	if Inside(p.Output, p.Firebase) || Inside(p.Firebase, p.Output) {
		return p, fmt.Errorf("Dart and Firebase output directories must not overlap")
	}
	var files []string
	total := 0
	entries := 0
	e = filepath.WalkDir(p.Source, func(path string, d fs.DirEntry, walkErr error) error {
		entries++
		if entries > 8192 {
			return fmt.Errorf("schema tree exceeds 8192 entry limit")
		}
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("schema directory must not contain symlinks")
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("schema directory must contain regular files only")
		}
		if filepath.Ext(path) == ".fQL" {
			files = append(files, path)
			if len(files) > 128 {
				return fmt.Errorf("schema exceeds 128 file limit")
			}
		}
		return nil
	})
	if e != nil {
		return p, e
	}
	if len(files) == 0 {
		return p, fmt.Errorf("no .fQL schemas found under %s", cfg.Schemas)
	}
	sort.Strings(files)
	hash := sha256.New()
	hash.Write(bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")))
	p.Schema.Version = validate.Version
	for _, file := range files {
		b, e := read(file, syntax.MaxSourceBytes)
		if e != nil {
			return p, e
		}
		total += len(b)
		if total > 16<<20 {
			return p, fmt.Errorf("combined schema exceeds 16 MiB")
		}
		rel, _ := filepath.Rel(p.Root, file)
		rel = filepath.ToSlash(rel)
		normalized := bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
		fmt.Fprintf(hash, "\x00%s\x00%d\x00", rel, len(normalized))
		hash.Write(normalized)
		s, e := syntax.Parse(rel, b)
		if e != nil {
			return p, e
		}
		if s.Version != validate.Version {
			return p, fmt.Errorf("%s: unsupported schema version %q", rel, s.Version)
		}
		p.Schema.Models = append(p.Schema.Models, s.Models...)
		p.Schema.Enums = append(p.Schema.Enums, s.Enums...)
		p.Schema.Objects = append(p.Schema.Objects, s.Objects...)
	}
	sort.Slice(p.Schema.Models, func(i, j int) bool { return p.Schema.Models[i].Name < p.Schema.Models[j].Name })
	sort.Slice(p.Schema.Enums, func(i, j int) bool { return p.Schema.Enums[i].Name < p.Schema.Enums[j].Name })
	sort.Slice(p.Schema.Objects, func(i, j int) bool { return p.Schema.Objects[i].Name < p.Schema.Objects[j].Name })
	p.Schema, e = validate.Schema(p.Schema)
	if e != nil {
		return p, e
	}
	p.Fingerprint = hex.EncodeToString(hash.Sum(nil))
	return p, nil
}
