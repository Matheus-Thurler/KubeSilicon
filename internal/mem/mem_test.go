package mem

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestFootprintSumsTheProcessGroup(t *testing.T) {
	one := exec.Command("/bin/sleep", "5")
	one.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := one.Start(); err != nil {
		t.Fatal(err)
	}
	defer one.Process.Kill()
	three := exec.Command("/bin/sh", "-c", "/bin/sleep 5 & /bin/sleep 5 & wait")
	three.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := three.Start(); err != nil {
		t.Fatal(err)
	}
	defer syscall.Kill(-three.Process.Pid, syscall.SIGKILL)
	time.Sleep(300 * time.Millisecond)

	a, b := Footprint(one.Process.Pid), Footprint(three.Process.Pid)
	if a == 0 || b <= a {
		t.Fatalf("one process: %d bytes, group of three: %d bytes", a, b)
	}
	if Footprint(1<<30) != 0 {
		t.Fatal("a missing group has a footprint")
	}
}

func TestAvailable(t *testing.T) {
	avail, total := Available()
	if total < 1<<30 || avail == 0 || avail > total {
		t.Fatalf("avail %d, total %d", avail, total)
	}
}
