package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJobSpecValidate(t *testing.T) {
	ok := jobSpec{Python: "3.12", Deps: []string{"torch", "numpy==2.1.0", "mlx>=0.30"}}
	if err := ok.validate(); err != nil {
		t.Fatal(err)
	}
	for _, s := range []jobSpec{
		{Python: "/tmp/evil"},
		{Python: "python3.12"},
		{Python: ""},
		{Python: "3.12", Deps: []string{"--target=/somewhere"}},
		{Python: "3.12", Deps: []string{"torch", "-r/etc/passwd"}},
		{Python: "3.12", Deps: []string{"./local"}},
		{Python: "3.12", Deps: []string{" --index-url=http://x"}},
		{Python: "3.12", Deps: []string{""}},
	} {
		if s.validate() == nil {
			t.Errorf("accepted %+v", s)
		}
	}
}

// A python path would be executed by uv outside the sandbox: the runner must stop before that.
func TestExecJobDoesNotRunAPythonPath(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	evil := filepath.Join(dir, "evil")
	os.WriteFile(evil, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755)
	os.WriteFile(filepath.Join(dir, "spec.json"), []byte(`{"python":"`+evil+`","script":"print(1)","timeoutSeconds":30}`), 0o600)
	if code, msg := execJob(dir); code == 0 {
		t.Fatalf("job accepted: %s", msg)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the python path was executed")
	}
}
