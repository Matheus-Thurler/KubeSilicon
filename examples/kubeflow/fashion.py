"""Fashion-MNIST in three steps: data (pod), train (Mac GPU through a SiliconJob), evaluate (pod).

Artifacts live in the KFP SeaweedFS bucket. The Mac job reaches it through presigned URLs on
MAC_S3 (kubectl port-forward -n kubeflow svc/seaweedfs 8333:8333); the pods use the service.
"""
from typing import NamedTuple

from kfp import compiler, dsl, kubernetes
from pipeline import silicon_job

BASE = ["boto3==1.35.99", "numpy==2.1.3"]


@dsl.component(base_image="python:3.12-slim", packages_to_install=BASE)
def prepare_data(run_id: str, s3: str = "http://seaweedfs.kubeflow:8333"):
    prefix = f"study/fashion-{run_id}"
    import gzip
    import io
    import urllib.request

    import boto3  # noqa: F401
    import numpy as np
    from botocore.config import Config
    import os
    c = boto3.client("s3", endpoint_url=s3, aws_access_key_id=os.environ["AK"], aws_secret_access_key=os.environ["SK"],
                     region_name="us-east-1", config=Config(s3={"addressing_style": "path"}, signature_version="s3v4"))
    host = "http://fashion-mnist.s3-website.eu-central-1.amazonaws.com/"
    get = lambda n: gzip.decompress(urllib.request.urlopen(host + n, timeout=120).read())
    img = lambda n: np.frombuffer(get(n), np.uint8, offset=16).reshape(-1, 784)
    lab = lambda n: np.frombuffer(get(n), np.uint8, offset=8)
    buf = io.BytesIO()
    np.savez_compressed(buf, xtr=img("train-images-idx3-ubyte.gz"), ytr=lab("train-labels-idx1-ubyte.gz"),
                        xte=img("t10k-images-idx3-ubyte.gz"), yte=lab("t10k-labels-idx1-ubyte.gz"))
    c.put_object(Bucket="mlpipeline", Key=f"{prefix}/data.npz", Body=buf.getvalue())
    print("data.npz", len(buf.getvalue()), "bytes")


@dsl.component(base_image="python:3.12-slim", packages_to_install=BASE)
def presign(run_id: str, mac_s3: str) -> NamedTuple("o", [("inputs", list), ("outputs", list)]):
    prefix = f"study/fashion-{run_id}"
    import os
    import boto3
    from botocore.config import Config
    c = boto3.client("s3", endpoint_url=mac_s3, aws_access_key_id=os.environ["AK"], aws_secret_access_key=os.environ["SK"],
                     region_name="us-east-1", config=Config(s3={"addressing_style": "path"}, signature_version="s3v4"))
    url = lambda op, k: c.generate_presigned_url(op, Params={"Bucket": "mlpipeline", "Key": f"{prefix}/{k}"}, ExpiresIn=7200)
    return ([{"url": url("get_object", "data.npz"), "path": "data.npz"}],
            [{"url": url("put_object", "model.npz"), "path": "model.npz"}])


TRAIN = """
import numpy as np, torch, time
dev = "mps" if torch.backends.mps.is_available() else "cpu"
d = np.load("data.npz"); torch.manual_seed(0)
f = lambda a: torch.tensor(a, dtype=torch.float32, device=dev) / 255
xtr, xte = f(d["xtr"]), f(d["xte"])
ytr, yte = (torch.tensor(d[k], dtype=torch.long, device=dev) for k in ("ytr", "yte"))
m = torch.nn.Sequential(torch.nn.Linear(784, 256), torch.nn.ReLU(), torch.nn.Linear(256, 10)).to(dev)
opt = torch.optim.Adam(m.parameters(), LR)
t = time.time()
for ep in range(EPOCHS):
    p = torch.randperm(len(xtr), device=dev)
    for i in range(0, len(p), 256):
        b = p[i:i + 256]
        loss = torch.nn.functional.cross_entropy(m(xtr[b]), ytr[b]); opt.zero_grad(); loss.backward(); opt.step()
    acc = (m(xte).argmax(1) == yte).float().mean().item()
    print(f"epoch {ep + 1} loss {loss.item():.4f} accuracy={acc:.4f}", flush=True)
print(f"trained in {time.time() - t:.1f}s on {dev}")
w = [x.detach().cpu().numpy() for x in m.parameters()]
np.savez("model.npz", w0=w[0], b0=w[1], w1=w[2], b1=w[3])
"""


@dsl.component(base_image="python:3.12-slim", packages_to_install=BASE)
def evaluate(run_id: str, metrics: dsl.Output[dsl.Metrics], s3: str = "http://seaweedfs.kubeflow:8333"):
    prefix = f"study/fashion-{run_id}"
    import io
    import os
    import boto3
    import numpy as np
    from botocore.config import Config
    c = boto3.client("s3", endpoint_url=s3, aws_access_key_id=os.environ["AK"], aws_secret_access_key=os.environ["SK"],
                     region_name="us-east-1", config=Config(s3={"addressing_style": "path"}, signature_version="s3v4"))
    load = lambda k: np.load(io.BytesIO(c.get_object(Bucket="mlpipeline", Key=f"{prefix}/{k}")["Body"].read()))
    d, m = load("data.npz"), load("model.npz")
    x = d["xte"] / 255.0
    pred = (np.maximum(x @ m["w0"].T + m["b0"], 0) @ m["w1"].T + m["b1"]).argmax(1)
    acc = float((pred == d["yte"]).mean())
    print(f"test accuracy {acc:.4f} (numpy, on CPU)")
    metrics.log_metric("accuracy", acc)


def secrets(task):
    return kubernetes.use_secret_as_env(task, "mlpipeline-minio-artifact", {"accesskey": "AK", "secretkey": "SK"})


@dsl.pipeline(name="fashion-mnist-on-mac-gpu")
def fashion(lr: float = 0.001, epochs: int = 5, mac_s3: str = "http://localhost:8333"):
    run_id = dsl.PIPELINE_JOB_ID_PLACEHOLDER
    data = secrets(prepare_data(run_id=run_id))
    urls = secrets(presign(run_id=run_id, mac_s3=mac_s3)).after(data)
    train = silicon_job(script=TRAIN.replace("LR", str(lr)).replace("EPOCHS", str(epochs)), dependencies=["torch", "numpy"],
                        inputs=urls.outputs["inputs"], outputs=urls.outputs["outputs"])
    secrets(evaluate(run_id=run_id)).after(train)


if __name__ == "__main__":
    compiler.Compiler().compile(fashion, "fashion.yaml")
