package main

import (
	"strings"
	"testing"
)

func TestAdmission(t *testing.T) {
	const gi = uint64(1) << 30
	total, reserve := 24*gi, 2*gi
	for _, c := range []struct {
		name   string
		req    uint64
		others []running
		avail  uint64
		want   string // "" starts, "fail" is an error, anything else must be in the wait reason
	}{
		{"no memory, idle: starts as before", 0, nil, 1 * gi, ""},
		{"no memory, something running: waits", 0, []running{{4 * gi, 1 * gi}}, 20 * gi, "runs alone"},
		{"behind a job that runs alone: waits", 4 * gi, []running{{0, 3 * gi}}, 20 * gi, "runs alone"},
		{"fits next to a running job", 4 * gi, []running{{4 * gi, 4 * gi}}, 8 * gi, ""},
		{"exactly fits", 6 * gi, nil, 8 * gi, ""},
		{"the Mac is busy: waits even when idle", 8 * gi, nil, 9 * gi, "needs 8.0Gi, 7.0Gi free"},
		{"a running job has not grown yet: its share is kept", 4 * gi, []running{{8 * gi, 1 * gi}}, 12 * gi, "needs 4.0Gi, 3.0Gi free"},
		{"a job over its request takes nothing extra", 4 * gi, []running{{2 * gi, 3 * gi}}, 6 * gi, ""},
		{"less available than the reserve", 1 * gi, nil, 1 * gi, "0.0Gi free"},
		{"bigger than the Mac: fails", 23 * gi, nil, 20 * gi, "fail"},
	} {
		got, err := admission(c.req, c.others, c.avail, total, reserve)
		switch {
		case c.want == "fail":
			if err == nil {
				t.Errorf("%s: no error (%q)", c.name, got)
			}
		case err != nil:
			t.Errorf("%s: %v", c.name, err)
		case c.want == "" && got != "":
			t.Errorf("%s: waits: %s", c.name, got)
		case c.want != "" && !strings.Contains(got, c.want):
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestJobSpecMemory(t *testing.T) {
	if got := (jobSpec{Memory: "4Gi"}).memBytes(); got != 4<<30 {
		t.Fatalf("4Gi is %d bytes", got)
	}
	if (jobSpec{}).memBytes() != 0 {
		t.Fatal("no memory must be 0")
	}
	for _, m := range []string{"lots", "-1Gi", "0"} {
		if (jobSpec{Python: "3.12", Memory: m}).validate() == nil {
			t.Errorf("accepted memory %q", m)
		}
	}
}
