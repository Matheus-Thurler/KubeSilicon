#!/bin/bash
# End-to-end test. Needs the "kubesilicon" kubeconfig context and the CRD applied.
# Usage: hack/e2e.sh
set -euo pipefail
cd "$(dirname "$0")/.."
K="kubectl --context kubesilicon"
ROOT=$HOME/.kubesilicon
KEY=$ROOT/e2e-key
IMG=https://cloud-images.ubuntu.com/releases/noble/release/ubuntu-24.04-server-cloudimg-arm64.img
log() { echo "== $*"; }
phase() { $K get svm e2e -o jsonpath='{.status.phase}' 2>/dev/null || true; }
wait_phase() { for _ in $(seq 120); do [ "$(phase)" = "$1" ] && return 0; sleep 2; done; echo "timeout waiting for $1 (now: $(phase))"; $K get svm e2e -o yaml; exit 1; }
ip() { $K get svm e2e -o jsonpath='{.status.ip}'; }
start_daemon() { ./bin/kubesilicond -context kubesilicon >>"$ROOT/kubesilicond.log" 2>&1 & DPID=$!; }
stop_daemon() { kill -TERM "$DPID"; wait "$DPID" 2>/dev/null || true; }
cleanup() { kill -TERM "$DPID" 2>/dev/null || true; $K delete svm e2e --ignore-not-found --wait=false >/dev/null; }
trap cleanup EXIT

mkdir -p "$ROOT"
[ -f "$KEY" ] || ssh-keygen -q -t ed25519 -N '' -f "$KEY"
make -s bin/kubesilicond
start_daemon; sleep 2

log "create SiliconVM"
$K apply -f - <<YAML
apiVersion: kubesilicon.io/v1
kind: SiliconVM
metadata: { name: e2e }
spec:
  cpu: 2
  memory: 2Gi
  disk: { source: $IMG, size: 10Gi }
  cloudInit:
    userData: |
      #cloud-config
      users:
        - name: ubuntu
          sudo: ALL=(ALL) NOPASSWD:ALL
          shell: /bin/bash
          ssh_authorized_keys: [$(cat "$KEY.pub")]
YAML
wait_phase Running
IP=$(ip); echo "IP=$IP"

log "SSH from the Mac"
for _ in $(seq 30); do ssh -i "$KEY" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5 ubuntu@"$IP" 'hostname; uname -m; df -h / | tail -1' 2>/dev/null && break; sleep 3; done

log "a pod in the cluster reaches the VM"
$K run e2e-probe --rm -i --restart=Never --image=busybox:1.36 --command -- sh -c "ping -c2 -W2 $IP && nc -zv -w3 $IP 22"

log "running=false"
$K patch svm e2e --type=merge -p '{"spec":{"running":false}}'
wait_phase Stopped
[ -z "$(ip)" ] && echo "ip cleared"
[ -f "$ROOT/vms/kubesilicon_e2e/disk.raw" ] && echo "disk kept"

log "running=true (disk reused)"
$K patch svm e2e --type=merge -p '{"spec":{"running":true}}'
wait_phase Running

log "restart daemon: VM goes down, status Stopped, comes back by itself"
stop_daemon
[ "$(phase)" = Stopped ] && echo "status Stopped when the daemon stopped"
start_daemon
wait_phase Running

log "delete"
$K delete svm e2e --timeout=120s
[ ! -d "$ROOT/vms/kubesilicon_e2e" ] && echo "local state removed"
echo "E2E OK"
