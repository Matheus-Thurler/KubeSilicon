package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func TestShouldRestart(t *testing.T) {
	for _, c := range []struct {
		policy, exit string
		want         bool
	}{
		{"", "", false}, {"Never", exitError, false},
		{"OnFailure", "", true}, {"OnFailure", exitError, true}, {"OnFailure", exitShutdown, false},
		{"Always", exitShutdown, true}, {"Always", "", true},
	} {
		if got := shouldRestart(c.policy, c.exit); got != c.want {
			t.Errorf("%q %q: %v", c.policy, c.exit, got)
		}
	}
	if restartDelay(0) != 10*time.Second || restartDelay(1) != 20*time.Second || restartDelay(100) != 5*time.Minute {
		t.Error("delay:", restartDelay(0), restartDelay(1), restartDelay(100))
	}
}

func TestVMStatusKeepsTransitionTime(t *testing.T) {
	a := vmStatus(nil, "Running", "10.0.0.1", "", false, 0, "t1")
	if b := vmStatus(a, "Running", "10.0.0.1", "", false, 0, "t2"); !reflect.DeepEqual(a, b) {
		t.Fatalf("same state changed the status (would update forever): %v %v", a, b)
	}
	c := vmStatus(a, "Running", "10.0.0.1", "", true, 0, "t3")["conditions"].([]any)[0].(map[string]any)
	if c["status"] != "True" || c["lastTransitionTime"] != "t3" {
		t.Fatalf("became ready: %v", c)
	}
}

func TestExitedWaitsThenRestarts(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kubesilicon.io/v1", "kind": "SiliconVM",
		"metadata": map[string]any{"name": "v", "namespace": "default", "uid": "u1"},
		"spec":     map[string]any{"restartPolicy": "OnFailure"},
	}}
	c := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "SiliconVMList"}, u)
	api := c.Resource(gvr).Namespace("default")
	d := &daemon{c: c, root: t.TempDir(), vms: map[string]*proc{"default/v": deadProc(t)}}
	dir := d.dir("default/v")
	os.MkdirAll(dir, 0o755)

	wait, err := d.exited(context.Background(), api, u, "default/v")
	if err != nil || wait <= 0 || wait > 10*time.Second {
		t.Fatal(wait, err)
	}
	got, _ := api.Get(context.Background(), "v", metav1.GetOptions{})
	if ph, _, _ := unstructured.NestedString(got.Object, "status", "phase"); ph != "Restarting" {
		t.Fatal("phase", ph)
	}
	old := time.Now().Add(-time.Minute)
	os.Chtimes(filepath.Join(dir, deadFile), old, old)
	if _, err := d.exited(context.Background(), api, got, "default/v"); err != nil {
		t.Fatal(err)
	}
	if restarts(dir) != 1 || d.vms["default/v"] != nil {
		t.Fatal("not restarted:", restarts(dir), d.vms)
	}

	os.WriteFile(filepath.Join(dir, exitFile), []byte(exitShutdown), 0o644)
	got, _ = api.Get(context.Background(), "v", metav1.GetOptions{})
	if wait, err := d.exited(context.Background(), api, got, "default/v"); err != nil || wait != 0 {
		t.Fatal("a clean shutdown must stay stopped under OnFailure:", wait, err)
	}
}
