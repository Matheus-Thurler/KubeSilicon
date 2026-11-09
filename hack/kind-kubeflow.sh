#!/bin/bash
# Local Kubeflow Pipelines on kind, wired to KubeSilicon jobs. Idempotent.
# Usage: hack/kind-kubeflow.sh [cluster-name]   (default: kubeflow; context kind-<name>)
# Needs Docker Desktop with 12 GB or more of memory (Settings > Resources).
set -euo pipefail
cd "$(dirname "$0")/.."
NAME=${1:-kubeflow}
CTX=kind-$NAME
KFP=2.18.0
CERT=v1.21.3
NODE=kindest/node:v1.34.11@sha256:44e222ee2132dab25ff87301682f89eb82c7880ea3a1bf543bfe9708fd08d67d
k() { kubectl --context "$CTX" "$@"; }

mem=$(docker info --format '{{.MemTotal}}')
[ "$mem" -ge $((11 * 1024 * 1024 * 1024)) ] || { echo "Docker has $((mem >> 30)) GB; give it 12 GB or more"; exit 1; }

kind get clusters | grep -qx "$NAME" || kind create cluster --name "$NAME" --image "$NODE"
k apply -f "https://github.com/cert-manager/cert-manager/releases/download/$CERT/cert-manager.yaml" >/dev/null
k -n cert-manager wait --for=condition=Available deploy --all --timeout=300s >/dev/null
k apply -k "github.com/kubeflow/pipelines/manifests/kustomize/cluster-scoped-resources?ref=$KFP" >/dev/null
k wait --for condition=established --timeout=120s crd/applications.app.k8s.io >/dev/null
k apply -k "github.com/kubeflow/pipelines/manifests/kustomize/env/platform-agnostic?ref=$KFP" >/dev/null
# kfp-metadata-writer has no arm64 image (only lineage metadata in the UI); pipelines run without it.
k -n kubeflow scale deploy metadata-writer --replicas=0 >/dev/null
k apply -f config/crd/ -f examples/kubeflow/rbac.yaml >/dev/null
k -n kubeflow wait --for=condition=Available deploy/ml-pipeline --timeout=600s >/dev/null
echo "ready: context $CTX"
echo "daemon: ./bin/kubesilicond -context $CTX   (or make install CTX=$CTX)"
echo "UI/API: kubectl --context $CTX -n kubeflow port-forward svc/ml-pipeline-ui 8080:80"
echo "demo:   python examples/kubeflow/pipeline.py, then upload train_on_mac_gpu.yaml in the UI"
