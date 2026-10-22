// kubesilicond: reconciles SiliconVM objects (kubesilicon.io/v1) into local VMs via Virtualization.framework.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"

	"github.com/Matheus-Thurler/kubesilicon/internal/image"
	"github.com/Matheus-Thurler/kubesilicon/internal/lease"
	"github.com/Matheus-Thurler/kubesilicon/internal/vm"
)

var gvr = schema.GroupVersionResource{Group: "kubesilicon.io", Version: "v1", Resource: "siliconvms"}

const finalizer = "kubesilicon.io/vm"

type daemon struct {
	c    dynamic.Interface
	root string // ~/.kubesilicon
	q    workqueue.TypedRateLimitingInterface[string]

	mu  sync.Mutex
	vms map[string]*proc // launcher per VM; adopted from <dir>/pid after a daemon restart
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "launch" {
		launch(os.Args[2:])
		return
	}
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig (default: ~/.kube/config)")
	kctx := flag.String("context", "", "kubeconfig context (required, avoids acting on the wrong cluster)")
	root := flag.String("root", filepath.Join(os.Getenv("HOME"), ".kubesilicon"), "state directory")
	flag.Parse()
	if *kctx == "" {
		log.Fatal("-context is required")
	}

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.ExplicitPath = *kubeconfig
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{CurrentContext: *kctx}).ClientConfig()
	if err != nil {
		log.Fatal(err)
	}
	c, err := dynamic.NewForConfig(cfg)
	if err != nil {
		log.Fatal(err)
	}
	d := &daemon{c: c, root: *root, vms: map[string]*proc{}, q: workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[string]())}
	for _, sub := range []string{"cache", "vms"} {
		if err := os.MkdirAll(filepath.Join(d.root, sub), 0o755); err != nil {
			log.Fatal(err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	inf := dynamicinformer.NewFilteredDynamicSharedInformerFactory(c, 30*time.Second, metav1.NamespaceAll, nil).ForResource(gvr).Informer()
	enqueue := func(o any) {
		if k, err := cache.DeletionHandlingMetaNamespaceKeyFunc(o); err == nil {
			d.q.Add(k)
		}
	}
	inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: enqueue, UpdateFunc: func(_, n any) { enqueue(n) }, DeleteFunc: enqueue,
	})
	go inf.Run(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), inf.HasSynced) {
		log.Fatal("CRD kubesilicon.io/v1 not found: apply config/crd first")
	}
	log.Printf("kubesilicond ready (context %s)", *kctx)

	go func() { <-ctx.Done(); d.q.ShutDown() }()
	// ponytail: single worker, so provisioning (download) blocks other VMs. Add per-key workers if it hurts.
	for {
		k, shutdown := d.q.Get()
		if shutdown {
			break
		}
		after, err := d.reconcile(ctx, k)
		switch {
		case err != nil:
			log.Printf("%s: %v", k, err)
			d.q.AddRateLimited(k)
		case after > 0:
			d.q.AddAfter(k, after)
		default:
			d.q.Forget(k)
		}
		d.q.Done(k)
	}
	log.Print("kubesilicond stopped; running VMs keep running")
}

func (d *daemon) dir(key string) string {
	return filepath.Join(d.root, "vms", strings.ReplaceAll(key, "/", "_"))
}

func (d *daemon) reconcile(ctx context.Context, key string) (time.Duration, error) {
	ns, name, _ := strings.Cut(key, "/")
	api := d.c.Resource(gvr).Namespace(ns)
	u, err := api.Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		d.stop(key)
		return 0, os.RemoveAll(d.dir(key))
	}
	if err != nil {
		return 0, err
	}

	if u.GetDeletionTimestamp() != nil {
		d.stop(key)
		if err := os.RemoveAll(d.dir(key)); err != nil {
			return 0, err
		}
		return 0, patchFinalizers(ctx, api, u, remove(u.GetFinalizers(), finalizer))
	}
	if !has(u.GetFinalizers(), finalizer) {
		if u, err = patchFinalizersGet(ctx, api, u, append(u.GetFinalizers(), finalizer)); err != nil {
			return 0, err
		}
	}

	running, _, _ := unstructured.NestedBool(u.Object, "spec", "running")
	v := d.lookup(key)
	alive := v != nil && v.alive()

	switch {
	case !running:
		d.stop(key)
		return 0, d.setStatus(ctx, api, u, "Stopped", "", "")
	case v == nil:
		if err := d.setStatus(ctx, api, u, "Provisioning", "", ""); err != nil {
			return 0, err
		}
		nr, err := d.provision(key, u)
		if err != nil {
			return 30 * time.Second, d.setStatus(ctx, api, u, "Failed", "", err.Error())
		}
		d.mu.Lock()
		d.vms[key] = nr
		d.mu.Unlock()
		return time.Second, nil
	case !alive:
		return 0, d.setStatus(ctx, api, u, "Stopped", "", "VM process exited (guest shutdown or error, see launcher.log and console.log in "+d.dir(key)+"); toggle spec.running to start it again")
	}
	ip := lease.IP(lease.File, hostname(u))
	if ip == "" {
		return 3 * time.Second, d.setStatus(ctx, api, u, "Starting", "", "")
	}
	return 0, d.setStatus(ctx, api, u, "Running", ip, "")
}

func (d *daemon) provision(key string, u *unstructured.Unstructured) (*proc, error) {
	spec, _ := u.Object["spec"].(map[string]any)
	src, _, _ := unstructured.NestedString(spec, "disk", "source")
	if src == "" {
		return nil, fmt.Errorf("spec.disk.source is empty")
	}
	size, err := quantity(spec, 0, "disk", "size")
	if err != nil {
		return nil, err
	}
	mem, err := quantity(spec, 2<<30, "memory")
	if err != nil {
		return nil, err
	}
	cpu, _, _ := unstructured.NestedInt64(spec, "cpu")
	if cpu == 0 {
		cpu = 2
	}
	userData, _, _ := unstructured.NestedString(spec, "cloudInit", "userData")

	dir := d.dir(key)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	disk := filepath.Join(dir, "disk.raw")
	if _, err := os.Stat(disk); err != nil { // the disk persists across stop/start
		if err := image.Disk(src, filepath.Join(d.root, "cache"), disk, size); err != nil {
			os.Remove(disk)
			return nil, err
		}
	}
	iso := filepath.Join(dir, "seed.iso")
	if err := image.Seed(filepath.Join(dir, "seed"), iso, hostname(u), string(u.GetUID()), userData); err != nil {
		return nil, err
	}
	return spawn(dir, vm.Config{
		Disk: disk, Seed: iso, EFIVars: filepath.Join(dir, "efivars"), Console: filepath.Join(dir, "console.log"),
		CPUs: uint(cpu), MemBytes: uint64(mem),
	})
}

// hostname is unique per object instance, so DHCP leases of deleted or other
// VMs never match: <name truncated to 54>-<first 8 chars of the UID>.
func hostname(u *unstructured.Unstructured) string {
	n := u.GetName()
	if len(n) > 54 {
		n = n[:54]
	}
	return n + "-" + string(u.GetUID())[:8]
}

// lookup returns the launcher for key, adopting a surviving one after a daemon restart.
func (d *daemon) lookup(key string) *proc {
	d.mu.Lock()
	defer d.mu.Unlock()
	if p := d.vms[key]; p != nil {
		return p
	}
	if p := adopt(d.dir(key)); p != nil {
		d.vms[key] = p
		return p
	}
	return nil
}

func (d *daemon) stop(key string) {
	p := d.lookup(key)
	d.mu.Lock()
	delete(d.vms, key)
	d.mu.Unlock()
	if p != nil {
		p.stop()
	}
}

func (d *daemon) setStatus(ctx context.Context, api dynamic.ResourceInterface, u *unstructured.Unstructured, phase, ip, msg string) error {
	cur, _, _ := unstructured.NestedStringMap(u.Object, "status")
	if cur["phase"] == phase && cur["ip"] == ip && cur["message"] == msg {
		return nil
	}
	if cur["ip"] != ip {
		if err := d.syncDNS(ctx, u, ip); err != nil {
			return err
		}
	}
	u = u.DeepCopy()
	if err := unstructured.SetNestedField(u.Object, map[string]any{"phase": phase, "ip": ip, "message": msg}, "status"); err != nil {
		return err
	}
	_, err := api.UpdateStatus(ctx, u, metav1.UpdateOptions{})
	return err
}

func quantity(spec map[string]any, def int64, path ...string) (int64, error) {
	s, ok, _ := unstructured.NestedString(spec, path...)
	if !ok || s == "" {
		return def, nil
	}
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return 0, fmt.Errorf("spec.%s: %v", strings.Join(path, "."), err)
	}
	return q.Value(), nil
}

func patchFinalizers(ctx context.Context, api dynamic.ResourceInterface, u *unstructured.Unstructured, f []string) error {
	_, err := patchFinalizersGet(ctx, api, u, f)
	return err
}

func patchFinalizersGet(ctx context.Context, api dynamic.ResourceInterface, u *unstructured.Unstructured, f []string) (*unstructured.Unstructured, error) {
	if f == nil {
		f = []string{}
	}
	body := fmt.Sprintf(`{"metadata":{"finalizers":%s}}`, jsonStrings(f))
	return api.Patch(ctx, u.GetName(), types.MergePatchType, []byte(body), metav1.PatchOptions{})
}

func jsonStrings(s []string) string {
	q := make([]string, len(s))
	for i, v := range s {
		q[i] = fmt.Sprintf("%q", v)
	}
	return "[" + strings.Join(q, ",") + "]"
}

func has(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func remove(s []string, v string) []string {
	var out []string
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}
