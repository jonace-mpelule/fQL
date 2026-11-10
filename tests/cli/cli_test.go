package cli_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/jonace-mpelule/fQL/internal/cli"
)

func TestCommandContract(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code int
		out  string
		err  string
	}{
		{name: "default help", out: "Usage:"},
		{name: "help", args: []string{"help"}, out: "Usage:"},
		{name: "help flag", args: []string{"--help"}, out: "Usage:"},
		{name: "short help", args: []string{"-h"}, out: "Usage:"},
		{name: "version", args: []string{"version"}, out: "fql test-version\n"},
		{name: "version flag", args: []string{"--version"}, out: "fql test-version\n"},
		{name: "unknown command", args: []string{"unknown"}, code: 2, err: "unknown command"},

		{name: "extra arguments", args: []string{"version", "extra"}, code: 2, err: "expected a single command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := cli.Run(tt.args, &stdout, &stderr, "test-version"); got != tt.code {
				t.Fatalf("exit code = %d, want %d", got, tt.code)
			}
			if tt.out == "" && stdout.Len() != 0 || tt.out != "" && !strings.Contains(stdout.String(), tt.out) {
				t.Errorf("stdout = %q, want %q", stdout.String(), tt.out)
			}
			if tt.err == "" && stderr.Len() != 0 || tt.err != "" && !strings.Contains(stderr.String(), tt.err) {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.err)
			}
		})
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) {
	return 0, errors.New("output unavailable")
}

func TestOutputFailure(t *testing.T) {
	var stderr bytes.Buffer
	if code := cli.Run([]string{"version"}, failedWriter{}, &stderr, "test-version"); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "could not write output") {
		t.Fatalf("missing output diagnostic: %q", stderr.String())
	}
}
