// Package cli implements the developer-facing FQL command entry point.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/jonace-mpelule/fQL/internal/audit"
	"github.com/jonace-mpelule/fQL/internal/evolve"
	formatter "github.com/jonace-mpelule/fQL/internal/format"
	"github.com/jonace-mpelule/fQL/internal/generate"
	"github.com/jonace-mpelule/fQL/internal/project"
)

const help = `FQL — Firestore Query Language

Usage:
  fql init --target dart|typescript [--project PATH]
  fql validate [--project PATH]
  fql generate [--project PATH] [--check | --watch]
  fql format [--project PATH] [--check]
  fql diff [--project PATH] [--against PROJECT] [--check] [--json]
  fql audit --firebase-project ID [--model NAME] [--emulator HOST:PORT] [--max-documents N] [--json]
  fql help
  fql version

Commands:
  init       Create fQL/schemas and project configuration without overwriting files
  validate   Parse and validate the complete project schema without writing files
  generate   Generate a typed client, Security Rules, indexes, and compatibility metadata
  format     Validate the whole schema, then format files while preserving comments
  diff       Compare against the last generated baseline or --against PROJECT
  audit      Read-only, bounded scan; production token comes from FQL_ACCESS_TOKEN
  help       Show usage
  version    Show the compiler version

The default project is the current directory. Generated files are committed.
--check never writes files. --watch regenerates and recovers after source errors.
`

// Run executes a command without terminating the calling process.
func Run(args []string, stdout, stderr io.Writer, version string) int {
	return RunContext(context.Background(), args, stdout, stderr, version)
}

// RunContext supports cancellation of a watch session. Codes: 0 success, 1 failure, 2 usage.
func RunContext(ctx context.Context, args []string, stdout, stderr io.Writer, version string) int {
	if len(args) == 0 {
		return output(stdout, stderr, help)
	}
	switch args[0] {
	case "help", "--help", "-h", "version", "--version":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "fql: expected a single command; run 'fql help'")
			return 2
		}
		if args[0] == "version" || args[0] == "--version" {
			return output(stdout, stderr, "fql "+version+"\n")
		}
		return output(stdout, stderr, help)
	}
	if args[0] != "init" && args[0] != "validate" && args[0] != "generate" && args[0] != "format" && args[0] != "diff" && args[0] != "audit" {
		fmt.Fprintf(stderr, "fql: unknown command %q; run 'fql help'\n", args[0])
		return 2
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("project", ".", "project root containing fQL/fql.config")
	var target *string
	var check, watch, asJSON bool
	var against string
	var auditOptions audit.Options
	if args[0] == "format" || args[0] == "diff" {
		flags.BoolVar(&check, "check", false, "report changes without writing; exit nonzero if changes exist")
	}
	if args[0] == "diff" {
		flags.StringVar(&against, "against", "", "baseline project directory")
	}
	if args[0] == "diff" || args[0] == "audit" {
		flags.BoolVar(&asJSON, "json", false, "structured output without stored document values")
	}
	if args[0] == "audit" {
		flags.StringVar(&auditOptions.ProjectID, "firebase-project", "", "explicit Firebase project ID")
		flags.StringVar(&auditOptions.Database, "database", "(default)", "Firestore database ID")
		flags.StringVar(&auditOptions.Model, "model", "", "one model (default: all)")
		flags.StringVar(&auditOptions.Emulator, "emulator", "", "loopback emulator host:port; privileged emulator reads")
		flags.IntVar(&auditOptions.MaxDocuments, "max-documents", 1000, "maximum total documents to read")
	}
	if args[0] == "init" {
		target = flags.String("target", "dart", "runtime target (dart or typescript)")
	}
	if args[0] == "generate" {
		flags.BoolVar(&check, "check", false, "fail if generated artifacts are stale; do not write")
		flags.BoolVar(&watch, "watch", false, "regenerate after source changes until interrupted")
	}
	if e := flags.Parse(args[1:]); e != nil {
		if e == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || check && watch {
		fmt.Fprintln(stderr, "fql: unexpected arguments or incompatible --check/--watch flags")
		return 2
	}
	if target != nil && *target != "dart" && *target != "typescript" {
		fmt.Fprintln(stderr, "fql: supported targets: dart, typescript")
		return 2
	}
	if args[0] == "init" {
		if e := project.InitTarget(*root, *target); e != nil {
			fmt.Fprintln(stderr, "fql:", e)
			return 1
		}
		return output(stdout, stderr, "Initialized fQL/schemas and fQL/fql.config. Run fql validate, then fql generate.\n")
	}
	if args[0] == "format" {
		if e := formatter.Project(*root, check); e != nil {
			fmt.Fprintln(stderr, "fql:", e)
			return 1
		}
		return output(stdout, stderr, "Schemas are valid and formatted.\n")
	}
	if watch {
		return watchProject(ctx, *root, stdout, stderr)
	}
	p, e := project.Load(*root)
	if e == nil && args[0] == "generate" {
		e = generate.Write(p, check)
	}
	if e != nil {
		fmt.Fprintln(stderr, "fql:", e)
		return 1
	}
	if args[0] == "diff" {
		baseline, err := evolve.Baseline(filepath.Join(p.Root, "fQL/fql.lock"))
		if against != "" {
			other, loadErr := project.Load(against)
			err = loadErr
			baseline = other.Schema
		}
		if err != nil {
			fmt.Fprintln(stderr, "fql:", err)
			return 1
		}
		report := evolve.Compare(baseline, p.Schema)
		if asJSON {
			if err = json.NewEncoder(stdout).Encode(report); err != nil {
				return 1
			}
		} else {
			if len(report.Changes) == 0 {
				fmt.Fprintln(stdout, "No schema changes.")
			}
			for _, c := range report.Changes {
				fmt.Fprintf(stdout, "%s %s (%s): %s\n", c.Path, c.Kind, c.Risk, c.Message)
			}
		}
		if check && len(report.Changes) > 0 {
			return 3
		}
		return 0
	}
	if args[0] == "audit" {
		auditOptions.Token = os.Getenv("FQL_ACCESS_TOKEN")
		report, err := audit.Run(ctx, p.Schema, auditOptions)
		if err != nil {
			report.Complete = false
		}
		if asJSON {
			if e = json.NewEncoder(stdout).Encode(report); e != nil {
				return 1
			}
		} else {
			fmt.Fprintf(stdout, "Audit (%s): scanned %d documents; %d invalid; complete=%t\n", report.Environment, report.Scanned, len(report.Violations), report.Complete)
			for _, v := range report.Violations {
				for _, issue := range v.Issues {
					fmt.Fprintf(stdout, "%q %q: %s\n", v.Document, issue.Field, issue.Message)
				}
			}
		}
		if err != nil {
			fmt.Fprintln(stderr, "fql:", err)
			return 1
		}
		if !report.Complete {
			return 4
		}
		if len(report.Violations) > 0 {
			return 3
		}
		return 0
	}
	message := "Schema is valid.\n"
	if args[0] == "generate" {
		message = fmt.Sprintf("Generated %s client, rules, indexes, and fQL/fql.lock.\n", p.Target)
		if check {
			message = "Generated artifacts are current.\n"
		}
	}
	return output(stdout, stderr, message)
}
func watchProject(ctx context.Context, root string, stdout, stderr io.Writer) int {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	lastFingerprint, lastError := "", ""
	for {
		if ctx.Err() != nil {
			return 0
		}
		p, e := project.Load(root)
		if e == nil && (p.Fingerprint != lastFingerprint || lastError != "") {
			e = generate.Write(p, false)
			if e == nil {
				lastFingerprint = p.Fingerprint
				lastError = ""
				if output(stdout, stderr, "Generated artifacts; watching for schema changes.\n") != 0 {
					return 1
				}
			}
		}
		if e != nil && e.Error() != lastError {
			fmt.Fprintln(stderr, "fql:", e)
			lastError = e.Error()
		}
		select {
		case <-ctx.Done():
			return 0
		case <-ticker.C:
		}
	}
}
func output(stdout, stderr io.Writer, message string) int {
	if _, e := io.WriteString(stdout, message); e != nil {
		fmt.Fprintln(stderr, "fql: could not write output")
		return 1
	}
	return 0
}
