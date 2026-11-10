package format_test

import (
	"bytes"
	formatter "github.com/jonace-mpelule/fQL/internal/format"
	"github.com/jonace-mpelule/fQL/internal/project"
	"os"
	"path/filepath"
	"testing"
)

func TestSourceCommentsAndIdempotence(t *testing.T) {
	source := []byte(`// Project note
schema "0.1"
enum State{ open,closed }
/* embedded value */ object Info{fields{label string.required().default("not // a comment")}}
model Example {collection="examples" fields{ info Info.required() tags list<string>.optional().maxItems(8) count int.required().min(-1).default(0) // preserve this
} access{read=authenticated}}
`)
	formatted, e := formatter.Source("test.fQL", source)
	if e != nil {
		t.Fatal(e)
	}
	again, e := formatter.Source("test.fQL", formatted)
	if e != nil || !bytes.Equal(formatted, again) {
		t.Fatalf("not idempotent: %v\n%s", e, formatted)
	}
	for _, text := range []string{"// Project note", "/* embedded value */", "not // a comment", "// preserve this", "tags list<string>.optional().maxItems(8)"} {
		if !bytes.Contains(formatted, []byte(text)) {
			t.Fatalf("lost %q", text)
		}
	}
}
func TestInvalidGraphNeverRewritesAnyFile(t *testing.T) {
	root := t.TempDir()
	if e := project.Init(root); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(root, "fQL/schemas/users.fQL")
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(root, "fQL/schemas/broken.fQL"), []byte(`schema "0.1" model Broken {fields{value Unknown.required()}}`), 0644); e != nil {
		t.Fatal(e)
	}
	if e = formatter.Project(root, false); e == nil {
		t.Fatal("invalid graph accepted")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("valid file changed despite invalid sibling")
	}
}
func TestCheckAndPermissionPreservation(t *testing.T) {
	root := t.TempDir()
	project.Init(root)
	path := filepath.Join(root, "fQL/schemas/users.fQL")
	source := []byte(`schema "0.1" model User{collection="users" fields{label string.required()}}`)
	os.WriteFile(path, source, 0640)
	os.Chmod(path, 0640)
	if e := formatter.Project(root, true); e == nil {
		t.Fatal("check accepted unformatted schema")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(source, after) {
		t.Fatal("check wrote source")
	}
	if e := formatter.Project(root, false); e != nil {
		t.Fatal(e)
	}
	if e := formatter.Project(root, true); e != nil {
		t.Fatal(e)
	}
}

func TestCommentsKeepTrailingSpacesAndModifierContinuations(t *testing.T) {
	source := []byte("schema \"0.1\"\nmodel Example { collection=\"examples\" fields{ label string. // note   \nrequired() } }\n")
	result, e := formatter.Source("comments.fQL", source)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Contains(result, []byte("// note   \n")) {
		t.Fatal("comment text changed")
	}
}

func FuzzFormatting(f *testing.F) {
	f.Add(project.UserSchema)
	f.Add("schema \"0.1\"\n// note\nmodel Example{collection=\"examples\" fields{label string.required()}}")
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 1<<20 {
			return
		}
		result, e := formatter.Source("fuzz.fQL", []byte(source))
		if e != nil {
			return
		}
		again, e := formatter.Source("fuzz.fQL", result)
		if e != nil || !bytes.Equal(result, again) {
			t.Fatalf("formatting was not idempotent: %v", e)
		}
	})
}
