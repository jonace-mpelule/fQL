// Package release defines version and artifact contracts shared by release tools.
package release

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const Repository = "jonace-mpelule/fQL"

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z][0-9A-Za-z-]*(\.[0-9A-Za-z-]+)*)?$`)

func ValidateVersion(v string) error {
	if !versionPattern.MatchString(v) {
		return fmt.Errorf("use a semantic version without v prefix, for example 0.1.0-dev.3")
	}
	if i := strings.IndexByte(v, '-'); i >= 0 {
		for _, part := range strings.Split(v[i+1:], ".") {
			numeric := true
			for _, c := range part {
				if c < '0' || c > '9' {
					numeric = false
				}
			}
			if numeric && len(part) > 1 && part[0] == '0' {
				return fmt.Errorf("numeric prerelease identifiers cannot contain leading zeros")
			}
		}
	}
	return nil
}
func Assets(v string) []string {
	return []string{"fql_" + v + "_darwin_arm64.tar.gz", "fql_" + v + "_darwin_amd64.tar.gz", "fql_" + v + "_linux_arm64.tar.gz", "fql_" + v + "_linux_amd64.tar.gz", "install.sh", "SHA256SUMS"}
}
func Verify(dir, v string) error {
	if e := ValidateVersion(v); e != nil {
		return e
	}
	b, e := os.ReadFile(filepath.Join(dir, "SHA256SUMS"))
	if e != nil {
		return e
	}
	expected := map[string]bool{}
	for _, name := range Assets(v) {
		if name != "SHA256SUMS" {
			expected[name] = true
		}
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		parts := strings.Split(line, "  ")
		if len(parts) != 2 || !expected[parts[1]] || seen[parts[1]] {
			return fmt.Errorf("unexpected or duplicate checksum entry")
		}
		name := parts[1]
		seen[name] = true
		data, e := os.ReadFile(filepath.Join(dir, name))
		if e != nil {
			return e
		}
		sum := sha256.Sum256(data)
		if parts[0] != hex.EncodeToString(sum[:]) {
			return fmt.Errorf("checksum mismatch: %s", name)
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("release assets are incomplete")
	}
	return nil
}
