package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jonace-mpelule/fQL/internal/cli"
)

// Only durations vary between runs; spacing, wording, and path order are contractual.
var terminalDuration = regexp.MustCompile(`[ \t]+[0-9]+\.[0-9] ms`)

func TestTerminalPresentation(t *testing.T) {
	for _, target := range []string{"dart", "typescript"} {
		t.Run(target, func(t *testing.T) {
			root := t.TempDir()
			steps := []struct {
				name string
				args []string
				code int
			}{
				{"init-" + target, []string{"init", "--target", target}, 0},
				{"validate", []string{"validate"}, 0},
				{"generate-" + target, []string{"generate"}, 0},
				{"generate-check", []string{"generate", "--check"}, 0},
				{"format-check", []string{"format", "--check"}, 0},
				{"diff", []string{"diff"}, 0},
				{"init-error", []string{"init", "--target", "invalid"}, 2},
			}
			for _, step := range steps {
				t.Run(step.name, func(t *testing.T) {
					var out, diagnostics bytes.Buffer
					args := append(append([]string{}, step.args...), "--project", root)
					code := cli.Run(args, &out, &diagnostics, "test")
					if code != step.code {
						t.Fatalf("exit %d: %s", code, &diagnostics)
					}
					actual := out.String()
					if code != 0 {
						if out.Len() != 0 {
							t.Fatalf("error leaked stdout: %q", &out)
						}
						actual = diagnostics.String()
					} else if diagnostics.Len() != 0 {
						t.Fatalf("success leaked stderr: %q", &diagnostics)
					}
					for _, line := range strings.Split(actual, "\n") {
						if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
							t.Fatalf("indented output: %q", line)
						}
					}
					actual = terminalDuration.ReplaceAllString(actual, " <time>")
					expected, err := os.ReadFile(filepath.Join("testdata", step.name+".txt"))
					if err != nil {
						t.Fatal(err)
					}
					if actual != string(expected) {
						t.Fatalf("output mismatch\ngot:\n%s\nwant:\n%s", actual, expected)
					}
				})
			}
		})
	}
}

func TestConfiguredOutputPaths(t *testing.T) {
	root := t.TempDir()
	var out, diagnostics bytes.Buffer
	if code := cli.Run([]string{"init", "--target", "typescript", "--project", root}, &out, &diagnostics, "test"); code != 0 {
		t.Fatal(&diagnostics)
	}
	configPath := filepath.Join(root, "fQL/fql.config")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	config = bytes.ReplaceAll(config, []byte("../src/data/fql"), []byte("../custom/client"))
	config = bytes.ReplaceAll(config, []byte("../firebase/generated"), []byte("../custom/firebase"))
	if err := os.WriteFile(configPath, config, 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := cli.Run([]string{"generate", "--project", root}, &out, &diagnostics, "test"); code != 0 {
		t.Fatal(&diagnostics)
	}
	for _, path := range []string{"custom/client/fql.ts", "custom/firebase/firestore.rules", "custom/firebase/firestore.indexes.json", "fQL/fql.lock"} {
		if !strings.Contains(out.String(), "\n"+path+"\n") {
			t.Fatalf("missing output path %s: %s", path, &out)
		}
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatal(err)
		}
	}
	result, code := report(t, "generate", "--check", "--json", "--project", root)
	if code != 0 {
		t.Fatalf("freshness: %v", result)
	}
	timings := result["timings_ms"].(map[string]any)
	for _, phase := range []string{"load", "validate", "load_validate", "generate"} {
		if value, ok := timings[phase].(float64); !ok || value < 0 {
			t.Fatalf("invalid %s timing: %v", phase, timings)
		}
	}
	if timings["load"].(float64)+timings["validate"].(float64) > timings["load_validate"].(float64) {
		t.Fatalf("phase timings exceed measured loading: %v", timings)
	}
}
