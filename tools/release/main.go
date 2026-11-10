// Command release creates reproducible, checksummed CLI archives without publishing.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"flag"
	"fmt"
	"github.com/jonace-mpelule/fQL/internal/release"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "release:", e)
		os.Exit(1)
	}
}
func run() error {
	version := flag.String("version", "", "release version without v prefix")
	out := flag.String("output", "dist", "archive output directory")
	flag.Parse()
	if release.ValidateVersion(*version) != nil || flag.NArg() != 0 {
		return fmt.Errorf("supply --version, for example 0.1.0-dev.3")
	}
	if _, e := os.Stat("go.mod"); e != nil {
		return fmt.Errorf("run from the repository root")
	}
	temp, e := os.MkdirTemp("", "fql-release-*")
	if e != nil {
		return e
	}
	defer os.RemoveAll(temp)
	sums := map[string]string{}
	for _, target := range []struct{ os, arch string }{{"darwin", "arm64"}, {"darwin", "amd64"}, {"linux", "arm64"}, {"linux", "amd64"}, {"windows", "amd64"}} {
		name := "fql"
		if target.os == "windows" {
			name += ".exe"
		}
		binary := filepath.Join(temp, name)
		cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags", "-s -w -X main.version="+*version, "-o", binary, "./cmd/fql")
		env := []string{}
		for _, v := range os.Environ() {
			if !strings.HasPrefix(v, "GOOS=") && !strings.HasPrefix(v, "GOARCH=") && !strings.HasPrefix(v, "CGO_ENABLED=") {
				env = append(env, v)
			}
		}
		cmd.Env = append(env, "GOOS="+target.os, "GOARCH="+target.arch, "CGO_ENABLED=0")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if e = cmd.Run(); e != nil {
			return e
		}
		if target.os == runtime.GOOS && target.arch == runtime.GOARCH {
			output, e := exec.Command(binary, "version").Output()
			if e != nil || string(output) != "fql "+*version+"\n" {
				return fmt.Errorf("built CLI failed version smoke check")
			}
		}
		data, e := os.ReadFile(binary)
		if e != nil {
			return e
		}
		var buf bytes.Buffer
		extension := ".tar.gz"
		if target.os == "windows" {
			extension = ".zip"
			w := zip.NewWriter(&buf)
			h := &zip.FileHeader{Name: name, Method: zip.Deflate}
			h.SetMode(0755)
			h.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
			entry, e := w.CreateHeader(h)
			if e != nil {
				return e
			}
			if _, e = entry.Write(data); e != nil {
				return e
			}
			if e = w.Close(); e != nil {
				return e
			}
		} else {
			gz := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gz)
			if e = tw.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(data)), ModTime: time.Unix(0, 0)}); e != nil {
				return e
			}
			if _, e = io.Copy(tw, bytes.NewReader(data)); e != nil {
				return e
			}
			if e = tw.Close(); e != nil {
				return e
			}
			if e = gz.Close(); e != nil {
				return e
			}
		}
		archive := "fql_" + *version + "_" + target.os + "_" + target.arch + extension
		b := buf.Bytes()
		hash := sha256.Sum256(b)
		sums[archive] = fmt.Sprintf("%x", hash)
		if e = os.WriteFile(filepath.Join(temp, archive), b, 0644); e != nil {
			return e
		}
	}
	for _, name := range []string{"install.sh", "install.ps1"} {
		b, e := os.ReadFile(name)
		if e != nil {
			return e
		}
		hash := sha256.Sum256(b)
		sums[name] = fmt.Sprintf("%x", hash)
		if e = os.WriteFile(filepath.Join(temp, name), b, 0644); e != nil {
			return e
		}
	}
	var names []string
	for name := range sums {
		names = append(names, name)
	}
	sort.Strings(names)
	var checksums strings.Builder
	for _, name := range names {
		fmt.Fprintf(&checksums, "%s  %s\n", sums[name], name)
	}
	if e = os.WriteFile(filepath.Join(temp, "SHA256SUMS"), []byte(checksums.String()), 0644); e != nil {
		return e
	}
	if e = os.MkdirAll(*out, 0755); e != nil {
		return e
	}
	for _, name := range append(names, "SHA256SUMS") {
		from, e := os.Open(filepath.Join(temp, name))
		if e != nil {
			return e
		}
		dest, e := os.Create(filepath.Join(*out, name))
		if e != nil {
			from.Close()
			return e
		}
		_, copyErr := io.Copy(dest, from)
		from.Close()
		closeErr := dest.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	fmt.Printf("Created %d CLI archives and SHA256SUMS in %s\n", len(names)-2, *out)
	return nil
}
