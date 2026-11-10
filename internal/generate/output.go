package generate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/jonace-mpelule/fQL/internal/evolve"
	"github.com/jonace-mpelule/fQL/internal/project"
	"github.com/jonace-mpelule/fQL/internal/schema"
)

const CompilerVersion = "0.1.0-dev.2"

type manifest struct {
	Baseline *schema.Schema    `json:"baseline,omitempty"`
	Version  string            `json:"format"`
	Compiler string            `json:"compiler"`
	Schema   string            `json:"schema"`
	Files    map[string]string `json:"files"`
}

func digest(b []byte) string { s := sha256.Sum256(canonicalText(b)); return hex.EncodeToString(s[:]) }
func readManifest(path string) (manifest, error) {
	m := manifest{Files: map[string]string{}}
	b, e := readArtifact(path, 16<<20)
	if os.IsNotExist(e) {
		return m, nil
	}
	if e != nil {
		return m, e
	}
	if len(b) > 16<<20 {
		return m, fmt.Errorf("lockfile exceeds size limit")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e = d.Decode(&m); e != nil {
		return m, fmt.Errorf("invalid fql.lock: %w", e)
	}
	if d.Decode(new(any)) != io.EOF {
		return m, fmt.Errorf("invalid trailing lockfile content")
	}
	if (m.Version != "1" && m.Version != "2") || m.Files == nil || len(m.Files) > 1000 {
		return m, fmt.Errorf("unsupported or invalid lockfile")
	}
	return m, nil
}

// Write generates and owns only its explicitly recorded artifacts. Check mode never writes.
func Write(p project.Project, check bool) error {
	rules, indexes, e := Firestore(p.Schema)
	if e != nil {
		return e
	}
	if len(rules) > 250000 {
		return fmt.Errorf("generated Security Rules exceed the supported 250000-byte budget; reduce the schema")
	}
	absolute := map[string][]byte{filepath.Join(p.Output, "fql.dart"): Dart(p.Schema), filepath.Join(p.Firebase, "firestore.rules"): rules, filepath.Join(p.Firebase, "firestore.indexes.json"): indexes}
	files := map[string][]byte{}
	baseline := evolve.Canonical(p.Schema)
	next := manifest{Baseline: &baseline, Version: "2", Compiler: CompilerVersion, Schema: p.Fingerprint, Files: map[string]string{}}
	for path, b := range absolute {
		if e = project.SafePath(p.Root, path); e != nil {
			return e
		}
		rel, _ := filepath.Rel(p.Root, path)
		rel = filepath.ToSlash(rel)
		files[rel] = b
		next.Files[rel] = digest(b)
	}
	lockPath := filepath.Join(p.Root, "fQL", "fql.lock")
	if e = project.SafePath(p.Root, lockPath); e != nil {
		return e
	}
	busy := filepath.Join(p.Root, "fQL", ".generate.lock")
	if e = project.SafePath(p.Root, busy); e != nil {
		return e
	}
	if check {
		if _, e = os.Stat(busy); e == nil {
			return fmt.Errorf("generation is in progress")
		}
	} else {
		if e = os.Mkdir(busy, 0700); e != nil {
			return fmt.Errorf("cannot acquire generation lock (remove fQL/.generate.lock only if no generator is running): %w", e)
		}
		defer os.Remove(busy)
	}
	old, e := readManifest(lockPath)
	if e != nil {
		return e
	}
	for rel, hash := range old.Files {
		if filepath.IsAbs(rel) || filepath.ToSlash(filepath.Clean(rel)) != rel {
			return fmt.Errorf("unsafe artifact path in lockfile")
		}
		base := filepath.Base(rel)
		if base != "fql.dart" && base != "firestore.rules" && base != "firestore.indexes.json" {
			return fmt.Errorf("unrecognized artifact in lockfile")
		}
		path := filepath.Join(p.Root, filepath.FromSlash(rel))
		if e = project.SafePath(p.Root, path); e != nil {
			return e
		}
		if project.Inside(p.Source, path) {
			return fmt.Errorf("owned artifact overlaps schema source")
		}
		current, e := readArtifact(path, 16<<20)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if e == nil && digest(current) != hash {
			return fmt.Errorf("generated artifact was modified: %s; restore it before generation", rel)
		}
		if _, ok := files[rel]; !ok {
			files[rel] = nil
		}
	}
	for rel, b := range files {
		if b == nil {
			continue
		}
		if _, owned := old.Files[rel]; !owned {
			existing, e := readArtifact(filepath.Join(p.Root, filepath.FromSlash(rel)), 16<<20)
			if e != nil && !os.IsNotExist(e) {
				return e
			}
			if e == nil && !bytes.Equal(canonicalText(existing), canonicalText(b)) {
				return fmt.Errorf("refusing to overwrite unowned file: %s", rel)
			}
		}
	}
	lockBytes, e := json.MarshalIndent(next, "", "  ")
	if len(lockBytes) > 16<<20 {
		return fmt.Errorf("schema baseline exceeds 16 MiB compatibility metadata budget")
	}
	if e != nil {
		return e
	}
	files["fQL/fql.lock"] = append(lockBytes, '\n')
	var paths []string
	for rel := range files {
		if rel != "fQL/fql.lock" {
			paths = append(paths, rel)
		}
	}
	sort.Strings(paths)
	paths = append(paths, "fQL/fql.lock")
	if check {
		for _, rel := range paths {
			got, e := readArtifact(filepath.Join(p.Root, filepath.FromSlash(rel)), 16<<20)
			want := files[rel]
			if want == nil && os.IsNotExist(e) {
				continue
			}
			if e != nil || want == nil || !bytes.Equal(canonicalText(got), canonicalText(want)) {
				return fmt.Errorf("generated artifacts are stale: %s; run fql generate", rel)
			}
		}
		return nil
	}
	// Stage every replacement before touching any owned artifact.
	staged := map[string]string{}
	defer func() {
		for _, tmp := range staged {
			os.Remove(tmp)
		}
	}()
	previous := map[string][]byte{}
	existed := map[string]bool{}
	for _, rel := range paths {
		path := filepath.Join(p.Root, filepath.FromSlash(rel))
		if e = project.SafePath(p.Root, path); e != nil {
			return e
		}
		b, e := readArtifact(path, 16<<20)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		previous[rel] = b
		existed[rel] = e == nil
		if files[rel] == nil {
			continue
		}
		if e = os.MkdirAll(filepath.Dir(path), 0755); e != nil {
			return e
		}
		f, e := os.CreateTemp(filepath.Dir(path), ".fql-stage-*")
		if e != nil {
			return e
		}
		staged[rel] = f.Name()
		_, writeErr := f.Write(files[rel])
		syncErr := f.Sync()
		closeErr := f.Close()
		if e = errors.Join(writeErr, syncErr, closeErr); e != nil {
			return e
		}
		if e = os.Chmod(f.Name(), 0644); e != nil {
			return e
		}
	}
	changed := []string{}
	rollback := func(cause error) error {
		for i := len(changed) - 1; i >= 0; i-- {
			rel := changed[i]
			path := filepath.Join(p.Root, filepath.FromSlash(rel))
			var err error
			if existed[rel] {
				err = os.WriteFile(path, previous[rel], 0644)
			} else {
				err = os.Remove(path)
				if os.IsNotExist(err) {
					err = nil
				}
			}
			if err != nil {
				cause = errors.Join(cause, fmt.Errorf("rollback failed for %s: %w", rel, err))
			}
		}
		return cause
	}
	for _, rel := range paths {
		path := filepath.Join(p.Root, filepath.FromSlash(rel))
		if e = project.SafePath(p.Root, path); e != nil {
			return rollback(e)
		}
		if files[rel] == nil {
			e = os.Remove(path)
			if os.IsNotExist(e) {
				e = nil
			}
		} else {
			e = os.Rename(staged[rel], path)
		}
		if e != nil {
			return rollback(e)
		}
		changed = append(changed, rel)
	}
	return nil
}

// readArtifact bounds memory even for unexpectedly large handwritten output files.
func readArtifact(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("artifact must be a regular file within its size budget: %s", filepath.Base(path))
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("artifact exceeds size budget")
	}
	return b, nil
}

func canonicalText(b []byte) []byte { return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")) }
