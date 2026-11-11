// Command publish verifies a committed release, stages assets in a draft, and publishes it.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jonace-mpelule/fQL/internal/release"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "publish:", e)
		os.Exit(1)
	}
}
func output(name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	var stderr strings.Builder
	c.Stderr = &stderr
	b, e := c.Output()
	if e != nil {
		return "", fmt.Errorf("%s failed: %s", name, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(b)), nil
}
func command(name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

type asset struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}
type githubRelease struct {
	Draft  bool    `json:"draft"`
	Target string  `json:"target_commitish"`
	URL    string  `json:"html_url"`
	Assets []asset `json:"assets"`
}

func lookup(tag string) (githubRelease, bool, error) {
	var r githubRelease
	b, e := output("gh", "api", "repos/"+release.Repository+"/releases/tags/"+tag)
	if e != nil {
		if strings.Contains(e.Error(), "HTTP 404") {
			// The tag endpoint can omit unpublished drafts whose Git tag does not exist yet.
			query := fmt.Sprintf(".[] | select(.tag_name == %q)", tag)
			b, err := output("gh", "api", "--paginate", "repos/"+release.Repository+"/releases?per_page=100", "--jq", query)
			if err != nil {
				return r, false, err
			}
			if b == "" {
				return r, false, nil
			}
			err = json.Unmarshal([]byte(b), &r)
			return r, true, err
		}
		return r, false, e
	}
	e = json.Unmarshal([]byte(b), &r)
	return r, true, e
}
func run() error {
	version := flag.String("version", "", "release version without v prefix")
	flag.Parse()
	if e := release.ValidateVersion(*version); e != nil {
		return e
	}
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if _, e := os.Stat("go.mod"); e != nil {
		return fmt.Errorf("run from the repository root")
	}
	status, e := output("git", "status", "--porcelain")
	if e != nil {
		return e
	}
	if status != "" {
		return fmt.Errorf("commit all changes before releasing; the worktree must be clean")
	}
	sha, e := output("git", "rev-parse", "HEAD")
	if e != nil {
		return e
	}
	origin, e := output("git", "remote", "get-url", "origin")
	if e != nil {
		return e
	}
	if origin != "https://github.com/"+release.Repository+".git" && origin != "https://github.com/"+release.Repository && origin != "git@github.com:"+release.Repository+".git" {
		return fmt.Errorf("origin must be the official FQL repository")
	}
	if _, e = output("gh", "api", "repos/"+release.Repository+"/commits/"+sha, "--jq", ".sha"); e != nil {
		return fmt.Errorf("push this commit before releasing: %w", e)
	}
	tag := "v" + *version
	r, exists, e := lookup(tag)
	if e != nil {
		return e
	}
	if exists && (!r.Draft || r.Target != sha) {
		return fmt.Errorf("release exists and is published or targets another commit; choose a new version")
	}
	if _, err := output("gh", "api", "repos/"+release.Repository+"/git/ref/tags/"+tag); err == nil {
		target, e := output("gh", "api", "repos/"+release.Repository+"/commits/"+tag, "--jq", ".sha")
		if e != nil {
			return e
		}
		if target != sha {
			return fmt.Errorf("existing tag points to a different commit")
		}
	} else if !strings.Contains(err.Error(), "HTTP 404") {
		return err
	}
	allowed := map[string]bool{}
	for _, name := range release.Assets(*version) {
		allowed[name] = true
	}
	for _, a := range r.Assets {
		if !allowed[a.Name] {
			return fmt.Errorf("draft contains unexpected asset %s; inspect it before resuming", a.Name)
		}
	}
	if e = command("make", "release-check"); e != nil {
		return e
	}
	// A version-specific directory prevents accidentally uploading old archives.
	dir := filepath.Join("dist", *version)
	if e = command("go", "run", "./tools/release", "--version", *version, "--output", dir); e != nil {
		return e
	}
	if e = command("python3", "tests/release/verify_archives.py", "--version", *version, "--directory", dir, "--binary-only"); e != nil {
		return e
	}
	if e = release.Verify(dir, *version); e != nil {
		return e
	}
	current, e := output("git", "rev-parse", "HEAD")
	if e != nil || current != sha {
		return fmt.Errorf("commit changed while preparing release")
	}
	status, e = output("git", "status", "--porcelain")
	if e != nil || status != "" {
		return fmt.Errorf("worktree changed while preparing release")
	}
	prerelease := strings.Contains(*version, "-")
	if !exists {
		notes := fmt.Sprintf("FQL %s\n\nGo compiler and CLI with generated Dart clients, schema validation, Security Rules/indexes, enums, embedded data, managed timestamps, atomic writes, formatting, diffs, and read-only audits.\n\nInstall on macOS/Linux:\n```sh\ncurl -fsSL https://raw.githubusercontent.com/%s/main/install.sh | sh -s -- --version %s\n```\n\nArchives and installers are covered by SHA256SUMS. No Go or Node.js is required to use the CLI.\n\nDevelopment release: native Android/iOS acceptance and production deployments remain separate verification boundaries.\n\nSource commit: %s\n", tag, release.Repository, *version, sha)
		f, e := os.CreateTemp("", "fql-release-notes-*.md")
		if e != nil {
			return e
		}
		defer os.Remove(f.Name())
		if _, e = f.WriteString(notes); e != nil {
			f.Close()
			return e
		}
		if e = f.Close(); e != nil {
			return e
		}
		args := []string{"release", "create", tag, "--repo", release.Repository, "--draft", "--target", sha, "--title", "FQL " + tag, "--notes-file", f.Name()}
		if prerelease {
			args = append(args, "--prerelease")
		}
		if e = command("gh", args...); e != nil {
			return e
		}
	}
	args := []string{"release", "upload", tag, "--repo", release.Repository, "--clobber"}
	for _, name := range release.Assets(*version) {
		args = append(args, filepath.Join(dir, name))
	}
	if e = command("gh", args...); e != nil {
		return fmt.Errorf("upload failed; rerun this version from the same commit to resume its draft: %w", e)
	}
	r, exists, e = lookup(tag)
	if e != nil {
		return e
	}
	if !exists || !r.Draft || r.Target != sha || len(r.Assets) != len(allowed) {
		return fmt.Errorf("draft verification failed; nothing was published")
	}
	verified := map[string]bool{}
	for _, a := range r.Assets {
		if !allowed[a.Name] || verified[a.Name] {
			return fmt.Errorf("unexpected uploaded asset")
		}
		verified[a.Name] = true
		b, e := os.ReadFile(filepath.Join(dir, a.Name))
		if e != nil {
			return e
		}
		sum := sha256.Sum256(b)
		if a.Size != int64(len(b)) {
			return fmt.Errorf("uploaded size mismatch: %s", a.Name)
		}
		if a.Digest != "" && a.Digest != "sha256:"+hex.EncodeToString(sum[:]) {
			return fmt.Errorf("uploaded digest mismatch: %s", a.Name)
		}
	}
	// Verify downloaded draft bytes too, including installations where GitHub omits digests.
	downloaded, e := os.MkdirTemp("", "fql-release-download-*")
	if e != nil {
		return e
	}
	defer os.RemoveAll(downloaded)
	if e = command("gh", "release", "download", tag, "--repo", release.Repository, "--dir", downloaded); e != nil {
		return e
	}
	for _, name := range release.Assets(*version) {
		local, e := os.ReadFile(filepath.Join(dir, name))
		if e != nil {
			return e
		}
		remote, e := os.ReadFile(filepath.Join(downloaded, name))
		if e != nil {
			return e
		}
		if sha256.Sum256(local) != sha256.Sum256(remote) {
			return fmt.Errorf("downloaded asset mismatch: %s", name)
		}
	}
	if e = command("gh", "release", "edit", tag, "--repo", release.Repository, "--draft=false", fmt.Sprintf("--prerelease=%t", prerelease), fmt.Sprintf("--latest=%t", !prerelease)); e != nil {
		return e
	}
	fmt.Printf("Published https://github.com/%s/releases/tag/%s\n", release.Repository, tag)
	return nil
}
