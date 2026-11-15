package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/jonace-mpelule/fQL/internal/project"
)

// commandReport contains presentation-only measurements; none enter artifacts.
type commandReport struct {
	Command        string
	Target         string
	Mode           string
	JSON           bool
	Stats          map[string]int
	Timings        map[string]float64
	Files          []string
	started        time.Time
	streaming      bool
	quiet          bool
	stdout, stderr io.Writer
}

func workingCommand(command string) bool {
	switch command {
	case "init", "validate", "generate", "format", "diff", "audit":
		return true
	}
	return false
}

func newReport(command string) *commandReport {
	return &commandReport{Command: command, Stats: map[string]int{}, Timings: map[string]float64{}, started: time.Now()}
}

func milliseconds(d time.Duration) float64 { return float64(d.Nanoseconds()) / 1e6 }

func (r *commandReport) project(p project.Project) {
	r.Target = p.Target
	r.Timings["load"] = milliseconds(p.LoadDuration)
	r.Timings["validate"] = milliseconds(p.ValidateDuration)
	client := "fql.dart"
	if p.Target == "typescript" {
		client = "fql.ts"
	}
	for _, path := range []string{filepath.Join(p.Output, client), filepath.Join(p.Firebase, "firestore.rules"), filepath.Join(p.Firebase, "firestore.indexes.json"), filepath.Join(p.Root, "fQL", "fql.lock")} {
		rel, _ := filepath.Rel(p.Root, path)
		r.Files = append(r.Files, filepath.ToSlash(rel))
	}
	r.Stats["schema_files"] = p.SourceFiles
	r.Stats["models"] = len(p.Schema.Models)
	r.Stats["enums"] = len(p.Schema.Enums)
	r.Stats["objects"] = len(p.Schema.Objects)
	r.Stats["named_policies"] = len(p.Schema.NamedPolicies)
	r.Stats["claims"] = len(p.Schema.Claims)
	fields, indexes, policies := 0, 0, 0
	for _, m := range p.Schema.Models {
		fields += len(m.Fields)
		indexes += len(m.Indexes)
		policies += len(m.Policies)
	}
	for _, o := range p.Schema.Objects {
		fields += len(o.Fields)
	}
	r.Stats["fields"], r.Stats["indexes"], r.Stats["access_policies"] = fields, indexes, policies
}

func (r *commandReport) finish(stdout, stderr io.Writer, out, diagnostics []byte, code int) int {
	if err := r.emit(stdout, stderr, out, diagnostics, code); err != nil {
		fmt.Fprintln(stderr, "fql: could not write output")
		return 1
	}
	return code
}

func (r *commandReport) emit(stdout, stderr io.Writer, out, diagnostics []byte, code int) error {
	elapsed := milliseconds(time.Since(r.started))
	status := "success"
	switch code {
	case 0:
	case 3:
		status = "changes"
		if r.Command == "audit" {
			status = "invalid"
		}
	case 4:
		status = "incomplete"
	default:
		status = "error"
	}
	if r.JSON && !r.quiet {
		// Preserve existing top-level diff/audit report fields for consumers.
		payload := map[string]any{}
		if len(out) > 0 {
			if err := json.Unmarshal(out, &payload); err != nil || payload == nil {
				payload = map[string]any{"message": strings.TrimSpace(string(out))}
			}
		}
		payload["command"], payload["status"], payload["exit_code"] = r.Command, status, code
		payload["duration_ms"], payload["stats"] = elapsed, r.Stats
		if r.Target != "" {
			payload["target"] = r.Target
		}
		if r.Mode != "" {
			payload["mode"] = r.Mode
		}
		if len(r.Timings) > 0 {
			payload["timings_ms"] = r.Timings
		}
		if len(diagnostics) > 0 {
			payload["error"] = strings.TrimSpace(string(diagnostics))
		}
		return json.NewEncoder(stdout).Encode(payload)
	}
	if r.quiet {
		if _, err := stdout.Write(out); err != nil {
			return err
		}
		_, err := stderr.Write(diagnostics)
		return err
	}
	text := r.human(out, diagnostics, code, elapsed)
	writer := stdout
	if code == 1 || code == 2 {
		writer = stderr
	}
	_, err := io.WriteString(writer, text)
	return err
}

func (r *commandReport) targetName() string {
	if r.Target == "typescript" {
		return "TypeScript"
	}
	if r.Target == "dart" {
		return "Dart"
	}
	return r.Target
}

func (r *commandReport) human(out, diagnostics []byte, code int, elapsed float64) string {
	var b strings.Builder
	if r.Mode == "watch" {
		if code == 0 {
			fmt.Fprintf(&b, "%s  ✓ Generated %d files · %.1f ms\n", time.Now().Format("15:04:05"), r.Stats["files_generated"], elapsed)
			return b.String()
		}
		fmt.Fprintf(&b, "%s  ✗ Generation failed\n", time.Now().Format("15:04:05"))
	} else if code == 1 || code == 2 {
		name := map[string]string{"init": "Initialization", "generate": "Generation", "validate": "Validation", "format": "Formatting", "diff": "Schema comparison", "audit": "Audit"}[r.Command]
		fmt.Fprintf(&b, "✗ %s failed\n", name)
	}
	if code == 1 || code == 2 {
		if len(diagnostics) > 0 {
			b.WriteString("\n")
			b.WriteString(strings.TrimPrefix(strings.TrimSpace(string(diagnostics)), "fql: "))
			b.WriteString("\n")
		}
		if r.Command == "audit" {
			fmt.Fprintf(&b, "\n%d documents scanned · %d invalid\n", r.Stats["documents_scanned"], r.Stats["invalid_documents"])
			if len(out) > 0 {
				fmt.Fprintf(&b, "%s", out)
			}
		}
		fmt.Fprintf(&b, "\nStopped after %.1f ms\n", elapsed)
		if r.Mode == "watch" {
			b.WriteString("Watching for changes…\n")
		}
		return b.String()
	}
	switch r.Command {
	case "init":
		for _, file := range r.Files {
			fmt.Fprintf(&b, "✓ Created %s\n", file)
		}
		fmt.Fprintf(&b, "\nInitialized %s project in %.1f ms\nNext: fql generate\n", r.targetName(), elapsed)
	case "validate":
		fmt.Fprintf(&b, "✓ Schema valid · %d models · %d fields · %.1f ms\n", r.Stats["models"], r.Stats["fields"], elapsed)
	case "generate":
		if r.Mode == "check" {
			fmt.Fprintf(&b, "✓ Generated files are up to date · %.1f ms\n", elapsed)
			break
		}
		fmt.Fprintf(&b, "FQL › generate · %s\n\n", r.targetName())
		stage := func(label string, duration float64) { fmt.Fprintf(&b, "%-56s %7.1f ms\n", "✓ "+label, duration) }
		stage(fmt.Sprintf("Loaded %d schema files", r.Stats["schema_files"]), r.Timings["load"])
		stage(fmt.Sprintf("Validated %d models, %d fields, %d policies", r.Stats["models"], r.Stats["fields"], r.Stats["access_policies"]), r.Timings["validate"])
		stage(fmt.Sprintf("Generated %d files", r.Stats["files_generated"]), r.Timings["generate"])
		b.WriteString("\n")
		for _, file := range r.Files {
			fmt.Fprintln(&b, file)
		}
		fmt.Fprintf(&b, "\nDone in %.1f ms\n", elapsed)
	case "format":
		label := "Formatted"
		if r.Mode == "check" {
			label = "Formatting valid for"
		}
		fmt.Fprintf(&b, "✓ %s %d schema files · %.1f ms\n", label, r.Stats["files_processed"], elapsed)
	case "diff":
		if r.Stats["changes"] == 0 {
			fmt.Fprintf(&b, "✓ No schema changes · %.1f ms\n", elapsed)
		} else {
			fmt.Fprintf(&b, "Schema changes: %d\n\n%s\nCompared in %.1f ms\n", r.Stats["changes"], out, elapsed)
		}
	case "audit":
		mark, label := "✓", "Audit complete"
		if code == 3 {
			mark = "✗"
		}
		if code == 4 {
			mark, label = "!", "Audit incomplete"
		}
		fmt.Fprintf(&b, "%s %s · %d documents scanned · %d invalid · %.1f ms\n", mark, label, r.Stats["documents_scanned"], r.Stats["invalid_documents"], elapsed)
		if len(out) > 0 {
			fmt.Fprintf(&b, "\n%s", out)
		}
	}
	return b.String()
}
