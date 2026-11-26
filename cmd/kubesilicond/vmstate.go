package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// Files in a VM directory about how its launcher ended.
const (
	exitFile     = "vmexit"   // written by the launcher when the guest stops by itself: exitShutdown or exitError
	deadFile     = "dead"     // its mtime is when the daemon saw the VM down and started the restart delay
	restartsFile = "restarts" // times restartPolicy brought the VM back

	exitShutdown = "shutdown"
	exitError    = "error"
)

// shouldRestart applies spec.restartPolicy. exit is "" when the launcher died without a word.
func shouldRestart(policy, exit string) bool {
	return policy == "Always" || policy == "OnFailure" && exit != exitShutdown
}

// restartDelay doubles from 10 s up to 5 min, so a VM that dies at boot is not restarted in a tight loop.
// ponytail: the count only resets on spec.running=false; reset it after a long healthy run if that matters.
func restartDelay(n int) time.Duration { return min(10*time.Second<<min(n, 5), 5*time.Minute) }

func readString(dir, name string) string {
	b, _ := os.ReadFile(filepath.Join(dir, name))
	return strings.TrimSpace(string(b))
}

func restarts(dir string) int {
	n, _ := strconv.Atoi(readString(dir, restartsFile))
	return n
}

// forget drops the memory of how the last run ended, so the next start is a fresh one.
func forget(dir string, names ...string) {
	for _, n := range names {
		os.Remove(filepath.Join(dir, n))
	}
}

// exited handles a VM that should run but is down: it stays Stopped or, if spec.restartPolicy
// says so, is started again after a delay.
func (d *daemon) exited(ctx context.Context, api dynamic.ResourceInterface, u *unstructured.Unstructured, key string) (time.Duration, error) {
	dir := d.dir(key)
	exit := readString(dir, exitFile)
	policy, _, _ := unstructured.NestedString(u.Object, "spec", "restartPolicy")
	if !shouldRestart(policy, exit) {
		msg := "VM process exited (see launcher.log and console.log in " + dir + "); toggle spec.running to start it again"
		if exit == exitShutdown {
			msg = "the guest shut down; toggle spec.running to start it again"
		}
		return 0, d.setStatus(ctx, api, u, "Stopped", "", msg, false)
	}
	marker := filepath.Join(dir, deadFile)
	st, err := os.Stat(marker)
	if err != nil {
		if err := os.WriteFile(marker, nil, 0o644); err != nil {
			return 0, err
		}
		if st, err = os.Stat(marker); err != nil {
			return 0, err
		}
	}
	n := restarts(dir)
	if wait := restartDelay(n) - time.Since(st.ModTime()); wait > 0 {
		return wait, d.setStatus(ctx, api, u, "Restarting", "", fmt.Sprintf("VM is down, restart %d in %s (restartPolicy %s)", n+1, restartDelay(n), policy), false)
	}
	if err := os.WriteFile(filepath.Join(dir, restartsFile), []byte(strconv.Itoa(n+1)), 0o644); err != nil {
		return 0, err
	}
	forget(dir, exitFile, deadFile, "pid")
	d.mu.Lock()
	delete(d.vms, key)
	d.mu.Unlock()
	return time.Millisecond, nil
}

// probe is the readiness check: the guest accepts TCP connections on spec.readinessPort.
func probe(ip string, port int64) bool {
	c, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.FormatInt(port, 10)), time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// vmStatus is the new status. The Ready condition keeps its lastTransitionTime while it does not change.
func vmStatus(cur map[string]any, phase, ip, msg string, ready bool, restartCount int, now string) map[string]any {
	status, reason := "False", phase
	if ready {
		status, reason = "True", "PortOpen"
	} else if phase == "Running" {
		reason = "PortClosed"
	}
	since := now
	if old, _ := cur["conditions"].([]any); len(old) > 0 {
		if c, _ := old[0].(map[string]any); c["status"] == status {
			if t, _ := c["lastTransitionTime"].(string); t != "" {
				since = t
			}
		}
	}
	return map[string]any{
		"phase": phase, "ip": ip, "message": msg, "restartCount": int64(restartCount),
		"conditions": []any{map[string]any{"type": "Ready", "status": status, "reason": reason, "lastTransitionTime": since}},
	}
}
