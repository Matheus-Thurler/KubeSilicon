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

// vmFixture is a daemon on a fake cluster holding one SiliconVM "default/v", with its VM directory.
func vmFixture(t *testing.T, spec map[string]any) (*daemon, dynamic.ResourceInterface, string) {
	t.Helper()
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kubesilicon.io/v1", "kind": "SiliconVM",
		"metadata": map[string]any{"name": "v", "namespace": "default", "uid": "6f3a1c2e-1f00-4c00-9a00-000000000000"},
		"spec":     spec,
	}}
	c := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "SiliconVMList"}, u)
	d := &daemon{c: c, root: t.TempDir(), vms: map[string]*proc{}}
	return d, c.Resource(gvr).Namespace("default"), d.dir("default/v")
}

// vmPhase reads the status of the single SiliconVM in the fake cluster.
func vmPhase(t *testing.T, api dynamic.ResourceInterface) (phase, ip string) {
	t.Helper()
	u, err := api.Get(context.Background(), "v", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	phase, _, _ = unstructured.NestedString(u.Object, "status", "phase")
	ip, _, _ = unstructured.NestedString(u.Object, "status", "ip")
	return phase, ip
}

// A VM whose launcher ended by itself must not be booted again after a daemon restart: exited()
// applies spec.restartPolicy to it, the same as when the launcher dies with the daemon looking.
func TestReconcileVMDownAfterRestart(t *testing.T) {
	for _, c := range []struct{ exit, policy, want string }{
		{exitError, "Never", "Stopped"},
		{exitError, "OnFailure", "Restarting"},
		{exitShutdown, "OnFailure", "Stopped"},
		{exitShutdown, "Always", "Restarting"},
	} {
		d, api, dir := vmFixture(t, map[string]any{"running": true, "restartPolicy": c.policy})
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, exitFile), []byte(c.exit), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := d.reconcile(context.Background(), "default/v"); err != nil {
			t.Fatalf("%s with %s: %v", c.exit, c.policy, err)
		}
		if phase, _ := vmPhase(t, api); phase != c.want {
			t.Errorf("%s with %s: %s, want %s", c.exit, c.policy, phase, c.want)
		}
		if _, err := os.Stat(filepath.Join(dir, "pid")); err == nil {
			t.Errorf("%s with %s: a launcher was started", c.exit, c.policy)
		}
	}
}

// A pass can write twice (Provisioning, then Failed). The second write compares against the object
// the pass started with, so without keeping that object current the Failed is dropped and the VM
// shows Provisioning for as long as it keeps failing.
func TestReconcileFailedProvisionKeepsItsStatus(t *testing.T) {
	d, api, _ := vmFixture(t, map[string]any{"running": true})
	for i := 1; i <= 2; i++ {
		if _, err := d.reconcile(context.Background(), "default/v"); err != nil {
			t.Fatal(err)
		}
		if phase, _ := vmPhase(t, api); phase != "Failed" {
			t.Fatalf("pass %d: %s after a provision that cannot work, want Failed", i, phase)
		}
	}
}
