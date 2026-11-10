// Package fileset stages text replacements and rolls back ordinary write failures.
package fileset

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/jonace-mpelule/fQL/internal/project"
	"os"
	"path/filepath"
	"sort"
)

type Replacement struct{ Before, After []byte }

func Apply(root string, files map[string]Replacement, check bool) error {
	var names []string
	for path, r := range files {
		if e := project.SafePath(root, path); e != nil {
			return e
		}
		current, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		if !bytes.Equal(current, r.Before) {
			return fmt.Errorf("source changed during formatting: %s", filepath.Base(path))
		}
		if !bytes.Equal(r.Before, r.After) {
			names = append(names, path)
		}
	}
	sort.Strings(names)
	if check {
		if len(names) > 0 {
			return fmt.Errorf("schema files need formatting (%d files); run fql format", len(names))
		}
		return nil
	}
	staged := map[string]string{}
	modes := map[string]os.FileMode{}
	defer func() {
		for _, name := range staged {
			os.Remove(name)
		}
	}()
	for _, path := range names {
		info, e := os.Stat(path)
		if e != nil {
			return e
		}
		modes[path] = info.Mode().Perm()
		f, e := os.CreateTemp(filepath.Dir(path), ".fql-format-*")
		if e != nil {
			return e
		}
		staged[path] = f.Name()
		_, writeErr := f.Write(files[path].After)
		syncErr := f.Sync()
		closeErr := f.Close()
		if e = errors.Join(writeErr, syncErr, closeErr); e != nil {
			return e
		}
		if e = os.Chmod(f.Name(), modes[path]); e != nil {
			return e
		}
	}
	var changed []string
	rollback := func(cause error) error {
		for _, path := range changed {
			if e := os.WriteFile(path, files[path].Before, modes[path]); e != nil {
				cause = errors.Join(cause, e)
			}
		}
		return cause
	}
	for _, path := range names {
		if e := project.SafePath(root, path); e != nil {
			return rollback(e)
		}
		current, e := os.ReadFile(path)
		if e != nil {
			return rollback(e)
		}
		if !bytes.Equal(current, files[path].Before) {
			return rollback(fmt.Errorf("schema changed during formatting"))
		}
		if e = os.Rename(staged[path], path); e != nil {
			return rollback(e)
		}
		changed = append(changed, path)
	}
	return nil
}
