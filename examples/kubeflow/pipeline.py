"""Kubeflow Pipelines component that runs a script on the Mac GPU through a SiliconJob.

The step only creates the SiliconJob in its own namespace and waits for it; kubesilicond runs
the script natively on macOS (PyTorch MPS / MLX) and mirrors phase and log tail into status.
"""
from kfp import compiler, dsl


@dsl.component(base_image="python:3.12-slim", packages_to_install=["kubernetes==31.0.0"])
def silicon_job(script: str, dependencies: list, timeout_seconds: int = 1800, pending_timeout_seconds: int = 900) -> str:
    import time
    import uuid

    from kubernetes import client, config

    config.load_incluster_config()
    ns = open("/var/run/secrets/kubernetes.io/serviceaccount/namespace").read().strip()
    api = client.CustomObjectsApi()
    args = ("kubesilicon.io", "v1", ns, "siliconjobs")
    name = f"kfp-{uuid.uuid4().hex[:8]}"
    api.create_namespaced_custom_object(*args, {
        "apiVersion": "kubesilicon.io/v1", "kind": "SiliconJob", "metadata": {"name": name},
        "spec": {"script": script, "dependencies": dependencies, "timeoutSeconds": timeout_seconds},
    })
    shown, phase, waiting_since = 0, "", time.time()
    try:
        while True:
            st = api.get_namespaced_custom_object(*args, name).get("status", {})
            tail = st.get("logTail", "")
            if len(tail) > shown:  # logTail is the last 4 KiB; print what is new
                print(tail[shown:], end="", flush=True)
                shown = len(tail)
            phase = st.get("phase", "")
            if phase in ("Succeeded", "Failed", "Cancelled"):
                break
            if phase == "Running":
                waiting_since = time.time()
            elif time.time() - waiting_since > pending_timeout_seconds:
                raise RuntimeError(f"SiliconJob {name} did not start in {pending_timeout_seconds}s "
                                   f"(phase '{phase or 'none'}'): is kubesilicond running on the Mac?")
            time.sleep(3)
    finally:
        if phase not in ("Succeeded", "Failed", "Cancelled"):  # step killed: stop the Mac job too
            api.delete_namespaced_custom_object(*args, name)
    if phase != "Succeeded":
        raise RuntimeError(f"SiliconJob {name}: {phase}: {st.get('message', '')}")
    return st.get("message", "")


SCRIPT = """
import torch, time
dev = "mps" if torch.backends.mps.is_available() else "cpu"
print("device:", dev)
torch.manual_seed(0)
x = torch.randn(8192, 32, device=dev)
y = (x[:, :4].sum(1, keepdim=True) > 0).float()
m = torch.nn.Sequential(torch.nn.Linear(32, 64), torch.nn.ReLU(), torch.nn.Linear(64, 1)).to(dev)
opt = torch.optim.Adam(m.parameters(), 1e-2)
t = time.time()
for step in range(300):
    loss = torch.nn.functional.binary_cross_entropy_with_logits(m(x), y)
    opt.zero_grad(); loss.backward(); opt.step()
    if step % 100 == 0: print("step", step, "loss", round(loss.item(), 4))
print("final loss", round(loss.item(), 4), "in", round(time.time() - t, 2), "s on", dev)
"""


@dsl.pipeline(name="train-on-mac-gpu")
def train_on_mac_gpu():
    silicon_job(script=SCRIPT, dependencies=["torch"])


if __name__ == "__main__":
    compiler.Compiler().compile(train_on_mac_gpu, "train_on_mac_gpu.yaml")
