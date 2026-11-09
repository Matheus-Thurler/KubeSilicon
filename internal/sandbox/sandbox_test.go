package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Real sandbox-exec: write inside works, write outside and read of a secret are denied.
func TestProfile(t *testing.T) {
	home, _ := os.UserHomeDir()
	work := t.TempDir()
	secret := filepath.Join(home, ".kubesilicon-sandbox-test-secret")
	if err := os.WriteFile(secret, []byte("x"), 0o600); err != nil {
		t.Skip(err)
	}
	defer os.Remove(secret)
	pf := filepath.Join(work, "p.sb")
	os.WriteFile(pf, []byte(Profile(home, []string{work}, nil)), 0o600)
	run := func(sh string) error {
		return exec.Command(Argv(pf, []string{"/bin/sh", "-c", sh})[0], Argv(pf, []string{"/bin/sh", "-c", sh})[1:]...).Run()
	}
	if err := run("touch " + filepath.Join(work, "ok")); err != nil {
		t.Fatalf("write inside denied: %v", err)
	}
	if err := run("touch " + filepath.Join(home, ".kubesilicon-sandbox-test-w")); err == nil {
		os.Remove(filepath.Join(home, ".kubesilicon-sandbox-test-w"))
		t.Fatal("write outside allowed")
	}
	if err := run("cat " + secret); err == nil {
		t.Fatal("read of home secret allowed")
	}
	if err := run("ls " + home); err == nil {
		t.Fatal("listing the home directory allowed")
	}
}
