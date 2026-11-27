package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// singleton takes the lock that allows one daemon per Mac. A second one reconciles the same objects,
// spawns a launcher for the same VM directory and opens the same disk. The lock is held by the open
// file, so the kernel drops it when the process ends, even on SIGKILL; the pid written inside is
// only there to make the message useful.
func singleton(root string) (release func(), err error) {
	path := filepath.Join(root, "lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		holder, _ := os.ReadFile(path) // best effort: the holder writes its pid after locking
		pid := strings.TrimSpace(string(holder))
		f.Close()
		if pid == "" {
			pid = "unknown"
		}
		return nil, fmt.Errorf("another kubesilicond is running (pid %s, lock %s): stop it first", pid, path)
	}
	if err := f.Truncate(0); err != nil {
		f.Close()
		return nil, err
	}
	if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
		f.Close()
		return nil, err
	}
	return func() { f.Close() }, nil
}
