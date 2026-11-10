package project_test

import (
	"bytes"
	"context"
	"github.com/jonace-mpelule/fQL/internal/cli"
	"github.com/jonace-mpelule/fQL/internal/generate"
	"github.com/jonace-mpelule/fQL/internal/project"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func initialized(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if e := project.Init(root); e != nil {
		t.Fatal(e)
	}
	return root
}
func load(t *testing.T, root string) project.Project {
	t.Helper()
	p, e := project.Load(root)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func read(t *testing.T, path string) []byte {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func write(t *testing.T, path string, b []byte) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, b, 0644); e != nil {
		t.Fatal(e)
	}
}
func TestWorkflowAndDeterminism(t *testing.T) {
	root := initialized(t)
	p := load(t, root)
	if e := generate.Write(p, true); e == nil {
		t.Fatal("missing artifacts should be stale")
	}
	if _, e := os.Stat(p.Output); !os.IsNotExist(e) {
		t.Fatal("check mode wrote output")
	}
	if e := generate.Write(p, false); e != nil {
		t.Fatal(e)
	}
	if e := generate.Write(p, true); e != nil {
		t.Fatal(e)
	}
	before := read(t, filepath.Join(p.Output, "fql.dart"))
	if e := generate.Write(p, false); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(before, read(t, filepath.Join(p.Output, "fql.dart"))) {
		t.Fatal("nondeterministic output")
	}
	// Source changes do not make a check write the lockfile.
	lock := filepath.Join(root, "fQL/fql.lock")
	original := read(t, lock)
	source := filepath.Join(root, "fQL/schemas/users.fQL")
	write(t, source, append(read(t, source), []byte("\n// revision\n")...))
	p = load(t, root)
	if e := generate.Write(p, true); e == nil {
		t.Fatal("expected stale metadata")
	}
	if !bytes.Equal(original, read(t, lock)) {
		t.Fatal("check modified lockfile")
	}
	if e := generate.Write(p, false); e != nil {
		t.Fatal(e)
	}
}
func TestRecursiveDiscoveryAndDuplicates(t *testing.T) {
	root := initialized(t)
	source := filepath.Join(root, "fQL/schemas/users.fQL")
	b := read(t, source)
	nested := filepath.Join(root, "fQL/schemas/nested/users.fQL")
	write(t, nested, b)
	if _, e := project.Load(root); e == nil || !strings.Contains(e.Error(), "duplicate model") {
		t.Fatalf("expected duplicate diagnostic, got %v", e)
	}
	if e := os.Remove(source); e != nil {
		t.Fatal(e)
	}
	if len(load(t, root).Schema.Models) != 2 {
		t.Fatal("nested file not discovered")
	}
}
func TestOutputOwnership(t *testing.T) {
	root := initialized(t)
	p := load(t, root)
	path := filepath.Join(p.Output, "fql.dart")
	write(t, path, []byte("handwritten"))
	if e := generate.Write(p, false); e == nil {
		t.Fatal("overwrote unowned file")
	}
	if string(read(t, path)) != "handwritten" {
		t.Fatal("unowned file changed")
	}
	os.Remove(path)
	if e := generate.Write(p, false); e != nil {
		t.Fatal(e)
	}
	write(t, path, []byte("edited"))
	if e := generate.Write(p, false); e == nil {
		t.Fatal("overwrote modified artifact")
	}
	if string(read(t, path)) != "edited" {
		t.Fatal("edited file changed")
	}
}
func TestChangedOutputPaths(t *testing.T) {
	root := initialized(t)
	p := load(t, root)
	if e := generate.Write(p, false); e != nil {
		t.Fatal(e)
	}
	hand := filepath.Join(p.Output, "custom.dart")
	write(t, hand, []byte("custom"))
	cfg := filepath.Join(root, "fQL/fql.config")
	write(t, cfg, bytes.ReplaceAll(read(t, cfg), []byte("../lib/data/fql"), []byte("../lib/generated")))
	next := load(t, root)
	if e := generate.Write(next, false); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(p.Output, "fql.dart")); !os.IsNotExist(e) {
		t.Fatal("stale owned output retained")
	}
	if string(read(t, hand)) != "custom" {
		t.Fatal("unrelated file changed")
	}
}
func TestUnsafeConfiguration(t *testing.T) {
	for _, out := range []string{"../../escape", "schemas", ".", "../firebase/generated"} {
		t.Run(out, func(t *testing.T) {
			root := initialized(t)
			cfg := filepath.Join(root, "fQL/fql.config")
			write(t, cfg, bytes.ReplaceAll(read(t, cfg), []byte("../lib/data/fql"), []byte(out)))
			if _, e := project.Load(root); e == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
}
func TestSymlinksRejected(t *testing.T) {
	root := initialized(t)
	outside := t.TempDir()
	path := filepath.Join(root, "fQL/schemas/link")
	if e := os.Symlink(outside, path); e != nil {
		t.Skip("symlink creation unavailable")
	}
	if _, e := project.Load(root); e == nil {
		t.Fatal("symlink accepted")
	}
}
func TestInitDoesNotOverwrite(t *testing.T) {
	root := initialized(t)
	path := filepath.Join(root, "fQL/schemas/users.fQL")
	before := read(t, path)
	if e := project.Init(root); e == nil {
		t.Fatal("reinitialization accepted")
	}
	if !bytes.Equal(before, read(t, path)) {
		t.Fatal("existing schema changed")
	}
}
func TestInvalidSchemaPreservesOutput(t *testing.T) {
	root := initialized(t)
	p := load(t, root)
	if e := generate.Write(p, false); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(p.Output, "fql.dart")
	before := read(t, path)
	write(t, filepath.Join(root, "fQL/schemas/users.fQL"), []byte("invalid"))
	if _, e := project.Load(root); e == nil {
		t.Fatal("invalid schema accepted")
	}
	if !bytes.Equal(before, read(t, path)) {
		t.Fatal("output changed")
	}
}
func TestWatchCancellation(t *testing.T) {
	root := initialized(t)
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	var out, err bytes.Buffer
	if code := cli.RunContext(ctx, []string{"generate", "--watch", "--project", root}, &out, &err, "test"); code != 0 {
		t.Fatalf("watch failed: %s", err.String())
	}
	if !strings.Contains(out.String(), "watching") {
		t.Fatal("watch did not generate")
	}
	if e := generate.Write(load(t, root), true); e != nil {
		t.Fatal(e)
	}
}
func TestCLIFailures(t *testing.T) {
	root := initialized(t)
	for _, args := range [][]string{{"validate", "--project", root}, {"generate", "--project", root}, {"generate", "--project", root, "--check"}} {
		var out, err bytes.Buffer
		if code := cli.Run(args, &out, &err, "test"); code != 0 {
			t.Fatalf("%v failed: %s", args, err.String())
		}
	}
	for _, args := range [][]string{{"init", "--target", "typescript"}, {"generate", "--watch", "--check"}, {"validate", "unexpected"}, {"generate", "--unknown"}} {
		var out, err bytes.Buffer
		if code := cli.Run(args, &out, &err, "test"); code != 2 {
			t.Fatalf("%v code %d, want 2", args, code)
		}
	}
}

func TestManifestCannotDeleteUnrelatedFiles(t *testing.T) {
	root := initialized(t)
	p := load(t, root)
	if e := generate.Write(p, false); e != nil {
		t.Fatal(e)
	}
	lock := filepath.Join(root, "fQL/fql.lock")
	write(t, lock, []byte(`{"format":"1","compiler":"x","schema":"x","files":{"../outside/fql.dart":"x"}}`))
	if e := generate.Write(p, false); e == nil {
		t.Fatal("unsafe manifest accepted")
	}
	write(t, lock, []byte(`{"format":"1","compiler":"x","schema":"x","files":{}} trailing`))
	if e := generate.Write(p, false); e == nil {
		t.Fatal("trailing manifest content accepted")
	}
}

func TestGenerationLockAndStagingFailurePreserveFiles(t *testing.T) {
	root := initialized(t)
	p := load(t, root)
	if e := generate.Write(p, false); e != nil {
		t.Fatal(e)
	}
	before := read(t, filepath.Join(p.Output, "fql.dart"))
	busy := filepath.Join(root, "fQL/.generate.lock")
	if e := os.Mkdir(busy, 0700); e != nil {
		t.Fatal(e)
	}
	if e := generate.Write(p, false); e == nil {
		t.Fatal("concurrent generation accepted")
	}
	if e := generate.Write(p, true); e == nil {
		t.Fatal("check accepted in-progress generation")
	}
	os.Remove(busy)
	cfg := filepath.Join(root, "fQL/fql.config")
	write(t, cfg, bytes.ReplaceAll(read(t, cfg), []byte("../firebase/generated"), []byte("../blocked/generated")))
	write(t, filepath.Join(root, "blocked"), []byte("handwritten"))
	next, err := project.Load(root)
	if err == nil {
		err = generate.Write(next, false)
	}
	if err == nil {
		t.Fatal("blocked output directory accepted")
	}
	if !bytes.Equal(before, read(t, filepath.Join(p.Output, "fql.dart"))) {
		t.Fatal("prior output changed on staging failure")
	}
	if string(read(t, filepath.Join(root, "blocked"))) != "handwritten" {
		t.Fatal("unrelated file changed")
	}
}

func TestLineEndingsRemainPortable(t *testing.T) {
	root := initialized(t)
	p := load(t, root)
	if e := generate.Write(p, false); e != nil {
		t.Fatal(e)
	}
	for _, rel := range []string{"fQL/fql.config", "fQL/schemas/users.fQL", "lib/data/fql/fql.dart", "fQL/fql.lock"} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		write(t, path, bytes.ReplaceAll(read(t, path), []byte("\n"), []byte("\r\n")))
	}
	next := load(t, root)
	if next.Fingerprint != p.Fingerprint {
		t.Fatal("line endings changed schema identity")
	}
	if e := generate.Write(next, true); e != nil {
		t.Fatal(e)
	}
}

func TestWatchRecoversAfterSourceError(t *testing.T) {
	root := initialized(t)
	source := filepath.Join(root, "fQL/schemas/users.fQL")
	original := read(t, source)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out, diagnostics bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- cli.RunContext(ctx, []string{"generate", "--watch", "--project", root}, &out, &diagnostics, "test")
	}()
	// Each transition spans more than one polling interval; buffers are read only after exit.
	time.Sleep(450 * time.Millisecond)
	write(t, source, []byte("invalid schema"))
	time.Sleep(450 * time.Millisecond)
	write(t, source, original)
	time.Sleep(450 * time.Millisecond)
	cancel()
	if code := <-done; code != 0 {
		t.Fatalf("watch exit %d", code)
	}
	if !strings.Contains(diagnostics.String(), "expected") {
		t.Fatalf("missing source error: %s", diagnostics.String())
	}
	if strings.Count(out.String(), "watching") < 2 {
		t.Fatalf("watch did not recover: %s", out.String())
	}
}

func TestDiffBaselineAndFormatCommands(t *testing.T) {
	root := initialized(t)
	var out, diag bytes.Buffer
	for _, args := range [][]string{{"format", "--project", root}, {"format", "--check", "--project", root}, {"generate", "--project", root}, {"diff", "--check", "--json", "--project", root}} {
		out.Reset()
		diag.Reset()
		if code := cli.Run(args, &out, &diag, "test"); code != 0 {
			t.Fatalf("%v failed: %s", args, diag.String())
		}
	}
	file := filepath.Join(root, "fQL/schemas/users.fQL")
	write(t, file, bytes.ReplaceAll(read(t, file), []byte("maxLength(100)"), []byte("maxLength(90)")))
	out.Reset()
	diag.Reset()
	if code := cli.Run([]string{"diff", "--check", "--json", "--project", root}, &out, &diag, "test"); code != 3 {
		t.Fatalf("expected breaking change, got %d %s", code, diag.String())
	}
	if !strings.Contains(out.String(), `"breaking":true`) {
		t.Fatalf("missing breaking report: %s", out.String())
	}
}
