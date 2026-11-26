package sandbox

import (
	"fmt"
	"net"
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

// Ways out of the sandbox that go through another process. Each one has a control run outside
// the sandbox where that is possible, so a test cannot pass just because the target is broken.
func TestProfileBlocksEscapes(t *testing.T) {
	home, _ := os.UserHomeDir()
	work := t.TempDir()
	outside, err := os.MkdirTemp("/private/tmp", "ks-t")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(outside)
	pf := filepath.Join(work, "p.sb")
	os.WriteFile(pf, []byte(Profile(home, []string{work}, nil)), 0o600)
	sandboxed := func(sh string) error {
		a := Argv(pf, []string{"/bin/sh", "-c", sh})
		return exec.Command(a[0], a[1:]...).Run()
	}

	os.WriteFile(filepath.Join(outside, "f"), []byte("x"), 0o600)
	if sandboxed("cat "+outside+"/f") == nil {
		t.Error("read of /tmp allowed")
	}

	sock := filepath.Join(outside, "s")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	nc := "/usr/bin/nc -U " + sock + " </dev/null"
	if err := exec.Command("/bin/sh", "-c", nc).Run(); err != nil {
		t.Fatal("control: socket not reachable outside the sandbox:", err)
	}
	if sandboxed(nc) == nil {
		t.Error("connection to a unix socket outside the work dir allowed")
	}

	victim := exec.Command("/bin/sleep", "30")
	if err := victim.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { victim.Process.Kill(); victim.Wait() }()
	if sandboxed(fmt.Sprintf("kill %d", victim.Process.Pid)) == nil {
		t.Error("signal to a process outside the sandbox allowed")
	}
	if err := sandboxed("sleep 5 & kill $!"); err != nil {
		t.Error("signal to a child denied:", err)
	}

	defer exec.Command("/usr/bin/defaults", "delete", "kubesilicon.sandbox-test").Run()
	if sandboxed("/usr/bin/defaults write kubesilicon.sandbox-test a 1 && /usr/bin/defaults read kubesilicon.sandbox-test") == nil {
		t.Error("writing user preferences allowed")
	}
}
