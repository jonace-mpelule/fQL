package generate_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonace-mpelule/fQL/internal/cli"
	"github.com/jonace-mpelule/fQL/internal/generate"
	"github.com/jonace-mpelule/fQL/internal/project"
)

func TestTypeScriptWorkflow(t *testing.T) {
	root := t.TempDir()
	var out, diagnostics bytes.Buffer
	if code := cli.Run([]string{"init", "--target", "typescript", "--project", root}, &out, &diagnostics, "test"); code != 0 {
		t.Fatal(diagnostics.String())
	}
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if p.Target != "typescript" || p.Output != filepath.Join(p.Root, "src/data/fql") {
		t.Fatal("wrong target or output")
	}
	if err := generate.Write(p, true); err == nil {
		t.Fatal("missing output should be stale")
	}
	if err := generate.Write(p, false); err != nil {
		t.Fatal(err)
	}
	if err := generate.Write(p, true); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(p.Output, "fql.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(before, []byte("from 'firebase/firestore'")) {
		t.Fatal("missing SDK integration")
	}
	if err := generate.Write(p, false); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(p.Output, "fql.ts"))
	if !bytes.Equal(before, after) {
		t.Fatal("nondeterministic TypeScript output")
	}
	// Target switching cleans only owned files and retains handwritten siblings.
	handwritten := filepath.Join(p.Output, "custom.ts")
	if err := os.WriteFile(handwritten, []byte("export const custom = true;"), 0644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "fQL/fql.config")
	config, _ := os.ReadFile(configPath)
	if err := os.WriteFile(configPath, bytes.ReplaceAll(config, []byte(`"typescript"`), []byte(`"dart"`)), 0644); err != nil {
		t.Fatal(err)
	}
	p, err = project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := generate.Write(p, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(p.Output, "fql.ts")); !os.IsNotExist(err) {
		t.Fatal("stale TypeScript file survived target switch")
	}
	if _, err := os.Stat(handwritten); err != nil {
		t.Fatal("handwritten file removed")
	}
}

func TestTypeScriptNameDiagnostics(t *testing.T) {
	for _, source := range []string{
		`schema "0.1" model Promise { collection="promises" fields { label string.required() } }`,
		`schema "0.1" model Sample { collection="samples" fields { constructor string.required() } }`,
	} {
		root := t.TempDir()
		if err := project.InitTarget(root, "typescript"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "fQL/schemas/conflict.fQL"), []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := project.Load(root); err == nil || !strings.Contains(err.Error(), "TypeScript") || !strings.Contains(err.Error(), "conflict.fQL:") {
			t.Fatalf("missing source diagnostic: %v", err)
		}
	}
}
