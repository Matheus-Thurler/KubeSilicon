package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestSingleton(t *testing.T) {
	root := t.TempDir()
	release, err := singleton(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := singleton(root); err == nil {
		t.Fatal("a second daemon took the lock")
	} else if !strings.Contains(err.Error(), strconv.Itoa(os.Getpid())) {
		t.Errorf("the message does not name the holder: %v", err)
	}
	release()
	if release, err = singleton(root); err != nil {
		t.Fatalf("the lock outlived the daemon: %v", err)
	}
	release()
}
