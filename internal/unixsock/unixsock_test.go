package unixsock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLongPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), strings.Repeat("n", 120))
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(dir, "console.sock")
	before, _ := os.Getwd()
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if _, err := os.Stat(sock); err != nil {
		t.Fatal(err)
	}
	c, err := Dial(sock)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if after, _ := os.Getwd(); after != before {
		t.Fatalf("cwd changed: %s", after)
	}
}
