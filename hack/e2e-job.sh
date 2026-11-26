#!/bin/bash
# End-to-end test of SiliconJob. Needs a running kubesilicond for CTX and the CRDs applied.
# Usage: hack/e2e-job.sh [kube-context]   (default kind-kubesilicon)
set -euo pipefail
cd "$(dirname "$0")/.."
CTX=${1:-kind-kubesilicon}
k() { kubectl --context "$CTX" "$@"; }
phase() { k get sjob "$1" -o jsonpath='{.status.phase}' 2>/dev/null || true; }
wait_phase() { for _ in $(seq 120); do [ "$(phase "$1")" = "$2" ] && return 0; sleep 1; done; echo "FAIL: $1 phase is '$(phase "$1")', wanted $2"; k get sjob "$1" -o yaml; exit 1; }
ok() { echo "ok: $*"; }
log() { ./bin/kubesilicon -context "$CTX" logs "$1"; }
# set -e ignores "! cmd" and all but the last command of "a && b": assertions go one per line.
has() { grep -qF -- "$2" <<<"$1" || { echo "FAIL: missing '$2' in:"; echo "$1"; exit 1; }; }
hasnt() { if grep -qF -- "$2" <<<"$1"; then echo "FAIL: found '$2' in:"; echo "$1"; exit 1; fi; }
rejected() { local want=$1 out; shift; if out=$("$@" 2>&1); then echo "FAIL: accepted: $*"; exit 1; fi; has "$out" "$want"; }
SRV=
cleanup() { [ -n "$SRV" ] && kill "$SRV" 2>/dev/null || true; k delete sjob -l e2e=true --ignore-not-found --wait=false >/dev/null; }
trap cleanup EXIT
mk() { # name, extra spec (yaml, indented 2)
  k apply -f - >/dev/null <<Y
apiVersion: kubesilicon.io/v1
kind: SiliconJob
metadata: { name: $1, labels: { e2e: "true" } }
spec:
$2
Y
}
make -s bin/kubesilicon

mk e2e-iso "  script: |
    import os
    for p, w in [('$HOME/.ssh/id_ed25519', 'r'), ('$HOME/e2e-should-not-exist', 'w')]:
        try:
            open(p, w); print('LEAK', p)
        except OSError as e:
            print('denied', p)
    open('inside.txt', 'w').write('x'); print('inside ok')"
wait_phase e2e-iso Succeeded
out=$(log e2e-iso)
has "$out" "denied $HOME/.ssh/id_ed25519"
has "$out" "denied $HOME/e2e-should-not-exist"
has "$out" "inside ok"
hasnt "$out" LEAK
ok "sandbox: secrets unreadable, writes only in the work dir"

mk e2e-timeout "  timeoutSeconds: 3
  script: |
    import time; time.sleep(60)"
wait_phase e2e-timeout Failed
[ "$(k get sjob e2e-timeout -o jsonpath='{.status.message}')" = "timed out" ]
ok "timeout kills the job"

mk e2e-long "  script: |
    import time; time.sleep(60)"
wait_phase e2e-long Running
mk e2e-queued "  script: |
    print('queued job ran')"
sleep 4; [ "$(phase e2e-queued)" = "Pending" ]
ok "a job without spec.memory runs alone"
k patch sjob e2e-long --type=merge -p '{"spec":{"running":false}}' >/dev/null
wait_phase e2e-long Cancelled
wait_phase e2e-queued Succeeded
ok "cancel works and the queue moves on"

# interruption (what a Mac reboot looks like: runner and script vanish without a result)
kill_job() { pkill -9 -f "jobs/default_$1" || true; }
mk e2e-retry "  script: |
    import time; time.sleep(8); print('survived the interruption')"
wait_phase e2e-retry Running; sleep 3; kill_job e2e-retry
wait_phase e2e-retry Succeeded
[ "$(k get sjob e2e-retry -o jsonpath='{.status.attempts}')" = 1 ]
log e2e-retry | grep -q "interrupted, retry 1 of 2"
[ "$(log e2e-retry | grep -c 'survived the interruption')" = 1 ]
ok "interrupted job is retried and counted"

mk e2e-giveup "  backoffLimit: 0
  script: |
    import time; time.sleep(30)"
wait_phase e2e-giveup Running; sleep 3; kill_job e2e-giveup
wait_phase e2e-giveup Failed
k get sjob e2e-giveup -o jsonpath='{.status.message}' | grep -q "giving up"
ok "backoffLimit 0 gives up"

# the API server rejects an empty job and changes to a created one (all but spec.running)
empty() { printf 'apiVersion: kubesilicon.io/v1\nkind: SiliconJob\nmetadata: { name: e2e-empty, labels: { e2e: "true" } }\nspec: { timeoutSeconds: 5 }\n' | k apply -f -; }
rejected "script or command" empty
mk e2e-frozen "  running: false
  script: |
    print(1)"
rejected "cannot change" k patch sjob e2e-frozen --type=merge -p '{"spec":{"script":"print(2)"}}'
rejected "cannot change" k patch sjob e2e-frozen --type=merge -p '{"spec":{"memory":"1Gi"}}'
k patch sjob e2e-frozen --type=merge -p '{"spec":{"running":true}}' >/dev/null
ok "empty and mutated jobs are rejected, spec.running can change"

mk e2e-exit130 "  script: |
    raise SystemExit(130)"
wait_phase e2e-exit130 Failed
[ "$(k get sjob e2e-exit130 -o jsonpath='{.status.exitCode}')" = 130 ]
[ -n "$(k get sjob e2e-exit130 -o jsonpath='{.status.startTime}{.status.completionTime}')" ]
ok "a script that exits 130 is Failed, with exitCode and times"

# escapes through other processes, and multiprocessing under a long job name
mk e2e-a-job-with-a-rather-long-name-for-unix-sockets "  script: |
    import multiprocessing as mp, socket, subprocess
    def put(q): q.put(42)
    if __name__ == '__main__':
        for p in ['/var/run/docker.sock', '$HOME/.docker/run/docker.sock']:
            try:
                socket.socket(socket.AF_UNIX).connect(p); print('LEAK', p)
            except OSError: print('denied socket')
        for c in [['/usr/bin/open', '-g', '-a', 'Calculator'], ['/usr/bin/pbpaste'], ['/bin/ls', '/Volumes']]:
            r = subprocess.run(c, capture_output=True)
            print('LEAK' if r.returncode == 0 and (r.stdout or c[0].endswith('open')) else 'denied', c[0])
        m = mp.Manager(); q = m.Queue(); x = mp.Process(target=put, args=(q,)); x.start(); x.join(); print('mp', q.get())"
wait_phase e2e-a-job-with-a-rather-long-name-for-unix-sockets Succeeded
out=$(log e2e-a-job-with-a-rather-long-name-for-unix-sockets)
has "$out" "mp 42"
has "$out" "denied socket"
has "$out" "denied /usr/bin/open"
hasnt "$out" LEAK
ok "sandbox: docker socket, open, pasteboard and volumes denied; multiprocessing works"

mk e2e-gpu "  memory: 4Gi
  dependencies: [torch, mlx]
  script: |
    import torch, mlx.core as mx
    a = torch.rand(1024, 1024, device='mps'); print('mps', float((a @ a).sum()) > 0)
    b = mx.random.uniform(shape=(1024, 1024)); mx.eval(b @ b); print('mlx ok')"
for _ in $(seq 600); do case "$(phase e2e-gpu)" in Succeeded|Failed) break;; esac; sleep 1; done
wait_phase e2e-gpu Succeeded
out=$(log e2e-gpu)
has "$out" "mps True"
has "$out" "mlx ok"
ok "PyTorch MPS and MLX use the GPU inside the sandbox"

mk e2e-secret "  inputs: [{ url: 'http://127.0.0.1:1/in?X-Amz-Signature=e2esecret', path: in }]
  script: |
    print('x')"
wait_phase e2e-secret Failed
st=$(k get sjob e2e-secret -o jsonpath='{.status}')
has "$st" "127.0.0.1:1/in"
hasnt "$st$(log e2e-secret)" e2esecret
ok "a failed download does not leak the URL signature into status"

# artifacts: a tiny HTTP server that serves in.txt and stores PUTs
D=$(mktemp -d); echo "hello" >"$D/in.txt"
python3 -I - "$D" >/dev/null 2>&1 <<'PY' &
import http.server, sys, os, time
d = sys.argv[1]
class H(http.server.SimpleHTTPRequestHandler):
    def __init__(s, *a, **k): super().__init__(*a, directory=d, **k)
    def do_GET(s):
        if s.path == '/hang': time.sleep(30); return
        super().do_GET()
    def do_PUT(s):
        open(os.path.join(d, 'out.txt'), 'wb').write(s.rfile.read(int(s.headers['Content-Length']))); s.send_response(200); s.end_headers()
http.server.ThreadingHTTPServer(('127.0.0.1', 18765), H).serve_forever()
PY
SRV=$!; sleep 1
mk e2e-art "  inputs: [{ url: 'http://127.0.0.1:18765/in.txt', path: data/in.txt }]
  outputs: [{ url: 'http://127.0.0.1:18765/out.txt', path: out.txt }]
  script: |
    open('out.txt', 'w').write(open('data/in.txt').read().upper())"
wait_phase e2e-art Succeeded
[ "$(cat "$D/out.txt")" = "HELLO" ]
ok "inputs downloaded, outputs uploaded"

mk e2e-hang "  timeoutSeconds: 3
  inputs: [{ url: 'http://127.0.0.1:18765/hang', path: in }]
  script: |
    print('x')"
wait_phase e2e-hang Failed
[ "$(k get sjob e2e-hang -o jsonpath='{.status.message}')" = "timed out" ]
ok "the timeout also covers a hung download"

mk e2e-badpath "  inputs: [{ url: 'http://127.0.0.1:18765/in.txt', path: ../escape.txt }]
  script: |
    print('x')"
wait_phase e2e-badpath Failed
ok "input path escaping the work dir is rejected"

for j in e2e-par1 e2e-par2; do mk $j "  memory: 1Gi
  script: |
    import time; time.sleep(20)"; sleep 1.2; done
wait_phase e2e-par1 Running; wait_phase e2e-par2 Running
ok "jobs that declare spec.memory run at the same time"
mk e2e-over "  memory: 1Gi
  script: |
    import time; b = bytearray(2 << 30); time.sleep(30)"
wait_phase e2e-over Failed
k get sjob e2e-over -o jsonpath='{.status.message}' | grep -q "memory limit exceeded"
ok "a job over its spec.memory is killed"
echo "ALL OK"
