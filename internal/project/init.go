package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DefaultConfig = `version = "0.1"
target = "dart"
schemas = "schemas"
output = "../lib/data/fql"
firebase = "../firebase/generated"
`
const UserSchema = `schema "0.1"

model User {
  collection = "users"
  fields {
    name string.required().minLength(1).maxLength(100)
    dateOfBirth date.optional()
    bio string.optional().nullable().maxLength(500)
    active bool.required().default(true)
    rating float.optional().min(0).max(5)
    status string.required().oneOf("active", "paused").default("active")
  }
  access {
    read = owner(id)
    create = owner(id)
    update = owner(id)
    delete = owner(id)
  }
}
`
const MessageSchema = `schema "0.1"

model Message {
  collection = "messages"
  fields {
    ownerId string.required().immutable()
    body string.required().minLength(1).maxLength(4000)
    createdAt timestamp.required().immutable()
    priority int.required().min(0).max(10).default(0)
  }
  access {
    read = owner(ownerId)
    create = owner(ownerId)
    update = owner(ownerId)
    delete = owner(ownerId)
  }
  indexes {
    index(ownerId.asc, createdAt.desc)
  }
}
`

// Init creates the initial files exclusively, rolling back its own files on failure.
func Init(root string) error { return InitTarget(root, "dart") }

// InitTarget scaffolds one supported client target without overwriting files.
func InitTarget(root, target string) error {
	config := DefaultConfig
	switch target {
	case "dart":
	case "typescript":
		config = strings.ReplaceAll(strings.ReplaceAll(config, `target = "dart"`, `target = "typescript"`), "../lib/data/fql", "../src/data/fql")
	default:
		return fmt.Errorf("unsupported target %q; use dart or typescript", target)
	}
	abs, e := filepath.Abs(root)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(abs, 0755); e != nil {
		return e
	}
	root, e = filepath.EvalSymlinks(abs)
	if e != nil {
		return e
	}
	files := []struct{ path, content string }{{"fQL/fql.config", config}, {"fQL/schemas/users.fql", UserSchema}, {"fQL/schemas/messages.fql", MessageSchema}}
	// Legacy sample spellings must also be protected when scaffolding.
	for _, name := range []string{"users.fQL", "messages.fQL"} {
		path := filepath.Join(root, "fQL", "schemas", name)
		if e = SafePath(root, path); e != nil {
			return e
		}
		if _, e = os.Lstat(path); e == nil {
			return fmt.Errorf("refusing to duplicate existing schema %s", name)
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	for _, f := range files {
		path := filepath.Join(root, filepath.FromSlash(f.path))
		if e = SafePath(root, path); e != nil {
			return e
		}
		if _, e = os.Lstat(path); e == nil {
			return fmt.Errorf("refusing to overwrite %s", f.path)
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	created := []string{}
	rollback := func() {
		for _, p := range created {
			os.Remove(p)
		}
	}
	for _, item := range files {
		path := filepath.Join(root, filepath.FromSlash(item.path))
		if e = os.MkdirAll(filepath.Dir(path), 0755); e != nil {
			rollback()
			return e
		}
		f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if e != nil {
			rollback()
			return e
		}
		created = append(created, path)
		_, writeErr := f.WriteString(item.content)
		closeErr := f.Close()
		if writeErr != nil {
			rollback()
			return writeErr
		}
		if closeErr != nil {
			rollback()
			return closeErr
		}
	}
	return nil
}
