package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
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
	if _, err := stdout.Write(out); err != nil {
		return err
	}
	if _, err := stderr.Write(diagnostics); err != nil {
		return err
	}
	if r.quiet {
		return nil
	}
	var summary strings.Builder
	fmt.Fprintf(&summary, "\nFQL %s · %s\n", r.Command, status)
	if r.Target != "" {
		fmt.Fprintf(&summary, "  %-20s %s\n", "Target", r.Target)
	}
	if r.Mode != "" {
		fmt.Fprintf(&summary, "  %-20s %s\n", "Mode", r.Mode)
	}
	keys := make([]string, 0, len(r.Stats))
	for key := range r.Stats {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&summary, "  %-20s %d\n", strings.ReplaceAll(key, "_", " "), r.Stats[key])
	}
	keys = keys[:0]
	for key := range r.Timings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&summary, "  %-20s %.3f ms\n", strings.ReplaceAll(key, "_", " "), r.Timings[key])
	}
	fmt.Fprintf(&summary, "  %-20s %.3f ms\n", "Elapsed", elapsed)
	writer := stdout
	if code != 0 {
		writer = stderr
	}
	_, err := io.WriteString(writer, summary.String())
	return err
}
