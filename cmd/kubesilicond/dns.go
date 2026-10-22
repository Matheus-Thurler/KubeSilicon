package main

import (
	"context"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

var (
	svcGVR   = schema.GroupVersionResource{Version: "v1", Resource: "services"}
	sliceGVR = schema.GroupVersionResource{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"}
)

// syncDNS publishes the VM as <name>.<namespace>.svc: a headless Service with no selector
// plus an EndpointSlice holding the VM IP (none when ip is ""). Both are owned by the
// SiliconVM, so Kubernetes garbage-collects them on delete.
func (d *daemon) syncDNS(ctx context.Context, u *unstructured.Unstructured, ip string) error {
	meta := map[string]any{
		"name": u.GetName(), "namespace": u.GetNamespace(),
		"ownerReferences": []any{map[string]any{
			"apiVersion": "kubesilicon.io/v1", "kind": "SiliconVM", "name": u.GetName(), "uid": string(u.GetUID()),
		}},
	}
	endpoints := []any{}
	if ip != "" {
		endpoints = append(endpoints, map[string]any{"addresses": []any{ip}, "conditions": map[string]any{"ready": true}})
	}
	sliceMeta := map[string]any{"labels": map[string]any{"kubernetes.io/service-name": u.GetName()}}
	for k, v := range meta {
		sliceMeta[k] = v
	}
	for _, o := range []struct {
		gvr schema.GroupVersionResource
		obj map[string]any
	}{
		{svcGVR, map[string]any{"apiVersion": "v1", "kind": "Service", "metadata": meta, "spec": map[string]any{"clusterIP": "None"}}},
		{sliceGVR, map[string]any{"apiVersion": "discovery.k8s.io/v1", "kind": "EndpointSlice", "metadata": sliceMeta, "addressType": "IPv4", "endpoints": endpoints}},
	} {
		b, err := json.Marshal(o.obj)
		if err != nil {
			return err
		}
		force := true
		if _, err := d.c.Resource(o.gvr).Namespace(u.GetNamespace()).Patch(ctx, u.GetName(), types.ApplyPatchType, b,
			metav1.PatchOptions{FieldManager: "kubesilicond", Force: &force}); err != nil {
			return err
		}
	}
	return nil
}
