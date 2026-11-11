package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/Matheus-Thurler/kubesilicon/internal/mem"
)

var jobGVR = schema.GroupVersionResource{Group: "kubesilicon.io", Version: "v1", Resource: "siliconjobs"}

const jobFinalizer = "kubesilicon.io/job"

func (d *daemon) jobDir(key string) string {
	return filepath.Join(d.root, "jobs", strings.ReplaceAll(key, "/", "_"))
}

// reconcileJob drives one SiliconJob. The job itself runs in a detached "runjob" process that
// writes <dir>/exit when done; this function only starts it and mirrors its state into status.
func (d *daemon) reconcileJob(ctx context.Context, key string) (time.Duration, error) {
	ns, name, _ := strings.Cut(key, "/")
	api := d.c.Resource(jobGVR).Namespace(ns)
	dir := d.jobDir(key)
	u, err := api.Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		d.stopJob(key)
		return 0, os.RemoveAll(dir)
	}
	if err != nil {
		return 0, err
	}
	if u.GetDeletionTimestamp() != nil {
		d.stopJob(key)
		if err := os.RemoveAll(dir); err != nil {
			return 0, err
		}
		return 0, patchFinalizers(ctx, api, u, remove(u.GetFinalizers(), jobFinalizer))
	}
	if !has(u.GetFinalizers(), jobFinalizer) {
		if u, err = patchFinalizersGet(ctx, api, u, append(u.GetFinalizers(), jobFinalizer)); err != nil {
			return 0, err
		}
	}

	if code, msg, done := readExit(dir); done { // terminal
		phase := "Succeeded"
		switch code {
		case 0:
		case 130:
			phase = "Cancelled"
		default:
			phase = "Failed"
		}
		return 0, d.setJobStatus(ctx, api, u, phase, msg)
	}

	running, ok, _ := unstructured.NestedBool(u.Object, "spec", "running")
	if ok && !running {
		d.stopJob(key)
		os.WriteFile(filepath.Join(dir, "exit"), []byte("130 cancelled by spec.running=false"), 0o644)
		return 0, d.setJobStatus(ctx, api, u, "Cancelled", "cancelled by spec.running=false")
	}

	p := d.lookupJob(key)
	ph, _, _ := unstructured.NestedString(u.Object, "status", "phase")
	switch {
	case p == nil && isFinal(u):
		// The cluster says it already finished but the local result is gone (state dir removed, or
		// another daemon cleaned it up). Never run it again; the recorded status stays.
		return 0, nil
	case p == nil && ph != "Running":
		wait, err := d.admit(key, u)
		if err != nil {
			return 0, d.setJobStatus(ctx, api, u, "Failed", err.Error())
		}
		if wait != "" {
			return 5 * time.Second, d.setJobStatus(ctx, api, u, "Pending", wait)
		}
		np, err := d.startJob(dir, u)
		if err != nil {
			return 0, d.setJobStatus(ctx, api, u, "Failed", err.Error())
		}
		d.mu.Lock()
		d.jobs[key] = np
		d.mu.Unlock()
		return 2 * time.Second, d.setJobStatus(ctx, api, u, "Running", "")
	case p != nil && p.alive():
		return 5 * time.Second, d.setJobStatus(ctx, api, u, "Running", "")
	}
	// The runner is gone and wrote no exit file: the Mac rebooted, or it was killed. Retry up to
	// spec.backoffLimit times (script failures are not retried: they wrote an exit file).
	n := attempts(u)
	limit, ok, _ := unstructured.NestedInt64(u.Object, "spec", "backoffLimit")
	if !ok {
		limit = 2
	}
	if int64(n) >= limit {
		return 0, d.setJobStatus(ctx, api, u, "Failed", fmt.Sprintf("interrupted %d time(s) without a result, giving up (see launcher.log in %s)", n+1, dir))
	}
	for _, f := range []string{"venv", "work", "pid", "exit"} {
		os.RemoveAll(filepath.Join(dir, f))
	}
	if f, err := os.OpenFile(filepath.Join(dir, "job.log"), os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintf(f, "[kubesilicon] interrupted, retry %d of %d\n", n+1, limit)
		f.Close()
	}
	d.mu.Lock()
	delete(d.jobs, key)
	d.mu.Unlock()
	return time.Second, d.setJobStatusN(ctx, api, u, "Pending", fmt.Sprintf("interrupted, retry %d of %d", n+1, limit), n+1)
}

func attempts(u *unstructured.Unstructured) int {
	n, _, _ := unstructured.NestedInt64(u.Object, "status", "attempts")
	return int(n)
}

func (d *daemon) startJob(dir string, u *unstructured.Unstructured) (*proc, error) {
	spec, _ := u.Object["spec"].(map[string]any)
	js := jobSpec{Python: "3.12", Timeout: 3600}
	b, _ := json.Marshal(spec) // jobSpec json tags are the CRD field names
	if err := json.Unmarshal(b, &js); err != nil {
		return nil, err
	}
	if js.Timeout <= 0 {
		js.Timeout = 3600
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	b, _ = json.Marshal(js)
	for f, data := range map[string][]byte{"spec.json": b, "uid": []byte(u.GetUID())} {
		if err := os.WriteFile(filepath.Join(dir, f), data, 0o600); err != nil {
			return nil, err
		}
	}
	return spawn(dir, "runjob", dir)
}

// running is a job with a live runner: what it declared (0: nothing, it runs alone) and what it uses now.
type running struct{ req, used uint64 }

// admit decides whether the job may start now: "" means yes, anything else is the reason it waits.
// Jobs start in creation order, so a big job at the head of the line is never starved by small ones.
func (d *daemon) admit(key string, u *unstructured.Unstructured) (string, error) {
	var js jobSpec
	b, _ := json.Marshal(u.Object["spec"])
	json.Unmarshal(b, &js)
	if js.Memory != "" && js.memBytes() == 0 {
		return "", fmt.Errorf("spec.memory %q must be a positive quantity like 4Gi", js.Memory)
	}
	avail, total := mem.Available()
	wait, err := admission(js.memBytes(), d.runningJobs(key), avail, total, d.reserve)
	if err != nil { // can never run: say so now, not after its turn in the queue
		return "", err
	}
	if !d.firstInLine(u) {
		return "waiting in the queue", nil
	}
	return wait, nil
}

// admission is the queue policy. The Mac GPU has no partitions; what runs out is the unified
// memory it shares with the desktop. So a job that declares spec.memory starts when that much
// is really available: what macOS reports now, minus what running jobs declared and have not
// used yet, minus a reserve for everything else. A job that declares nothing runs alone.
func admission(req uint64, others []running, avail, total, reserve uint64) (string, error) {
	if req > 0 && req+reserve > total {
		return "", fmt.Errorf("spec.memory %s can never fit: this Mac has %s and keeps %s free", gib(req), gib(total), gib(reserve))
	}
	if req == 0 {
		if len(others) > 0 {
			return "waiting for the running jobs to finish (no spec.memory: this job runs alone)", nil
		}
		return "", nil
	}
	free := int64(avail) - int64(reserve)
	for _, o := range others {
		if o.req == 0 {
			return "waiting for a job that runs alone (it has no spec.memory)", nil
		}
		if o.req > o.used {
			free -= int64(o.req - o.used)
		}
	}
	if int64(req) > free {
		return fmt.Sprintf("waiting for memory: needs %s, %s free", gib(req), gib(uint64(max(free, 0)))), nil
	}
	return "", nil
}

// runningJobs lists the jobs other than key that have a live runner (also after a daemon restart).
func (d *daemon) runningJobs(key string) []running {
	entries, _ := os.ReadDir(filepath.Join(d.root, "jobs"))
	self := strings.ReplaceAll(key, "/", "_")
	var out []running
	for _, e := range entries {
		dir := filepath.Join(d.root, "jobs", e.Name())
		if e.Name() == self || adopt(dir, "runjob") == nil {
			continue
		}
		var js jobSpec
		b, _ := os.ReadFile(filepath.Join(dir, "spec.json"))
		json.Unmarshal(b, &js)
		b, _ = os.ReadFile(filepath.Join(dir, "mem")) // written by the runner's watchdog; absent until its first sample
		used, _ := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
		out = append(out, running{js.memBytes(), used})
	}
	return out
}

// firstInLine reports whether no older job is still waiting to start.
func (d *daemon) firstInLine(u *unstructured.Unstructured) bool {
	for _, o := range d.jstore.List() {
		x := o.(*unstructured.Unstructured)
		if x.GetUID() != u.GetUID() && waiting(x) && x.GetCreationTimestamp().Time.Before(u.GetCreationTimestamp().Time) {
			return false
		}
	}
	return true
}

// waiting: the job has not started and still wants to.
func waiting(u *unstructured.Unstructured) bool {
	ph, _, _ := unstructured.NestedString(u.Object, "status", "phase")
	run, ok, _ := unstructured.NestedBool(u.Object, "spec", "running")
	return u.GetDeletionTimestamp() == nil && !isFinal(u) && ph != "Running" && (!ok || run)
}

func (d *daemon) lookupJob(key string) *proc {
	d.mu.Lock()
	defer d.mu.Unlock()
	if p := d.jobs[key]; p != nil {
		return p
	}
	if p := adopt(d.jobDir(key), "runjob"); p != nil {
		d.jobs[key] = p
		return p
	}
	return nil
}

func (d *daemon) stopJob(key string) {
	p := d.lookupJob(key)
	d.mu.Lock()
	delete(d.jobs, key)
	d.mu.Unlock()
	if p != nil {
		p.stop()
	}
}

func readExit(dir string) (code int, msg string, done bool) {
	b, err := os.ReadFile(filepath.Join(dir, "exit"))
	if err != nil {
		return 0, "", false
	}
	c, m, _ := strings.Cut(string(b), " ")
	code, err = strconv.Atoi(c)
	if err != nil {
		return 1, "unreadable exit file", true
	}
	return code, m, true
}

// logTail is how a pod in the cluster (a Kubeflow step) can see the job output, which lives on the Mac.
const logTail = 4096

func (d *daemon) setJobStatus(ctx context.Context, api dynamic.ResourceInterface, u *unstructured.Unstructured, phase, msg string) error {
	return d.setJobStatusN(ctx, api, u, phase, msg, attempts(u))
}

func (d *daemon) setJobStatusN(ctx context.Context, api dynamic.ResourceInterface, u *unstructured.Unstructured, phase, msg string, n int) error {
	tail := readTail(filepath.Join(d.jobDir(u.GetNamespace()+"/"+u.GetName()), "job.log"), logTail)
	cur, _, _ := unstructured.NestedMap(u.Object, "status")
	if cur["phase"] == phase && cur["message"] == msg && cur["logTail"] == tail && attempts(u) == n {
		return nil
	}
	u = u.DeepCopy()
	if err := unstructured.SetNestedField(u.Object, map[string]any{"phase": phase, "message": msg, "logTail": tail, "attempts": int64(n)}, "status"); err != nil {
		return err
	}
	_, err := api.UpdateStatus(ctx, u, metav1.UpdateOptions{})
	if err == nil {
		log.Printf("job %s/%s: %s %s", u.GetNamespace(), u.GetName(), phase, msg)
	}
	return err
}

func readTail(path string, n int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > n {
		f.Seek(-n, io.SeekEnd)
	}
	b, _ := io.ReadAll(f)
	return strings.ToValidUTF8(string(b), "")
}

func isFinal(u *unstructured.Unstructured) bool {
	ph, _, _ := unstructured.NestedString(u.Object, "status", "phase")
	return ph == "Succeeded" || ph == "Failed" || ph == "Cancelled"
}
