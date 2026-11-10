package release_test

import (
	"crypto/sha256"
	"fmt"
	"github.com/jonace-mpelule/fQL/internal/release"
	"os"
	"path/filepath"
	"testing"
)

func TestVersions(t *testing.T) {
	for _, v := range []string{"0.1.0-dev.3", "0.1.0", "1.2.3-rc.1"} {
		if e := release.ValidateVersion(v); e != nil {
			t.Fatal(e)
		}
	}
	for _, v := range []string{"dev", "v0.1.0", "0.01.0", "0.1.0-01", "0.1.0-", "0.1.0\n", "../../unsafe"} {
		if release.ValidateVersion(v) == nil {
			t.Fatalf("invalid version accepted: %q", v)
		}
	}
}
func TestManifestIntegrity(t *testing.T) {
	dir := t.TempDir()
	version := "0.1.0-dev.3"
	manifest := ""
	for _, name := range release.Assets(version) {
		if name == "SHA256SUMS" {
			continue
		}
		b := []byte(name)
		os.WriteFile(filepath.Join(dir, name), b, 0644)
		sum := sha256.Sum256(b)
		manifest += fmt.Sprintf("%x  %s\n", sum, name)
	}
	os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(manifest), 0644)
	if e := release.Verify(dir, version); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(dir, "install.sh"), []byte("tampered"), 0644)
	if release.Verify(dir, version) == nil {
		t.Fatal("tampering accepted")
	}
}
