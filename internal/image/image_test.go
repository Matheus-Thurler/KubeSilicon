package image

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func qemuImg(t *testing.T, args ...string) {
	t.Helper()
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not installed")
	}
	if out, err := exec.Command("qemu-img", args...).CombinedOutput(); err != nil {
		t.Fatalf("qemu-img %v: %v: %s", args, err, out)
	}
}

func TestDiskRejectsFilesOfTheHost(t *testing.T) {
	tmp := t.TempDir()
	images, cache := filepath.Join(tmp, "images"), filepath.Join(tmp, "cache")
	os.Mkdir(images, 0o755)
	os.Mkdir(cache, 0o755)
	secret := filepath.Join(tmp, "secret")
	os.WriteFile(secret, []byte(strings.Repeat("TOPSECRET", 1000)), 0o600)
	dest := filepath.Join(tmp, "disk.raw")

	good := filepath.Join(images, "good.qcow2")
	qemuImg(t, "create", "-q", "-f", "qcow2", good, "1M")
	backed := filepath.Join(images, "backed.qcow2")
	qemuImg(t, "create", "-q", "-f", "qcow2", "-b", secret, "-F", "raw", backed)
	os.Symlink(secret, filepath.Join(images, "link"))

	if err := Disk(good, "", cache, images, dest, 2<<20); err != nil {
		t.Fatalf("self-contained qcow2 under the image dir: %v", err)
	}
	if st, _ := os.Stat(dest); st == nil || st.Size() != 2<<20 {
		t.Fatalf("disk not converted and resized: %v", st)
	}
	for name, src := range map[string]string{
		"path outside the image dir": secret,
		"symlink out of the dir":     filepath.Join(images, "link"),
		"dot-dot out of the dir":     filepath.Join(images, "..", "secret"),
		"qcow2 with a backing file":  backed,
	} {
		os.Remove(dest)
		if err := Disk(src, "", cache, images, dest, 0); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if b, _ := os.ReadFile(dest); strings.Contains(string(b), "TOPSECRET") {
			t.Errorf("%s: host file leaked into the disk", name)
		}
	}

	// The same backing-file image, served over HTTP.
	srv := httptest.NewServer(http.FileServer(http.Dir(images)))
	defer srv.Close()
	if err := Disk(srv.URL+"/backed.qcow2", "", cache, images, dest, 0); err == nil {
		t.Error("downloaded qcow2 with a backing file: accepted")
	}
}

func TestFetchChecksum(t *testing.T) {
	body := []byte("image bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write(body) }))
	defer srv.Close()
	cache := t.TempDir()

	if _, err := fetch(srv.URL, strings.Repeat("0", 64), cache); err == nil {
		t.Fatal("wrong sha256 accepted")
	}
	if left, _ := os.ReadDir(cache); len(left) != 0 {
		t.Fatalf("a rejected download stayed in the cache: %v", left)
	}
	sum := strings.ToUpper(fmt.Sprintf("%x", sha256.Sum256(body)))
	p, err := fetch(srv.URL, sum, cache)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != string(body) {
		t.Fatalf("cached %q", b)
	}
}
