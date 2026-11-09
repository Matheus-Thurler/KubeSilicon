package main

import (
	"context"
	"encoding/json"
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
	switch {
	case p == nil:
		// One GPU job at a time: the Mac GPU and its unified memory are shared with the desktop.
		if d.gpuBusy(key) {
			return 5 * time.Second, d.setJobStatus(ctx, api, u, "Pending", "waiting for the running job to finish")
		}
		np, err := d.startJob(dir, u)
		if err != nil {
			return 0, d.setJobStatus(ctx, api, u, "Failed", err.Error())
		}
		d.mu.Lock()
		d.jobs[key] = np
		d.mu.Unlock()
		return 2 * time.Second, d.setJobStatus(ctx, api, u, "Running", "")
	case p.alive():
		return 5 * time.Second, d.setJobStatus(ctx, api, u, "Running", "")
	}
	// The process is gone and wrote no exit file: crashed or killed.
	return 0, d.setJobStatus(ctx, api, u, "Failed", "job process died without a result (see launcher.log in "+dir+")")
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

// gpuBusy reports whether a job other than key has a live runner (also after a daemon restart).
func (d *daemon) gpuBusy(key string) bool {
	entries, _ := os.ReadDir(filepath.Join(d.root, "jobs"))
	self := strings.ReplaceAll(key, "/", "_")
	for _, e := range entries {
		if e.Name() != self && adopt(filepath.Join(d.root, "jobs", e.Name()), "runjob") != nil {
			return true
		}
	}
	return false
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
	tail := readTail(filepath.Join(d.jobDir(u.GetNamespace()+"/"+u.GetName()), "job.log"), logTail)
	cur, _, _ := unstructured.NestedStringMap(u.Object, "status")
	if cur["phase"] == phase && cur["message"] == msg && cur["logTail"] == tail {
		return nil
	}
	u = u.DeepCopy()
	if err := unstructured.SetNestedField(u.Object, map[string]any{"phase": phase, "message": msg, "logTail": tail}, "status"); err != nil {
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
