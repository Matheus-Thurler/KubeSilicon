#!/bin/bash
# Creates the least-privilege identity for kubesilicond in a cluster and writes a kubeconfig for it.
# Usage: hack/daemon-kubeconfig.sh <admin-context> [output-file]
# Then:  make install CTX=kubesilicond KCFG=<output-file>
set -euo pipefail
cd "$(dirname "$0")/.."
ctx=${1:?usage: hack/daemon-kubeconfig.sh <admin-context> [output-file]}
out=${2:-$HOME/.kubesilicon/kubeconfig-$ctx}
k() { kubectl --context "$ctx" "$@"; }

k apply -f config/crd/ -f config/rbac/daemon.yaml >/dev/null
for _ in $(seq 20); do tok=$(k -n kubesilicon-system get secret kubesilicond-token -o jsonpath='{.data.token}' 2>/dev/null || true); [ -n "$tok" ] && break; sleep 1; done
[ -n "$tok" ] || { echo "token not issued"; exit 1; }
ca=$(k -n kubesilicon-system get secret kubesilicond-token -o jsonpath='{.data.ca\.crt}')
server=$(kubectl config view --raw -o jsonpath="{.clusters[?(@.name==\"$(kubectl config view -o jsonpath="{.contexts[?(@.name==\"$ctx\")].context.cluster}")\")].cluster.server}")

mkdir -p "$(dirname "$out")"
umask 077
cat >"$out" <<Y
apiVersion: v1
kind: Config
clusters:
  - name: target
    cluster: { server: $server, certificate-authority-data: $ca }
users:
  - name: kubesilicond
    user: { token: $(echo "$tok" | base64 -d) }
contexts:
  - name: kubesilicond
    context: { cluster: target, user: kubesilicond }
current-context: kubesilicond
Y
echo "wrote $out"
echo "install: make install CTX=kubesilicond KCFG=$out"
