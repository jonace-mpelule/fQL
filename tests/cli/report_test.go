package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jonace-mpelule/fQL/internal/cli"
	"github.com/jonace-mpelule/fQL/internal/project"
)

func report(t *testing.T, args ...string) (map[string]any, int) {
	t.Helper()
	var out, diagnostics bytes.Buffer
	code := cli.Run(args, &out, &diagnostics, "test")
	if diagnostics.Len() != 0 {
		t.Fatalf("JSON leaked diagnostics: %s", &diagnostics)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid report %q: %v", &out, err)
	}
	if result["exit_code"] != float64(code) {
		t.Fatalf("exit mismatch: %v", result)
	}
	if elapsed, ok := result["duration_ms"].(float64); !ok || elapsed < 0 {
		t.Fatalf("invalid duration: %v", result)
	}
	return result, code
}

func TestCommandReports(t *testing.T) {
	for _, target := range []string{"dart", "typescript"} {
		t.Run(target, func(t *testing.T) {
			root := t.TempDir()
			result, code := report(t, "init", "--json", "--target", target, "--project", root)
			if code != 0 || result["stats"].(map[string]any)["files_created"] != float64(3) {
				t.Fatalf("init: %v", result)
			}
			for _, command := range []string{"validate", "format", "generate", "diff", "audit"} {
				result, code = report(t, command, "--json", "--project", root)
				if command == "audit" {
					if code != 1 || result["complete"] != false || result["error"] == nil {
						t.Fatalf("audit failure: %v", result)
					}
					continue
				}
				if code != 0 || result["target"] != target {
					t.Fatalf("%s: %v", command, result)
				}
				stats := result["stats"].(map[string]any)
				for key, want := range map[string]float64{"schema_files": 2, "models": 2, "fields": 10, "indexes": 1, "access_policies": 8} {
					if stats[key] != want {
						t.Fatalf("%s %s = %v, want %v", command, key, stats[key], want)
					}
				}
				if command == "generate" && stats["files_generated"] != float64(4) {
					t.Fatalf("output count: %v", stats)
				}
				if command == "diff" && result["changes"] == nil {
					t.Fatalf("lost existing diff report: %v", result)
				}
			}
			result, code = report(t, "generate", "--json", "--check", "--project", root)
			stats := result["stats"].(map[string]any)
			if code != 0 || result["mode"] != "check" || stats["files_checked"] != float64(4) || stats["files_generated"] != nil {
				t.Fatalf("check: %v", result)
			}
			result, code = report(t, "generate", "--json", "--check", "--watch", "--project", root)
			if code != 2 || result["status"] != "error" {
				t.Fatalf("usage failure: %v", result)
			}
		})
	}
}

func TestReportFailuresAndHumanOutput(t *testing.T) {
	root := t.TempDir()
	if err := project.Init(root); err != nil {
		t.Fatal(err)
	}
	result, code := report(t, "generate", "--json", "--check", "--project", root)
	if code != 1 || result["stats"].(map[string]any)["files_checked"] != nil {
		t.Fatalf("stale output claimed current: %v", result)
	}
	var out, diagnostics bytes.Buffer
	if code := cli.Run([]string{"generate", "--project", root}, &out, &diagnostics, "test"); code != 0 {
		t.Fatalf("generate: %s", &diagnostics)
	}
	for _, want := range []string{"FQL generate · success", "files generated", "models", "Elapsed", " ms"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %s", want, &out)
		}
	}
	if code := cli.Run([]string{"validate", "--project", root}, failedWriter{}, &diagnostics, "test"); code != 1 {
		t.Fatal("output failure not propagated")
	}
	if err := os.WriteFile(filepath.Join(root, "fQL/schemas/users.fql"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	result, code = report(t, "generate", "--json", "--project", root)
	if code != 1 || result["status"] != "error" || result["error"] == nil || result["stats"].(map[string]any)["files_generated"] != nil {
		t.Fatalf("invalid input: %v", result)
	}
}

type eventWriter struct{ write func([]byte) error }

func (w eventWriter) Write(b []byte) (int, error) {
	if err := w.write(b); err != nil {
		return 0, err
	}
	return len(b), nil
}

func TestWatchReportsAndRecovery(t *testing.T) {
	root := t.TempDir()
	if err := project.Init(root); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var events []map[string]any
	var diagnostics bytes.Buffer
	path := filepath.Join(root, "fQL/schemas/users.fql")
	writer := eventWriter{write: func(b []byte) error {
		var event map[string]any
		if err := json.Unmarshal(b, &event); err != nil {
			return err
		}
		events = append(events, event)
		switch len(events) {
		case 1:
			return os.WriteFile(path, []byte("invalid"), 0600)
		case 2:
			return os.WriteFile(path, []byte(project.UserSchema+"\nenum Extra { yes no }\n"), 0600)
		case 3:
			cancel()
		default:
			return fmt.Errorf("unexpected event")
		}
		return nil
	}}
	code := cli.RunContext(ctx, []string{"generate", "--watch", "--json", "--project", root}, writer, &diagnostics, "test")
	if code != 0 || len(events) != 3 || diagnostics.Len() != 0 {
		t.Fatalf("watch code=%d events=%v stderr=%s", code, events, &diagnostics)
	}
	for i, want := range []string{"success", "error", "success"} {
		if events[i]["status"] != want || events[i]["mode"] != "watch" {
			t.Fatalf("event %d: %v", i, events[i])
		}
	}
	if events[2]["stats"].(map[string]any)["enums"] != float64(1) {
		t.Fatalf("stale counts: %v", events[2])
	}
}
