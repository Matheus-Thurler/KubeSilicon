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
log e2e-iso | grep -q "denied $HOME/.ssh/id_ed25519" && log e2e-iso | grep -q "denied $HOME/e2e-should-not-exist" && log e2e-iso | grep -q "inside ok" && ! log e2e-iso | grep -q LEAK
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
ok "second job waits (one GPU job at a time)"
k patch sjob e2e-long --type=merge -p '{"spec":{"running":false}}' >/dev/null
wait_phase e2e-long Cancelled
wait_phase e2e-queued Succeeded
ok "cancel works and the queue moves on"

# artifacts: a tiny HTTP server that serves in.txt and stores PUTs
D=$(mktemp -d); echo "hello" >"$D/in.txt"
python3 -I - "$D" >/dev/null 2>&1 <<'PY' &
import http.server, sys, os
d = sys.argv[1]
class H(http.server.SimpleHTTPRequestHandler):
    def __init__(s, *a, **k): super().__init__(*a, directory=d, **k)
    def do_PUT(s):
        open(os.path.join(d, 'out.txt'), 'wb').write(s.rfile.read(int(s.headers['Content-Length']))); s.send_response(200); s.end_headers()
http.server.HTTPServer(('127.0.0.1', 18765), H).serve_forever()
PY
SRV=$!; sleep 1
mk e2e-art "  inputs: [{ url: 'http://127.0.0.1:18765/in.txt', path: data/in.txt }]
  outputs: [{ url: 'http://127.0.0.1:18765/out.txt', path: out.txt }]
  script: |
    open('out.txt', 'w').write(open('data/in.txt').read().upper())"
wait_phase e2e-art Succeeded
[ "$(cat "$D/out.txt")" = "HELLO" ]
ok "inputs downloaded, outputs uploaded"

mk e2e-badpath "  inputs: [{ url: 'http://127.0.0.1:18765/in.txt', path: ../escape.txt }]
  script: |
    print('x')"
wait_phase e2e-badpath Failed
ok "input path escaping the work dir is rejected"
echo "ALL OK"
