package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"
)

// cleanup removes the state (and stops the launcher) of VMs whose object was deleted, or deleted
// and recreated with another UID, while the daemon was down. A directory with no uid file
// (created by an older version) is kept and stamped.
func (d *daemon) cleanup(store cache.Store) {
	entries, _ := os.ReadDir(filepath.Join(d.root, "vms"))
	for _, e := range entries {
		dir := d.dir(strings.Replace(e.Name(), "_", "/", 1))
		uidFile := filepath.Join(dir, "uid")
		uid, _ := os.ReadFile(uidFile)
		if obj, ok, _ := store.GetByKey(strings.Replace(e.Name(), "_", "/", 1)); ok {
			cur := string(obj.(*unstructured.Unstructured).GetUID())
			if len(uid) == 0 {
				os.WriteFile(uidFile, []byte(cur), 0o644)
				continue
			}
			if string(uid) == cur {
				continue
			}
		}
		log.Printf("removing orphan VM state %s", e.Name())
		if p := adopt(dir); p != nil {
			p.stop()
		}
		os.RemoveAll(dir)
	}
}
