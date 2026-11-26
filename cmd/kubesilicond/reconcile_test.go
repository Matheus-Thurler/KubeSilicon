package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"
)

// jobFixture is a daemon on a fake cluster holding one job "default/j"; run is the runner it starts.
func jobFixture(t *testing.T, spec map[string]any, run *proc) (*daemon, dynamic.ResourceInterface, string) {
	t.Helper()
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kubesilicon.io/v1", "kind": "SiliconJob",
		"metadata": map[string]any{"name": "j", "namespace": "default", "uid": "u1"},
		"spec":     spec,
	}}
	c := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{jobGVR: "SiliconJobList"}, u)
	d := &daemon{c: c, root: t.TempDir(), jobs: map[string]*proc{}, jstore: cache.NewStore(cache.MetaNamespaceKeyFunc),
		runner: func(string) (*proc, error) { return run, nil }}
	return d, c.Resource(jobGVR).Namespace("default"), d.jobDir("default/j")
}

func status(t *testing.T, api dynamic.ResourceInterface) map[string]any {
	t.Helper()
	u, err := api.Get(context.Background(), "j", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	st, _, _ := unstructured.NestedMap(u.Object, "status")
	return st
}

func reconcile(t *testing.T, d *daemon) {
	t.Helper()
	if _, err := d.reconcileJob(context.Background(), "default/j"); err != nil {
		t.Fatal(err)
	}
}

func deadProc(t *testing.T) *proc {
	c := exec.Command("true")
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	return &proc{c.Process.Pid}
}

func TestReconcileJobStartsAndSucceeds(t *testing.T) {
	d, api, dir := jobFixture(t, map[string]any{"script": "print(1)"}, &proc{os.Getpid()})
	reconcile(t, d)
	if st := status(t, api); st["phase"] != "Running" || st["startTime"] == nil {
		t.Fatalf("after start: %v", st)
	}
	if _, err := os.Stat(filepath.Join(dir, "spec.json")); err != nil {
		t.Fatal("runner not given a spec:", err)
	}
	reconcile(t, d) // runner alive: still Running
	if st := status(t, api); st["phase"] != "Running" {
		t.Fatalf("alive runner: %v", st)
	}
	os.WriteFile(filepath.Join(dir, "exit"), []byte("0 done"), 0o644)
	reconcile(t, d)
	if st := status(t, api); st["phase"] != "Succeeded" || st["exitCode"] != int64(0) || st["completionTime"] == nil {
		t.Fatalf("after exit 0: %v", st)
	}
}

func TestReconcileJobFailsOnNonZeroExit(t *testing.T) {
	d, api, dir := jobFixture(t, map[string]any{"script": "x"}, &proc{os.Getpid()})
	reconcile(t, d)
	os.WriteFile(filepath.Join(dir, "exit"), []byte("130 command exited with 130"), 0o644)
	reconcile(t, d)
	if st := status(t, api); st["phase"] != "Failed" || st["exitCode"] != int64(130) {
		t.Fatalf("script exit 130: %v", st)
	}
}

func TestReconcileJobRetriesThenFails(t *testing.T) {
	d, api, _ := jobFixture(t, map[string]any{"script": "x", "backoffLimit": int64(1)}, deadProc(t))
	reconcile(t, d) // starts; the runner is already gone
	reconcile(t, d) // no exit file: retry 1
	if st := status(t, api); st["phase"] != "Pending" || st["attempts"] != int64(1) {
		t.Fatalf("after interruption: %v", st)
	}
	reconcile(t, d) // restarts
	reconcile(t, d) // gone again: backoffLimit reached
	if st := status(t, api); st["phase"] != "Failed" || st["exitCode"] != nil {
		t.Fatalf("after giving up: %v", st)
	}
}

func TestReconcileJobCancelled(t *testing.T) {
	d, api, _ := jobFixture(t, map[string]any{"script": "x", "running": false}, &proc{os.Getpid()})
	reconcile(t, d)
	if st := status(t, api); st["phase"] != "Cancelled" || st["exitCode"] != nil {
		t.Fatalf("running=false: %v", st)
	}
	reconcile(t, d) // the exit file the cancel left must not turn it into Failed
	if st := status(t, api); st["phase"] != "Cancelled" {
		t.Fatalf("second pass: %v", st)
	}
}
