package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Matheus-Thurler/kubesilicon/internal/vm"
)

// proc is a launcher process. The daemon finds it again after a restart through <dir>/pid.
type proc struct{ pid int }

func (p *proc) alive() bool { return syscall.Kill(p.pid, 0) == nil }

// stop asks the launcher to shut the guest down (it waits up to 20 s), then kills it.
func (p *proc) stop() {
	_ = syscall.Kill(p.pid, syscall.SIGTERM)
	for i := 0; i < 250 && p.alive(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if p.alive() {
		_ = syscall.Kill(p.pid, syscall.SIGKILL)
	}
}

// spawn starts a detached launcher (own session) for the VM described by c.
func spawn(dir string, c vm.Config) (*proc, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	logf, err := os.OpenFile(filepath.Join(dir, "launcher.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	defer logf.Close()
	cmd := exec.Command(self, "launch",
		"-disk", c.Disk, "-seed", c.Seed, "-efi", c.EFIVars, "-console", c.Console, "-sock", c.Sock,
		"-cpus", strconv.Itoa(int(c.CPUs)), "-mem", strconv.FormatUint(c.MemBytes, 10))
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go cmd.Wait() // reap, so a finished launcher does not linger as a zombie
	if err := os.WriteFile(filepath.Join(dir, "pid"), []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		return nil, err
	}
	return &proc{cmd.Process.Pid}, nil
}

// adopt returns the launcher recorded in <dir>/pid if it is still ours and alive.
// The command line check guards against pid reuse.
func adopt(dir string) *proc {
	b, err := os.ReadFile(filepath.Join(dir, "pid"))
	if err != nil {
		return nil
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return nil
	}
	p := &proc{pid}
	if !p.alive() {
		return nil
	}
	out, _ := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if !strings.Contains(string(out), " launch ") || !strings.Contains(string(out), dir) {
		return nil
	}
	return p
}
