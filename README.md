# KubeSilicon

Run Linux virtual machines from Kubernetes objects on a Mac with Apple Silicon, the way KubeVirt does on Linux. It also runs GPU jobs natively on the Mac (PyTorch MPS or MLX) from a `SiliconJob` object, so a Kubeflow Pipelines step can train on the Apple GPU: see [GPU jobs](#gpu-jobs-siliconjob) and [benchmarks](docs/benchmarks.md).

[KubeVirt](https://kubevirt.io) needs KVM, which does not exist on macOS. KubeSilicon replaces the KVM layer with Apple's [`Virtualization.framework`](https://developer.apple.com/documentation/virtualization): you declare a `SiliconVM`, a daemon on the Mac boots it, and pods in your local cluster (kind, for example) can reach it over the network.

> **Status: early.** It works end to end on one Mac with kind on Docker Desktop (see [docs/validation.md](docs/validation.md)). The API is `v1` in name only and will change. Not production software.

https://github.com/user-attachments/assets/a0dd7642-8db3-4625-8520-1b3d0d13762c

## How it works

```
 kind (Docker Desktop VM)              macOS host
 ┌──────────────────────┐              ┌────────────────────────────────────┐
 │ SiliconVM objects    │◄─ API watch ─┤ kubesilicond                        │
 │ (CRD kubesilicon.io) │── status ───►│   download + convert image          │
 │                      │              │   cloud-init ISO                    │
 │ pod ──────── ping/ssh ─────────────►│   Virtualization.framework  ──► VM  │
 └──────────────────────┘              └────────────────────────────────────┘
```

`kubesilicond` runs on the Mac, outside the cluster, watches `SiliconVM` objects and starts one VM for each, in its own detached process. It writes `status.phase` and `status.ip` back and publishes a headless Service, so pods reach the VM by IP or by name (`<name>.<namespace>.svc`).

## Quick start

Requirements: Apple Silicon Mac, macOS 13 or newer, Go, `qemu-img` (`brew install qemu`), a running kind cluster, `kubectl`.

```sh
make install CTX=kind-<your-cluster>    # build, install the CRD, start the daemon at login
```

`CTX` is required on purpose, so the daemon never acts on the wrong cluster. Undo with `make uninstall`.

In another terminal:

```yaml
# vm.yaml
apiVersion: kubesilicon.io/v1
kind: SiliconVM
metadata: { name: demo }
spec:
  cpu: 2
  memory: 2Gi
  disk:
    source: https://cloud-images.ubuntu.com/releases/noble/release/ubuntu-24.04-server-cloudimg-arm64.img
    size: 10Gi
  cloudInit:
    userData: |
      #cloud-config
      users:
        - name: ubuntu
          sudo: ALL=(ALL) NOPASSWD:ALL
          ssh_authorized_keys: [ssh-ed25519 AAAA... you@host]
```

```sh
kubectl apply -f vm.yaml
kubectl get svm -w          # Provisioning -> Starting -> Running, with IP
make bin/kubesilicon        # the client
./bin/kubesilicon -context kind-<your-cluster> ssh demo
# from a pod: ping/ssh demo.kubesilicon.svc (namespace of the SiliconVM)
kubectl patch svm demo --type=merge -p '{"spec":{"running":false}}'   # stop, keeps the disk
kubectl delete svm demo     # stops the VM and removes its disk
```

Stopping `kubesilicond` does **not** stop the VMs. Stop a VM with `running: false`. When the daemon starts again it picks the running VMs back up.

## GPU jobs (SiliconJob)

A Linux guest cannot reach the Mac GPU, so GPU work runs natively on macOS. A `SiliconJob` runs a Python script in its own `uv` venv, inside a `sandbox-exec` profile (writes only in the job directory, no reads of your home), with PyTorch MPS or MLX. One job runs at a time; the others wait as `Pending`.

```yaml
apiVersion: kubesilicon.io/v1
kind: SiliconJob
metadata: { name: train }
spec:
  dependencies: [torch]          # binary wheels only
  timeoutSeconds: 3600
  inputs:  [{ url: "https://.../data.csv", path: data.csv }]      # GET before the job
  outputs: [{ url: "https://.../model.pt?X-Amz-...", path: model.pt }]  # PUT after (presigned)
  script: |
    import torch
    print(torch.backends.mps.is_available())
```

```sh
kubectl get sjob -w
kubesilicon -context kind-<cluster> logs train
kubectl patch sjob train --type=merge -p '{"spec":{"running":false}}'   # cancel
hack/e2e-job.sh kind-<cluster>                                          # end-to-end test
```

### MLX

`kubectl apply -f examples/mlx-matmul.yaml` runs the same job with MLX. Measured on an M5 Pro (one run, float32 4096x4096 matmul, inside the sandbox):

| | per matmul | TFLOPS |
|---|---|---|
| GPU (Metal) | 8.2 ms | 16.7 |
| CPU | 69 ms | 2.0 |

`examples/bench-cnn.yaml` trains the same small CNN (2 conv layers, Fashion-MNIST, 5 epochs, same seed) on both devices. One run on an M5 Pro, with the kind cluster running at the same time and the job at `nice 10`:

| | 5 epochs | test accuracy |
|---|---|---|
| MPS | 7.7 s | 0.9008 |
| CPU | 40.5 s | 0.9007 |

About 5x faster on the GPU, same accuracy. The small MLP in the pipeline example is too small to tell the two apart. For three more benchmarks each on PyTorch and MLX (median of 5, with the cases where the CPU wins), see [docs/benchmarks.md](docs/benchmarks.md).

### With Kubeflow Pipelines

```sh
hack/kind-kubeflow.sh                       # kind (k8s 1.34) + cert-manager + KFP 2.18 + CRDs + RBAC; needs Docker with 12 GB
./bin/kubesilicond -context kind-kubeflow   # or: make install CTX=kind-kubeflow
python examples/kubeflow/pipeline.py        # needs `pip install kfp`; writes train_on_mac_gpu.yaml
kubectl --context kind-kubeflow -n kubeflow port-forward svc/ml-pipeline-ui 8080:80   # upload the YAML at http://localhost:8080
```

The `silicon_job` component in `examples/kubeflow/pipeline.py` creates a `SiliconJob` and waits for it; the job log appears in the step log. Jobs survive a daemon restart and are retried if the Mac reboots (`backoffLimit`). To run this beyond a laptop lab, read [docs/production.md](docs/production.md). Needs [`uv`](https://docs.astral.sh/uv/). See [docs/design.md](docs/design.md).

### Training example: Fashion-MNIST

`examples/kubeflow/fashion.py` is a 4-step pipeline: `prepare_data` (pod) puts the dataset in the KFP SeaweedFS, `presign` (pod) creates the presigned URLs, `silicon_job` trains on the Mac GPU (PyTorch MPS) and `evaluate` (pod) scores the model on CPU. The Mac job gets its data and returns the model through presigned URLs (`inputs` / `outputs` of the `SiliconJob`).

```sh
kubectl -n kubeflow port-forward svc/seaweedfs 8333:8333     # the Mac reaches the bucket here
python examples/kubeflow/fashion.py                           # needs `pip install kfp kfp-kubernetes boto3`; writes fashion.yaml
```

Upload `fashion.yaml` in the KFP UI. Measured on an M5 Pro with Docker Desktop (5 epochs, 784-256-10 MLP): trained in 1.4 s on `mps`, test accuracy 0.8645 (the same number the training reported, so the artifacts round-trip intact).

## Start at login

`make install` copies the daemon to `~/.kubesilicon/bin` and loads a LaunchAgent. `make uninstall` removes the agent; running VMs keep running. launchd restarts the daemon if it dies. After a real Mac reboot the daemon is back at login and waits for the API server, so with kind on Docker Desktop turn on "Start Docker Desktop when you sign in"; nothing else needs a manual start. The daemon's log is `~/.kubesilicon/kubesilicond.log`. Objects deleted while the daemon was down are cleaned up the next time it starts (their VM is shut down and its directory removed).

## Client

```sh
export KUBESILICON_CONTEXT=kind-<your-cluster>     # or pass -context; there is no default on purpose
kubesilicon ip demo                  # print the VM IP
kubesilicon ssh demo                 # ssh as ubuntu; extra ssh args after --, user with -l
kubesilicon ssh demo -- uname -a
kubesilicon console demo             # serial console, leave with Ctrl-]
```

`console` needs a user with a password (cloud-init `plain_text_passwd` and `lock_passwd: false`). It is mostly for debugging boot problems; use `ssh` otherwise. The client calls `kubectl` for the lookup, so `kubectl` must be on the PATH. Host keys are not checked, because VM IPs get reused on a local lab.

Serial output is also written to `~/.kubesilicon/vms/<namespace>_<name>/console.log`.

## What works and what does not

| Works | Not yet |
|---|---|
| Linux ARM64 guests, EFI boot, cloud images | macOS guests |
| cloud-init (NoCloud ISO), `kubesilicon` client (`ip`, `ssh`, `console`) | Windowed or VNC console |
| start, stop (ACPI, then forced), delete with finalizer | Automatic restart of a VM whose launcher died |
| Disk reuse across stop/start, qcow2 to raw conversion | Multiple Macs as nodes |
| Pods reach the VM over NAT, by IP or DNS name | Live migration, hotplug, snapshots, GPU |
| One process per VM: the daemon can stop, restart or crash and VMs keep running | |
| Start at login (LaunchAgent), orphan cleanup | |

Limits that come from Apple's framework: no live migration, no graphical console without a window. Nested virtualization in the guest exists on M3 and newer with macOS 15, but is not enabled yet. See [docs/design.md](docs/design.md) for the mapping to KubeVirt and the roadmap.

## Development

```sh
make test              # unit tests
hack/e2e.sh            # full cycle against a cluster; needs a kubeconfig context named "kubesilicon"
```

Contributions are welcome, see [CONTRIBUTING.md](CONTRIBUTING.md).

## Disclaimer

KubeSilicon is an independent project. It is not affiliated with or endorsed by Apple Inc. or the KubeVirt project. Apple, Mac, macOS and Apple Silicon are trademarks of Apple Inc. Running macOS guests is subject to Apple's license terms; this project only runs Linux guests.

## License

Apache License 2.0. See [LICENSE](LICENSE).
