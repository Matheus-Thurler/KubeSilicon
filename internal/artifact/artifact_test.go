package artifact

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPath(t *testing.T) {
	for _, bad := range []string{"", "/etc/passwd", "../x", "a/../../x"} {
		if _, err := Path("/w", bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if p, err := Path("/w", "a/b.txt"); err != nil || p != "/w/a/b.txt" {
		t.Errorf("good path: %v %v", p, err)
	}
}

func TestRoundTrip(t *testing.T) {
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			got, _ = io.ReadAll(r.Body)
			return
		}
		w.Write([]byte("data"))
	}))
	defer srv.Close()
	d := t.TempDir()
	f := filepath.Join(d, "sub", "in.txt")
	if err := Download(srv.URL+"/x?sig=secret", f); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(f); string(b) != "data" {
		t.Fatalf("download got %q", b)
	}
	if err := Upload(f, srv.URL+"/y"); err != nil || string(got) != "data" {
		t.Fatalf("upload: %v %q", err, got)
	}
}
