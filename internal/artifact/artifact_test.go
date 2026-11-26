package artifact

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	if err := Download(context.Background(), srv.URL+"/x?sig=secret", f); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(f); string(b) != "data" {
		t.Fatalf("download got %q", b)
	}
	if err := Upload(context.Background(), f, srv.URL+"/y"); err != nil || string(got) != "data" {
		t.Fatalf("upload: %v %q", err, got)
	}
}

func TestContextBoundsHungServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	f := filepath.Join(t.TempDir(), "f")
	os.WriteFile(f, []byte("x"), 0o644)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if Download(ctx, srv.URL, f) == nil || Upload(ctx, f, srv.URL) == nil {
		t.Fatal("hung server: no error")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("did not return when the context ended")
	}
}

// Transport errors end up in status.message: they must not carry the signature of a presigned URL.
func TestErrorsHideSignature(t *testing.T) {
	f := filepath.Join(t.TempDir(), "f")
	os.WriteFile(f, []byte("x"), 0o644)
	for _, u := range []string{"http://127.0.0.1:1/x?X-Amz-Signature=secret", "http://127.0.0.1:bad/x?X-Amz-Signature=secret"} {
		for _, err := range []error{Download(context.Background(), u, f), Upload(context.Background(), f, u)} {
			if err == nil || strings.Contains(err.Error(), "secret") {
				t.Errorf("%s: %v", u, err)
			}
		}
	}
}
