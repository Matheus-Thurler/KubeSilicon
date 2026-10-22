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
stop_daemon() { kill -"${1:-TERM}" "$DPID"; wait "$DPID" 2>/dev/null || true; }
vm_ssh() { ssh -i "$KEY" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5 ubuntu@"$IP" "$@" 2>/dev/null; }
no_launcher() { if pgrep -fl "[k]ubesilicond launch"; then echo "FAIL: launcher still running"; exit 1; fi; echo "$1"; }
assert() { local m=$1; shift; if "$@"; then echo "$m"; else echo "FAIL: $m"; exit 1; fi; }
cleanup() { kill -TERM "$DPID" 2>/dev/null || true; $K delete svm e2e --ignore-not-found --wait=false >/dev/null; }
trap cleanup EXIT

mkdir -p "$ROOT"
[ -f "$KEY" ] || ssh-keygen -q -t ed25519 -N '' -f "$KEY"
make -s bin/kubesilicond bin/kubesilicon
CLI="./bin/kubesilicon -context kubesilicon"
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
          lock_passwd: false
          plain_text_passwd: ubuntu
          ssh_authorized_keys: [$(cat "$KEY.pub")]
YAML
wait_phase Running
IP=$(ip); echo "IP=$IP"

log "SSH from the Mac"
for _ in $(seq 30); do vm_ssh 'hostname; uname -m; df -h / | tail -1' && break; sleep 3; done

log "a pod in the cluster reaches the VM by IP and by DNS name"
$K run e2e-probe --rm -i --restart=Never --image=busybox:1.36 --command -- sh -c "
  ping -c2 -W2 $IP && nc -zv -w3 $IP 22 &&
  nslookup e2e.kubesilicon.svc.cluster.local | grep -q $IP && echo 'dns ok' &&
  nc -zv -w3 e2e.kubesilicon.svc.cluster.local 22"

log "CLI: ip, ssh, console"
assert "ip ok" test "$($CLI ip e2e)" = "$IP"
assert "ssh ok" test "$($CLI ssh e2e -- 'echo via-cli')" = "via-cli"
out=$( (sleep 1; printf '\n'; sleep 2; printf 'ubuntu\n'; sleep 2; printf 'ubuntu\n'; sleep 3; printf 'echo MARK$((6*7))\n'; sleep 3) | $CLI console e2e )
assert "console ok (logged in over the serial socket)" grep -q MARK42 <<<"$out"

log "stop the daemon (SIGTERM): the VM keeps running"
stop_daemon
vm_ssh 'echo still up'
assert "status still Running" test "$(phase)" = Running
start_daemon; sleep 5
assert "same IP after daemon restart" test "$(ip)" = "$IP"

log "kill -9 the daemon: VM survives, the new daemon adopts it"
stop_daemon KILL
vm_ssh 'echo still up'
start_daemon; sleep 5
assert "adopted, still Running" test "$(phase)" = Running

log "running=false (through the adopted launcher)"
$K patch svm e2e --type=merge -p '{"spec":{"running":false}}'
wait_phase Stopped
assert "ip cleared" test -z "$(ip)"
assert "dns endpoints cleared" test -z "$($K get endpointslice e2e -o jsonpath='{.endpoints[*].addresses[0]}')"
no_launcher "launcher exited"
assert "disk kept" test -f "$ROOT/vms/kubesilicon_e2e/disk.raw"

log "running=true (disk reused)"
$K patch svm e2e --type=merge -p '{"spec":{"running":true}}'
wait_phase Running

log "delete"
$K delete svm e2e --timeout=120s
assert "local state removed" test ! -d "$ROOT/vms/kubesilicon_e2e"
for _ in $(seq 20); do $K get svc e2e >/dev/null 2>&1 || break; sleep 1; done
if $K get svc e2e >/dev/null 2>&1; then echo "FAIL: service not garbage-collected"; exit 1; fi; echo "service garbage-collected"
no_launcher "no launcher left"
echo "E2E OK"
