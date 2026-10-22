# kubesilicon

Run Linux virtual machines from Kubernetes objects on a Mac with Apple Silicon, the way KubeVirt does on Linux.

[KubeVirt](https://kubevirt.io) needs KVM, which does not exist on macOS. kubesilicon replaces the KVM layer with Apple's [`Virtualization.framework`](https://developer.apple.com/documentation/virtualization): you declare a `SiliconVM`, a daemon on the Mac boots it, and pods in your local cluster (kind, for example) can reach it over the network.

> **Status: early.** It works end to end on one Mac with kind on Docker Desktop (see [docs/validation.md](docs/validation.md)). The API is `v1` in name only and will change. Not production software.

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

`kubesilicond` runs on the Mac, outside the cluster, watches `SiliconVM` objects and starts one VM for each. It writes `status.phase` and `status.ip` back. Pods reach the VM directly at that IP.

## Quick start

Requirements: Apple Silicon Mac, macOS 13 or newer, Go, `qemu-img` (`brew install qemu`), a running kind cluster, `kubectl`.

```sh
make bin/kubesilicond                                     # build and ad-hoc codesign
kubectl apply -f config/crd/kubesilicon.io_siliconvms.yaml
./bin/kubesilicond -context kind-<your-cluster>           # -context is required on purpose
```

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
ssh ubuntu@<IP>
kubectl patch svm demo --type=merge -p '{"spec":{"running":false}}'   # stop, keeps the disk
kubectl delete svm demo     # stops the VM and removes its disk
```

The serial console output is in `~/.kubesilicon/vms/<namespace>_<name>/console.log`.

## What works and what does not

| Works | Not yet |
|---|---|
| Linux ARM64 guests, EFI boot, cloud images | macOS guests |
| cloud-init (NoCloud ISO) | Service/DNS per VM (use `status.ip`) |
| start, stop (ACPI, then forced), delete with finalizer | Interactive console, `kubesilicon` CLI |
| Disk reuse across stop/start, qcow2 to raw conversion | Multiple Macs as nodes |
| Pods reach the VM over NAT | Live migration, hotplug, snapshots, GPU |

Limits that come from Apple's framework: no live migration, no graphical console without a window. Nested virtualization in the guest exists on M3 and newer with macOS 15, but is not enabled yet. See [docs/design.md](docs/design.md) for the mapping to KubeVirt and the roadmap.

## Development

```sh
make test              # unit tests
hack/e2e.sh            # full cycle against a cluster; needs a kubeconfig context named "kubesilicon"
```

Contributions are welcome, see [CONTRIBUTING.md](CONTRIBUTING.md).

## Disclaimer

kubesilicon is an independent project. It is not affiliated with or endorsed by Apple Inc. or the KubeVirt project. Apple, Mac, macOS and Apple Silicon are trademarks of Apple Inc. Running macOS guests is subject to Apple's license terms; this project only runs Linux guests.

## License

Apache License 2.0. See [LICENSE](LICENSE).
